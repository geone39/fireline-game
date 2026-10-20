package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/OWNER/fireline/sim"
	"github.com/geone39/fireline/server/svc/raid"
)

var (
	envOnce sync.Once
	envVal  *Env
	envErr  error
)

// testEnv loads the real content once; every test only reads it.
func testEnv(t *testing.T) *Env {
	t.Helper()
	envOnce.Do(func() {
		dir, err := FindContentDir(".")
		if err != nil {
			envErr = err
			return
		}
		envVal, envErr = LoadEnv(dir, "", "")
	})
	if envErr != nil {
		t.Fatalf("LoadEnv: %v", envErr)
	}
	return envVal
}

func ref(t *testing.T, e *Env, short string) *RefBase {
	t.Helper()
	rs, err := e.SelectRefs(short)
	if err != nil {
		t.Fatal(err)
	}
	return rs[0]
}

func strat(t *testing.T, e *Env, id string) *Strategy {
	t.Helper()
	ss, err := e.SelectStrategies(id)
	if err != nil {
		t.Fatal(err)
	}
	return &ss[0]
}

func play(t *testing.T, e *Env, r, s string, seed uint64) *Outcome {
	t.Helper()
	o, err := e.Play(ref(t, e, r), strat(t, e, s), seed)
	if err != nil {
		t.Fatalf("%s/%s seed %d: %v", r, s, seed, err)
	}
	return o
}

func TestAllSixteenBasesLoad(t *testing.T) {
	e := testEnv(t)
	if len(e.Refs) != 16 {
		t.Fatalf("want 16 reference bases (HQ1..HQ10 plain + coastal HQ5..HQ10), got %d", len(e.Refs))
	}
	if e.Refs[0].Short() != "hq01" || e.Refs[0].Coastal() {
		t.Fatalf("order: %s first", e.Refs[0].Short())
	}
	for _, r := range e.Refs {
		if r.Coastal() && e.Water[r.Map] == nil {
			t.Errorf("%s: no water grid for map %s", r.ID, r.Map)
		}
	}
}

// Same (base, strategy, seed) => the same command log and the same Result.
func TestPlayIsDeterministic(t *testing.T) {
	e := testEnv(t)
	for _, c := range []struct{ r, s string }{{"hq03", "direct"}, {"hq06_coastal", "mixed"}, {"hq08", "airborne"}, {"hq05_coastal", "naval"}} {
		a, b := play(t, e, c.r, c.s, 7), play(t, e, c.r, c.s, 7)
		if !reflect.DeepEqual(a.Cmds, b.Cmds) {
			t.Errorf("%s/%s: command logs differ between two runs", c.r, c.s)
		}
		if !reflect.DeepEqual(a.Result, b.Result) {
			t.Errorf("%s/%s: results differ between two runs", c.r, c.s)
		}
	}
}

// The seed changes the bot's choices (landing side, composition jitter).
func TestSeedVariesTheBot(t *testing.T) {
	e := testEnv(t)
	logs := map[string]bool{}
	for seed := uint64(1); seed <= 6; seed++ {
		b, _ := json.Marshal(play(t, e, "hq04", "direct", seed).Cmds)
		logs[string(b)] = true
	}
	if len(logs) < 3 {
		t.Fatalf("6 seeds gave only %d different battles", len(logs))
	}
}

// Every command of every strategy is legal for the sim (no RejectCode), the log replays to the same Result on a
// fresh sim.Run, and the server-side Verify accepts it as an honest client.
func TestBotCommandsAreLegalAndReplay(t *testing.T) {
	e := testEnv(t)
	bases := []string{"hq01", "hq02", "hq04", "hq05", "hq05_coastal", "hq07", "hq08_coastal", "hq10", "hq10_coastal"}
	if testing.Short() {
		bases = []string{"hq02", "hq05_coastal", "hq08"}
	}
	for _, rn := range bases {
		for si := range e.Strategies {
			s := &e.Strategies[si]
			rn := rn
			t.Run(rn+"/"+s.ID, func(t *testing.T) {
				t.Parallel()
				o, err := e.Play(ref(t, e, rn), s, 3)
				if err != nil {
					t.Fatal(err)
				}
				if o.NA != "" {
					t.Skip(o.NA)
				}
				if o.Rejected != 0 {
					t.Fatalf("the sim refused %d bot commands: %v", o.Rejected, o.RejectInfo)
				}
				if o.Result.EndReason == sim.EndLogEnd {
					t.Fatal("the log ends without an ending (the server would not accept it)")
				}
				res, err := sim.Run(o.Snap, o.Cmds, 3)
				if err != nil {
					t.Fatal(err)
				}
				if !reflect.DeepEqual(res, o.Result) {
					t.Fatalf("replay differs:\n got  %+v\n want %+v", res, o.Result)
				}
				v, err := raid.Verify(o.Snap, o.Cmds, 3, raid.Claim{StateHash: o.Result.StateHash, EngineVersion: sim.EngineVersion})
				if err != nil {
					t.Fatal(err)
				}
				if !v.ClientAccepted {
					t.Fatalf("Verify rejects the bot log: %v", v.Reasons)
				}
				if o.Stars != o.Result.Stars || o.Destruction != o.Result.Destruction {
					t.Fatalf("outcome fields do not match the result")
				}
			})
		}
	}
}

