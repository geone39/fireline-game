package raid

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/OWNER/fireline/sim"
	"github.com/geone39/fireline/server/svc/base"
)

const (
	contentDir = "../../../content"
	replayGlob = "../../../sim/testdata/replays/G*.json"
)

// goldenFile mirrors the replay format of sim/golden_test.go (testdata/replays/Gxx.json).
type goldenFile struct {
	Name          string       `json:"name"`
	EngineVersion string       `json:"engine_version"`
	Seed          uint64       `json:"seed"`
	Snapshot      sim.Snapshot `json:"snapshot"`
	Commands      []goldenCmd  `json:"commands"`
	Expect        sim.Result   `json:"expect"`
	StateHashHex  string       `json:"state_hash_hex"`
}

type goldenCmd struct {
	Turn   int        `json:"turn"`
	Op     string     `json:"op"`
	Unit   uint32     `json:"unit"`
	Target uint32     `json:"target"`
	ID     string     `json:"id"`
	Level  int        `json:"level"`
	To     *sim.Cell  `json:"to"`
	Path   []sim.Cell `json:"path"`
}

var opNames = []string{"deploy", "load", "start", "move", "attack", "ability", "cast", "end_turn", "retreat"}

func (g goldenFile) commands(t *testing.T) []sim.Command {
	t.Helper()
	out := make([]sim.Command, len(g.Commands))
	for i, c := range g.Commands {
		op := -1
		for k, n := range opNames {
			if n == c.Op {
				op = k
			}
		}
		if op < 0 {
			t.Fatalf("unknown op %q", c.Op)
		}
		cmd := sim.Command{Turn: c.Turn, Op: sim.Op(op), Unit: sim.EntityID(c.Unit), Target: sim.EntityID(c.Target), ID: c.ID, Level: c.Level, Path: c.Path}
		if c.To != nil {
			cmd.To = *c.To
		}
		out[i] = cmd
	}
	return out
}

// Every golden replay of the sim plays on the server the same way: Result and StateHash equal, the client's honest
// claim is accepted.
func TestVerifyGoldenReplays(t *testing.T) {
	files, err := filepath.Glob(replayGlob)
	if err != nil || len(files) < 29 {
		t.Fatalf("golden replays: %v, found %d", err, len(files))
	}
	for _, f := range files {
		t.Run(strings.TrimSuffix(filepath.Base(f), ".json"), func(t *testing.T) {
			raw, err := os.ReadFile(f)
			if err != nil {
				t.Fatal(err)
			}
			var g goldenFile
			if err := json.Unmarshal(raw, &g); err != nil {
				t.Fatal(err)
			}
			hash, err := strconv.ParseUint(strings.TrimPrefix(g.StateHashHex, "0x"), 16, 64)
			if err != nil {
				t.Fatalf("state_hash_hex %q: %v", g.StateHashHex, err)
			}
			v, err := Verify(g.Snapshot, g.commands(t), g.Seed, Claim{StateHash: hash, EngineVersion: g.EngineVersion})
			if err != nil {
				t.Fatal(err)
			}
			if v.Result.StateHash != hash {
				t.Fatalf("StateHash %016x, golden %016x", v.Result.StateHash, hash)
			}
			if !reflect.DeepEqual(v.Result, g.Expect) {
				t.Fatalf("result differs from golden:\n got %+v\nwant %+v", v.Result, g.Expect)
			}
			// A golden may contain refused commands on purpose (G03, G09): then the strict policy must say so.
			if want := len(g.Expect.Rejected) == 0 && g.Expect.EndReason != sim.EndLogEnd; v.ClientAccepted != want {
				t.Fatalf("ClientAccepted = %v (reasons %v), want %v", v.ClientAccepted, v.Reasons, want)
			}
		})
	}
}