// Armies stay inside the housing limits, use only units the base can train, and never exceed the spell housing.
func TestArmyRespectsLimits(t *testing.T) {
	e := testEnv(t)
	for _, r := range e.Refs {
		bud := e.Budgets(r)
		for si := range e.Strategies {
			s := &e.Strategies[si]
			for seed := uint64(1); seed <= 3; seed++ {
				a, na := e.Compose(r, s, sim.NewRNG(seed))
				if na != "" {
					continue
				}
				camp, garage := 0, 0
				for _, sl := range a.Slots {
					if !e.Available(r, sl.ID) {
						t.Errorf("%s/%s: %s is not available at HQ%d", r.ID, s.ID, sl.ID, r.HQ)
					}
					ud, m := e.Unit[sl.ID], e.Meta[sl.ID]
					if g := max(1, ud.Group); sl.Count%g != 0 {
						t.Errorf("%s/%s: %s count %d is not a multiple of the squad %d", r.ID, s.ID, sl.ID, sl.Count, g)
					}
					if sl.Level < 1 || sl.Level > e.A.Levels.UnitCap {
						t.Errorf("%s/%s: %s level %d", r.ID, s.ID, sl.ID, sl.Level)
					}
					if m.Pool == "camp" {
						camp += sl.Count * m.Housing
					} else {
						garage += sl.Count * m.Housing
					}
				}
				if camp > bud.Camp || garage > bud.Garage {
					t.Errorf("%s/%s: camp %d/%d garage %d/%d", r.ID, s.ID, camp, bud.Camp, garage, bud.Garage)
				}
				if camp != a.Used.Camp || garage != a.Used.Garage {
					t.Errorf("%s/%s: Used %+v does not match the slots (camp %d garage %d)", r.ID, s.ID, a.Used, camp, garage)
				}
				sp := 0
				for _, sl := range a.Spells {
					sp += sl.Count * e.SpellMeta[sl.ID].Housing
					if e.SpellMeta[sl.ID].UnlockHQ > r.HQ {
						t.Errorf("%s/%s: spell %s locked at HQ%d", r.ID, s.ID, sl.ID, r.HQ)
					}
				}
				if sp > bud.Spell {
					t.Errorf("%s/%s: spells %d/%d", r.ID, s.ID, sp, bud.Spell)
				}
				if a.Hero != "" && r.MaxLevel["b_command"] < e.HeroUnlock[a.Hero] {
					t.Errorf("%s/%s: hero %s needs a higher command post", r.ID, s.ID, a.Hero)
				}
			}
		}
	}
}

func TestNotApplicable(t *testing.T) {
	e := testEnv(t)
	cases := []struct {
		r, s string
		na   bool
	}{
		{"hq01", "armor", true},    // humvee from HQ3
		{"hq05", "airborne", true}, // drone from HQ6
		{"hq05", "naval", true},    // plain map
		{"hq05_coastal", "naval", false},
		{"hq03", "armor", false},
		{"hq01", "rifle_mass", false},
	}
	for _, c := range cases {
		if _, why := e.Compose(ref(t, e, c.r), strat(t, e, c.s), sim.NewRNG(1)); (why != "") != c.na {
			t.Errorf("%s/%s: not-applicable = %q, want %v", c.r, c.s, why, c.na)
		}
	}
}

func TestHousingCapacityScale(t *testing.T) {
	e := testEnv(t)
	// docs: 4 camps x 20 at level 1, 280 housing at HQ10 (4 camps at their top level)
	if got := capacity(e.A.Housing.CampL1, e.A.Housing.CampMax, 1, 10); got != 20 {
		t.Errorf("camp L1: %d", got)
	}
	if got := e.Budgets(ref(t, e, "hq10")).Camp; got != 280 {
		t.Errorf("HQ10 camp housing %d, want 280", got)
	}
	if got := e.Budgets(ref(t, e, "hq01")).Camp; got != 20 {
		t.Errorf("HQ1 camp housing %d, want 20", got)
	}
}

func TestReportAggregationAndVerdicts(t *testing.T) {
	e := testEnv(t)
	mk := func(stars, destr int, stop StopReason, dmg ...NamedInt) *Outcome {
		return &Outcome{Ref: "hq01", HQ: 1, Map: "plain", Strategy: "direct", Stars: stars, Destruction: destr, Turns: 10, Stop: stop,
			Units: 10, Lost: 4, DamageTaken: dmg}
	}
	var outs []*Outcome
	for i := 0; i < 10; i++ { // 7 wins (>=1 star): 70% -> too easy
		o := mk(1, 60, StopTurnCap, NamedInt{"d_mg_nest", 100})
		if i >= 7 {
			o = mk(0, 20, StopNoUnits, NamedInt{"d_mg_nest", 300})
		}
		outs = append(outs, o)
	}
	outs = append(outs, &Outcome{Ref: "hq01", HQ: 1, Map: "plain", Strategy: "armor", NA: "locked"})
	rep := BuildReport(e, outs, ReportMeta{Seeds: 10})
	if len(rep.Cells) != 2 {
		t.Fatalf("cells: %d", len(rep.Cells))
	}
	c := rep.Cells[0]
	if c.N != 10 || c.Wins != 7 || c.WinPct != 70 || c.Verdict != VerdictTooEasy {
		t.Fatalf("cell: %+v", c)
	}
	if c.Stars != [4]int{3, 7, 0, 0} || c.AvgDestruction != 48 || c.AvgLostPct != 40 {
		t.Fatalf("cell numbers: %+v", c)
	}
	if len(c.FailStops) != 1 || c.FailStops[0] != (NamedInt{"no_units", 3}) {
		t.Fatalf("failure causes: %+v", c.FailStops)
	}
	if len(c.FailDamage) == 0 || c.FailDamage[0] != (NamedInt{"d_mg_nest", 900}) {
		t.Fatalf("failure damage: %+v", c.FailDamage)
	}
	if rep.Cells[1].Verdict != VerdictNA || rep.Totals.NotApplic != 1 || rep.Totals.Battles != 10 {
		t.Fatalf("n/a cell: %+v totals %+v", rep.Cells[1], rep.Totals)
	}
	// the edges of the target are inside it
	for _, tc := range []struct {
		wins int
		want string
	}{{2, VerdictTooHard}, {3, VerdictOK}, {6, VerdictOK}, {7, VerdictTooEasy}} {
		var os []*Outcome
		for i := 0; i < 10; i++ {
			st := 0
			if i < tc.wins {
				st = 1
			}
			os = append(os, mk(st, 10, StopNoUnits))
		}
		if got := BuildReport(e, os, ReportMeta{}).Cells[0].Verdict; got != tc.want {
			t.Errorf("%d/10 wins: verdict %s, want %s", tc.wins, got, tc.want)
		}
	}
	md := rep.Markdown()
	for _, want := range []string{"# simbattle", "Матрица win-rate", "hq01", "n/a: locked", "Топ причин провала"} {
		if !strings.Contains(md, want) {
			t.Errorf("markdown lacks %q", want)
		}
	}
	var back Report
	b, err := json.Marshal(rep)
	if err != nil || json.Unmarshal(b, &back) != nil || len(back.Cells) != 2 {
		t.Fatalf("json round trip: %v", err)
	}
}

// The report does not depend on how many workers play the battles.
func TestRunAllIndependentOfWorkers(t *testing.T) {
	e := testEnv(t)
	refs, _ := e.SelectRefs("hq02,hq05_coastal")
	strats, _ := e.SelectStrategies("direct,mixed")
	var sums []string
	for _, w := range []int{1, 4} {
		outs, err := e.RunAll(refs, strats, 2, 5, w, nil)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := json.Marshal(BuildReport(e, outs, ReportMeta{Seeds: 2}))
		sums = append(sums, string(b))
	}
	if sums[0] != sums[1] {
		t.Fatal("report differs between 1 and 4 workers")
	}
}