func TestVerifyDivergence(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("../../../sim/testdata/replays", "G06.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g goldenFile
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatal(err)
	}
	cmds := g.commands(t)
	honest := g.Expect.StateHash

	t.Run("wrong hash", func(t *testing.T) {
		v, err := Verify(g.Snapshot, cmds, g.Seed, Claim{StateHash: honest ^ 1})
		if err != nil {
			t.Fatal(err)
		}
		if !v.Diverged || v.ClientAccepted || !reflect.DeepEqual(v.Reasons, []Divergence{DivHash}) {
			t.Fatalf("verdict %+v", v)
		}
		if v.Result.StateHash != honest {
			t.Fatal("the server recompute must stay authoritative")
		}
	})
	t.Run("engine version", func(t *testing.T) {
		v, _ := Verify(g.Snapshot, cmds, g.Seed, Claim{StateHash: honest, EngineVersion: "0.0.1"})
		if !reflect.DeepEqual(v.Reasons, []Divergence{DivEngineVersion}) {
			t.Fatalf("reasons %v", v.Reasons)
		}
	})
	t.Run("log without an ending", func(t *testing.T) {
		short := cmds[:len(cmds)-1] // drop the final Retreat
		v, err := Verify(g.Snapshot, short, g.Seed, Claim{StateHash: honest})
		if err != nil {
			t.Fatal(err)
		}
		if v.Result.EndReason != sim.EndLogEnd || v.ClientAccepted {
			t.Fatalf("verdict %+v", v)
		}
	})
	t.Run("rejected command", func(t *testing.T) {
		bad := append(append([]sim.Command(nil), cmds...), sim.Command{Turn: 99, Op: sim.OpEndTurn})
		v, err := Verify(g.Snapshot, bad, g.Seed, Claim{StateHash: honest})
		if err != nil {
			t.Fatal(err)
		}
		if len(v.Result.Rejected) == 0 || v.ClientAccepted {
			t.Fatalf("verdict %+v", v)
		}
	})
	t.Run("bad snapshot is an error", func(t *testing.T) {
		s := g.Snapshot
		s.Buildings = append(append([]sim.PlacedBuilding(nil), s.Buildings...), sim.PlacedBuilding{ID: "nope", Level: 1})
		if _, err := Verify(s, cmds, g.Seed, Claim{}); err == nil {
			t.Fatal("want an error")
		}
	})
}

// ---- real content ----

func loadContent(t *testing.T) *Content {
	t.Helper()
	c, err := LoadContent(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestLoadRulesRealContent(t *testing.T) {
	c := loadContent(t)
	r := c.Rules
	if r.GridW != 44 || r.GridH != 44 || r.MaxTurns != 60 || r.ParadropEdgeCells != 3 || r.SplashFalloffPct != 40 || c.TimeLimitS != 300 {
		t.Fatalf("battle/grid keys: %+v time_limit_s=%d", r, c.TimeLimitS)
	}
	if len(c.Version) != 16 {
		t.Fatalf("version %q", c.Version)
	}
	unit := func(id string) sim.UnitDef {
		for _, u := range r.Units {
			if u.ID == id {
				return u
			}
		}
		t.Fatalf("no unit %s", id)
		return sim.UnitDef{}
	}
	building := func(id string) sim.BuildingDef {
		for _, b := range r.Buildings {
			if b.ID == id {
				return b
			}
		}
		t.Fatalf("no building %s", id)
		return sim.BuildingDef{}
	}
	// turn_s = 2: dmg = dps*2, heal = heal_ps*2 (spec 3.2)
	if u := unit("u_rifleman"); u.HP != 55 || u.Dmg != 18 || u.AP != 2 || u.MoveCells != 4 || u.RangeCells != 4 || u.HPGrowthPermille != 1180 || u.DmgGrowthPermille != 1160 {
		t.Fatalf("u_rifleman %+v", u)
	}
	if u := unit("u_brawler"); u.RangeCells != 1 || u.Dmg != 68 {
		t.Fatalf("u_brawler %+v", u)
	}
	if u := unit("u_breacher"); u.Burst != 160 || u.Dmg != 0 || u.WallMult != 40 || u.SplashCells != 1 {
		t.Fatalf("u_breacher %+v", u)
	}
	if u := unit("u_medic"); u.Heal != 28 || u.HealVs != sim.ClassBit(sim.ClassINF) {
		t.Fatalf("u_medic %+v", u)
	}
	if u := unit("u_saboteur"); u.JamTurns != 4 {
		t.Fatalf("u_saboteur %+v", u)
	}
	if u := unit("u_flamer"); len(u.VsMult) != 1 || u.VsMult[0] != (sim.VsMult{Tag: sim.VsWire, Permille: 1500}) {
		t.Fatalf("u_flamer %+v", u)
	}
	if u := unit("u_paratrooper"); u.Group != 4 || !u.Paradrop {
		t.Fatalf("u_paratrooper %+v", u)
	}
	if u := unit("h_commander"); !u.Hero || u.Ability.Kind != sim.AbilityZoneDamage || u.Ability.CooldownTurns != 20 || u.Ability.DurationTurns != 2 || u.Ability.Dmg != 400 || u.AuraCells != 3 {
		t.Fatalf("h_commander %+v", u)
	}
	if u := unit("h_engineer"); u.Ability.Kind != sim.AbilityDecoy || u.Ability.DecoyUnit != "u_decoy_turret" || u.Ability.DurationTurns != 10 {
		t.Fatalf("h_engineer %+v", u)
	}
	if u := unit("h_ghost"); u.Ability.Kind != sim.AbilityStealth || u.Ability.BonusVs != sim.VsHQ || u.Ability.BonusPermille != 4000 {
		t.Fatalf("h_ghost %+v", u)
	}
	for _, u := range r.Units {
		if u.ID == "h_warden" {
			t.Fatal("h_warden is out of R1 (OQ-7.6) and must be skipped")
		}
	}
	if b := building("d_mine"); !b.Hidden || b.Trap != sim.TrapMine || b.Burst != 180 || b.Block != sim.BlockNone || b.Counted {
		t.Fatalf("d_mine %+v", b)
	}
	if b := building("d_wire"); b.Block != sim.BlockNone || b.SlowPct != 50 || b.DotDmg != 8 || b.Kind != sim.KindWall {
		t.Fatalf("d_wire %+v", b)
	}
	if b := building("d_tank_trap"); b.Block != sim.BlockVehicles {
		t.Fatalf("d_tank_trap %+v", b)
	}
	if b := building("d_mg_nest"); b.Dmg != 60 || b.RangeCells != 7 || b.Vs != sim.ClassBit(sim.ClassINF)|sim.ClassBit(sim.ClassAIR) || !b.Counted || b.HPGrowthPermille != 1200 {
		t.Fatalf("d_mg_nest %+v", b)
	}
	if b := building("d_tesla"); b.Chain != 4 || b.ChainCells != 3 {
		t.Fatalf("d_tesla %+v", b)
	}
	if b := building("b_hq"); !b.HQ || b.HP != 800 || b.HPGrowthPermille != 1220 || b.Weight != 1 {
		t.Fatalf("b_hq %+v", b)
	}
	if b := building("b_vault"); !b.Storage {
		t.Fatalf("b_vault %+v", b)
	}
	if b := building("x_statue"); !b.Untargetable || b.Counted {
		t.Fatalf("x_statue %+v", b)
	}
	if len(r.Spells) != 6 {
		t.Fatalf("spells %+v", r.Spells)
	}
	if got, err := LoadRules(contentDir); err != nil || !reflect.DeepEqual(got, r) {
		t.Fatalf("LoadRules differs from LoadContent: %v", err)
	}
}

// The loader is deterministic: same content, same Rules, same version.
func TestLoadContentDeterministic(t *testing.T) {
	a, b := loadContent(t), loadContent(t)
	if a.Version != b.Version || !reflect.DeepEqual(a.Rules, b.Rules) {
		t.Fatal("two loads differ")
	}
}

// Missing battle keys are errors, all of them reported.
func TestLoadRulesMissingKeys(t *testing.T) {
	dir := copyContent(t)
	mustWrite(t, filepath.Join(dir, "battle.yaml"), "schema: 1\nbattle:\n  turn_s: 2\n  max_turns: 60\n")
	_, err := LoadRules(dir)
	if err == nil {
		t.Fatal("want an error")
	}
	for _, k := range []string{"time_limit_s", "splash_falloff_pct", "spells_per_turn", "paradrop_edge_cells"} {
		if !strings.Contains(err.Error(), k) {
			t.Errorf("error does not mention %s: %v", k, err)
		}
	}

	dir = copyContent(t)
	b, _ := os.ReadFile(filepath.Join(dir, "troops.yaml"))
	mustWrite(t, filepath.Join(dir, "troops.yaml"), strings.Replace(string(b), "ap: 2, move_cells: 4, range_cells: 4}", "move_cells: 4, range_cells: 4}", 1))
	if _, err := LoadRules(dir); err == nil || !strings.Contains(err.Error(), `unit u_rifleman: missing "ap"`) {
		t.Fatalf("missing ap: %v", err)
	}

	if _, err := LoadRules(t.TempDir()); err == nil {
		t.Fatal("empty dir must fail")
	}
}

func copyContent(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, f := range []string{"battle", "base_grid", "troops", "defences", "buildings", "heroes"} {
		b, err := os.ReadFile(filepath.Join(contentDir, f+".yaml"))
		if err != nil {
			t.Fatal(err)
		}
		mustWrite(t, filepath.Join(dir, f+".yaml"), string(b))
	}
	return dir
}

func mustWrite(t *testing.T, path, s string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ---- snapshot + battle on real content ----

func testLayout() base.Layout {
	return base.Layout{Items: []base.Item{
		{UID: "hq", Type: "b_hq", Level: 1, X: 20, Y: 10},
		{UID: "nest", Type: "d_mg_nest", Level: 1, X: 12, Y: 10},
		{UID: "vault", Type: "b_vault", Level: 1, X: 24, Y: 20},
		{UID: "m1", Type: "d_mine", Level: 1, X: 4, Y: 14},
		{UID: "m2", Type: "d_mine", Level: 1, X: 6, Y: 14},
		{UID: "wall", Type: "d_wall", Level: 1, X: 18, Y: 14},
		{UID: "flag", Type: "x_flag", Level: 1, X: 30, Y: 30},
	}}
}

func testInput() SnapshotInput {
	return SnapshotInput{
		BaseID: "b1", Map: "plain", Layout: testLayout(),
		Army:   []sim.ArmySlot{{ID: "u_rifleman", Level: 1, Count: 6}, {ID: "u_brawler", Level: 1, Count: 2}},
		Spells: []sim.ArmySlot{{ID: "c_airstrike", Level: 1, Count: 1}},
		Hero:   "h_commander", HeroLevel: 1,
	}
}

func TestClientSnapshotHidesTraps(t *testing.T) {
	c := loadContent(t)
	in := testInput()
	in.Revealed = map[string]bool{"m2": true}

	full, err := c.FullSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	client, err := c.ClientSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	count := func(s sim.Snapshot, id string) (n, revealed int) {
		for _, b := range s.Buildings {
			if b.ID == id {
				n++
				if b.Revealed {
					revealed++
				}
			}
		}
		return
	}
	if n, rev := count(full, "d_mine"); n != 2 || rev != 1 {
		t.Fatalf("full snapshot: %d mines, %d revealed", n, rev)
	}
	if n, rev := count(client, "d_mine"); n != 1 || rev != 1 {
		t.Fatalf("client snapshot must carry only the revealed mine: %d, %d", n, rev)
	}
	if len(full.Buildings) != len(client.Buildings)+1 || full.ContentVersion != c.Version {
		t.Fatalf("buildings %d vs %d, version %q", len(full.Buildings), len(client.Buildings), full.ContentVersion)
	}
}

func TestSnapshotRejectsBadInput(t *testing.T) {
	c := loadContent(t)
	cases := map[string]func(*SnapshotInput){
		"unknown type":  func(in *SnapshotInput) { in.Layout.Items[0].Type = "b_nope" },
		"zero level":    func(in *SnapshotInput) { in.Layout.Items[0].Level = 0 },
		"unknown map":   func(in *SnapshotInput) { in.Map = "moon" },
		"empty map":     func(in *SnapshotInput) { in.Map = "" },
		"unknown unit":  func(in *SnapshotInput) { in.Army[0].ID = "u_nope" },
		"zero count":    func(in *SnapshotInput) { in.Army[0].Count = 0 },
		"unknown spell": func(in *SnapshotInput) { in.Spells[0].ID = "c_nope" },
		"not a hero":    func(in *SnapshotInput) { in.Hero = "u_rifleman" },
		"hero level":    func(in *SnapshotInput) { in.HeroLevel = 0 },
		"hero garrison": func(in *SnapshotInput) { in.Garrison = []sim.GarrisonUnit{{ID: "h_commander", Level: 1}} },
	}
	for name, mut := range cases {
		t.Run(name, func(t *testing.T) {
			in := testInput()
			in.Layout = base.Layout{Items: append([]base.Item(nil), in.Layout.Items...)}
			in.Army = append([]sim.ArmySlot(nil), in.Army...)
			in.Spells = append([]sim.ArmySlot(nil), in.Spells...)
			mut(&in)
			if _, err := c.FullSnapshot(in); err == nil {
				t.Fatal("want an error")
			}
		})
	}
}

// A real-content battle: the rules load, the engine accepts the snapshot, the replay is reproducible, and a
// client that played on the trap-less view diverges from the server's full recompute (OQ-7.8).
func TestVerifyRealContentBattle(t *testing.T) {
	c := loadContent(t)
	in := testInput()
	full, err := c.FullSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	client, err := c.ClientSnapshot(in)
	if err != nil {
		t.Fatal(err)
	}
	// Deploy a rifleman, walk it over a hidden mine at (4,14); the mine kills it (EndNoUnits). Buildings take the first entity ids, so the unit is the next one.
	cmds := func(path ...sim.Cell) []sim.Command {
		return []sim.Command{
			{Op: sim.OpDeploy, ID: "u_rifleman", Level: 1, To: sim.Cell{X: 1, Y: 14}},
			{Op: sim.OpStart},
			{Turn: 1, Op: sim.OpMove, Unit: sim.EntityID(len(testLayout().Items) + 1), Path: path},
		}
	}
	walk := cmds(sim.Cell{X: 2, Y: 14}, sim.Cell{X: 3, Y: 14}, sim.Cell{X: 4, Y: 14})
	clientRes, err := sim.Run(client, walk, 7)
	if err != nil {
		t.Fatal(err)
	}
	v, err := Verify(full, walk, 7, Claim{StateHash: clientRes.StateHash, EngineVersion: sim.EngineVersion})
	if err != nil {
		t.Fatal(err)
	}
	if v.ClientAccepted || !v.Diverged || !reflect.DeepEqual(v.Reasons, []Divergence{DivHash}) {
		t.Fatalf("a client on the trap-less view must diverge: %+v", v)
	}
	if len(v.Result.Rejected) != 0 || v.Result.EndReason != sim.EndNoUnits {
		t.Fatalf("server result %+v", v.Result)
	}
	// An honest claim (the same snapshot) is accepted and the result is stable.
	again, err := Verify(full, walk, 7, Claim{StateHash: v.Result.StateHash})
	if err != nil || !again.ClientAccepted || !reflect.DeepEqual(again.Result, v.Result) {
		t.Fatalf("honest claim: %+v %v", again, err)
	}
}

// Every unit, hero, spell and building of the real content can be put on a battlefield: NewBattle validates the
// whole Rules and each id is playable.
func TestRealContentIsPlayable(t *testing.T) {
	c := loadContent(t)
	in := SnapshotInput{BaseID: "b", Map: "coastal", Layout: testLayout()}
	for _, u := range c.Rules.Units {
		if u.Hero || u.ID == "u_decoy_turret" {
			continue
		}
		in.Army = append(in.Army, sim.ArmySlot{ID: u.ID, Level: 3, Count: 1})
	}
	for _, s := range c.Rules.Spells {
		in.Spells = append(in.Spells, sim.ArmySlot{ID: s.ID, Level: 1, Count: 1})
	}
	for _, u := range c.Rules.Units {
		if !u.Hero {
			continue
		}
		in.Hero, in.HeroLevel = u.ID, 2
		snap, err := c.FullSnapshot(in)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := sim.NewBattle(snap, 1); err != nil {
			t.Fatalf("hero %s: %v", u.ID, err)
		}
	}
}
