package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OWNER/fireline/sim"
)

// ReportMeta describes the run.
type ReportMeta struct {
	Seeds      int      `json:"seeds"`
	SeedBase   uint64   `json:"seed_base"`
	Bases      []string `json:"bases"`
	Strategies []string `json:"strategies"`
	Levels     string   `json:"levels"`
	Garrison   string   `json:"garrison"`
	TurnCap    int      `json:"turn_cap"`
}

// Verdicts of a cell against the 30..65% target.
const (
	VerdictOK      = "ok"
	VerdictTooEasy = "too_easy" // win rate above the maximum
	VerdictTooHard = "too_hard" // win rate below the minimum
	VerdictNA      = "n/a"
)

// CellStat is the statistics of one (reference base, strategy) pair over all seeds.
type CellStat struct {
	Ref      string `json:"ref"`
	HQ       int    `json:"hq"`
	Map      string `json:"map"`
	Strategy string `json:"strategy"`
	NA       string `json:"na,omitempty"`

	N       int     `json:"n"`
	Wins    int     `json:"wins"` // stars >= win.min_stars
	WinPct  float64 `json:"win_pct"`
	Win2Pct float64 `json:"win2_pct"` // >= 2 stars
	Win3Pct float64 `json:"win3_pct"` // 3 stars
	Stars   [4]int  `json:"stars"`    // histogram 0..3
	AvgStar float64 `json:"avg_stars"`

	AvgDestruction float64    `json:"avg_destruction"`
	AvgTurns       float64    `json:"avg_turns"`
	AvgLostPct     float64    `json:"avg_lost_pct"` // units lost / units deployed
	Stops          []NamedInt `json:"stops"`        // why the battles ended
	// FailStops / FailDamage describe the battles that were NOT won: the stop reasons and the top damage sources.
	FailStops  []NamedInt `json:"fail_stops,omitempty"`
	FailDamage []NamedInt `json:"fail_damage,omitempty"`
	// NotFullStops / NotFullDamage are the same for the battles that did not reach 3 stars (why the base survived).
	NotFullStops  []NamedInt `json:"not3_stops,omitempty"`
	NotFullDamage []NamedInt `json:"not3_damage,omitempty"`
	Verdict       string     `json:"verdict"`
	Army          ArmyInfo   `json:"army,omitempty"`
}

// Totals are the run totals.
type Totals struct {
	Battles    int `json:"battles"`
	NotApplic  int `json:"not_applicable_cells"`
	Rejected   int `json:"rejected_commands"`
	DeployFail int `json:"undeployed_units"` // no free landing cell left (the rim of a coastal map is smaller)
}

// Report is the machine-readable result (report.json).
type Report struct {
	Meta       ReportMeta `json:"meta"`
	TargetMin  int        `json:"target_min_pct"`
	TargetMax  int        `json:"target_max_pct"`
	MinStars   int        `json:"win_min_stars"`
	Cells      []CellStat `json:"cells"`
	Totals     Totals     `json:"totals"`
	TooEasy    []string   `json:"too_easy"`
	TooHard    []string   `json:"too_hard"`
	Failures   []NamedInt `json:"top_failure_causes"`
	NotFull    []NamedInt `json:"top_not3_causes"`
	Damage     []NamedInt `json:"damage_by_source"`
	Silent     []string   `json:"defences_that_never_hurt"`
	Findings   []string   `json:"findings"`
	Assumption []string   `json:"assumptions"`
}

func pct(a, b int) float64 {
	if b == 0 {
		return 0
	}
	return float64(a*1000/b) / 10
}

func addNamed(m map[string]int, name string, v int) { m[name] += v }

func sortedNamed(m map[string]int, top int) []NamedInt {
	out := make([]NamedInt, 0, len(m))
	for k, v := range m {
		out = append(out, NamedInt{k, v})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Value != out[j].Value {
			return out[i].Value > out[j].Value
		}
		return out[i].Name < out[j].Name
	})
	if top > 0 && len(out) > top {
		out = out[:top]
	}
	return out
}

// BuildReport aggregates the outcomes. Outcomes arrive grouped by (base, strategy) in job order.
func BuildReport(e *Env, outs []*Outcome, meta ReportMeta) *Report {
	rep := &Report{Meta: meta, TargetMin: e.A.Targets.MinPct, TargetMax: e.A.Targets.MaxPct, MinStars: e.A.Win.MinStars}
	rep.Assumption = assumptionNotes(e)
	type key struct{ ref, strat string }
	idx := map[key]int{}
	type acc struct {
		destr, turns, lost, units int
		stops, fstops, fdmg       map[string]int
		nstops, ndmg              map[string]int
	}
	var accs []*acc
	allFail, allDmg, allNot3 := map[string]int{}, map[string]int{}, map[string]int{}
	for _, o := range outs {
		k := key{o.Ref, o.Strategy}
		ci, ok := idx[k]
		if !ok {
			ci = len(rep.Cells)
			idx[k] = ci
			rep.Cells = append(rep.Cells, CellStat{Ref: o.Ref, HQ: o.HQ, Map: o.Map, Strategy: o.Strategy, NA: o.NA})
			accs = append(accs, &acc{stops: map[string]int{}, fstops: map[string]int{}, fdmg: map[string]int{}, nstops: map[string]int{}, ndmg: map[string]int{}})
		}
		c, a := &rep.Cells[ci], accs[ci]
		if o.NA != "" {
			continue
		}
		rep.Totals.Battles++
		rep.Totals.Rejected += o.Rejected
		rep.Totals.DeployFail += o.DeployFail
		if c.N == 0 {
			c.Army = o.Army
		}
		c.N++
		c.Stars[min(o.Stars, 3)]++
		a.destr += o.Destruction
		a.turns += o.Turns
		a.lost += o.Lost
		a.units += o.Units
		addNamed(a.stops, string(o.Stop), 1)
		if o.Stars >= e.A.Win.MinStars {
			c.Wins++
		} else {
			addNamed(a.fstops, string(o.Stop), 1)
			addNamed(allFail, string(o.Stop), 1)
			for _, d := range o.DamageTaken {
				addNamed(a.fdmg, d.Name, d.Value)
			}
		}
		if o.Stars < 3 {
			addNamed(a.nstops, string(o.Stop), 1)
			addNamed(allNot3, string(o.Stop), 1)
			for _, d := range o.DamageTaken {
				addNamed(a.ndmg, d.Name, d.Value)
			}
		}
		for _, d := range o.DamageTaken {
			addNamed(allDmg, d.Name, d.Value)
		}
	}
	for i := range rep.Cells {
		c, a := &rep.Cells[i], accs[i]
		if c.NA != "" || c.N == 0 {
			c.Verdict = VerdictNA
			rep.Totals.NotApplic++
			continue
		}
		c.WinPct = pct(c.Wins, c.N)
		c.Win2Pct = pct(c.Stars[2]+c.Stars[3], c.N)
		c.Win3Pct = pct(c.Stars[3], c.N)
		c.AvgStar = float64((c.Stars[1]+2*c.Stars[2]+3*c.Stars[3])*100/c.N) / 100
		c.AvgDestruction = float64(a.destr*10/c.N) / 10
		c.AvgTurns = float64(a.turns*10/c.N) / 10
		c.AvgLostPct = pct(a.lost, a.units)
		c.Stops = sortedNamed(a.stops, 0)
		c.FailStops = sortedNamed(a.fstops, 0)
		c.FailDamage = sortedNamed(a.fdmg, 4)
		c.NotFullStops = sortedNamed(a.nstops, 0)
		c.NotFullDamage = sortedNamed(a.ndmg, 4)
		switch {
		case c.WinPct > float64(rep.TargetMax):
			c.Verdict = VerdictTooEasy
			rep.TooEasy = append(rep.TooEasy, fmt.Sprintf("%s/%s %.0f%%", c.Ref, c.Strategy, c.WinPct))
		case c.WinPct < float64(rep.TargetMin):
			c.Verdict = VerdictTooHard
			rep.TooHard = append(rep.TooHard, fmt.Sprintf("%s/%s %.0f%%", c.Ref, c.Strategy, c.WinPct))
		default:
			c.Verdict = VerdictOK
		}
	}
	rep.Failures = sortedNamed(allFail, 0)
	rep.NotFull = sortedNamed(allNot3, 0)
	rep.Damage = sortedNamed(allDmg, 0)
	rep.Silent = silentDefences(e, outs, allDmg)
	rep.Findings = findings(rep)
	return rep
}

// silentDefences lists the armed defence types that stand in the played bases but never damaged the attacker.
func silentDefences(e *Env, outs []*Outcome, dealt map[string]int) []string {
	played := map[string]bool{}
	for _, o := range outs {
		if o.NA == "" {
			played[o.Ref] = true
		}
	}
	present := map[string]bool{}
	for _, r := range e.Refs {
		if !played[r.Short()] {
			continue
		}
		for t := range r.Count {
			if d := e.Def[t]; d != nil && d.Kind == sim.KindDefence && (d.Dmg > 0 || d.Burst > 0 || d.Trap != sim.TrapNone) {
				present[t] = true
			}
		}
	}
	var out []string
	for t := range present {
		if dealt[t] == 0 {
			out = append(out, t)
		}
	}
	sort.Strings(out)
	return out
}

func assumptionNotes(e *Env) []string {
	a := e.A
	return []string{
		fmt.Sprintf("OQ-QA2-1: housing: camp %d..%d per camp by level (content gives only 4x20 at L1 -> 280 at HQ10); garage %d..%d; spells %d per support bay", a.Housing.CampL1, a.Housing.CampMax, a.Housing.GarageL1, a.Housing.GarageMax, a.Housing.SpellsPerSupportBay),
		fmt.Sprintf("OQ-QA2-2: unit level policy %q (parity = HQ level)", a.Levels.Policy),
		fmt.Sprintf("OQ-QA2-3: bot turn cap %d (300 s / ~12 s per turn), then Retreat", a.Battle.TurnCap),
		fmt.Sprintf("OQ-QA2-4: bunker garrison policy %q (%s, level of the defence barracks)", a.Garrison.Policy, a.Garrison.Unit),
		fmt.Sprintf("OQ-QA2-5: win = at least %d star(s)", a.Win.MinStars),
		"the sim has no randomness in its rules: the seed varies the bot's choices (landing side, composition jitter +-20%)",
		"defence damage does not grow with building level yet (OQ-RAID0-2, SIM-8 queued): the balance of high HQ levels is skewed",
		"bots are simple (no formation play): a low win rate is a lower bound of what a human achieves, a high one is strong evidence of an easy base",
	}
}

func findings(r *Report) []string {
	var out []string
	type agg struct{ n, easy, hard int }
	var refs, strats []string
	byRef, byStrat := map[string]*agg{}, map[string]*agg{}
	count := func(m map[string]*agg, order *[]string, k string, c *CellStat) {
		a := m[k]
		if a == nil {
			a = &agg{}
			m[k] = a
			*order = append(*order, k)
		}
		a.n++
		if c.Verdict == VerdictTooEasy {
			a.easy++
		}
		if c.Verdict == VerdictTooHard {
			a.hard++
		}
	}
	for i := range r.Cells {
		if c := &r.Cells[i]; c.Verdict != VerdictNA {
			count(byRef, &refs, c.Ref, c)
			count(byStrat, &strats, c.Strategy, c)
		}
	}
	pick := func(order []string, m map[string]*agg, f func(*agg) bool) string {
		var l []string
		for _, k := range order {
			if f(m[k]) {
				l = append(l, k)
			}
		}
		return strings.Join(l, ", ")
	}
	if l := pick(refs, byRef, func(a *agg) bool { return a.easy == a.n }); l != "" {
		out = append(out, fmt.Sprintf("bases where EVERY composition wins more than %d%% (too easy): %s", r.TargetMax, l))
	}
	if l := pick(refs, byRef, func(a *agg) bool { return a.hard == a.n }); l != "" {
		out = append(out, fmt.Sprintf("bases where EVERY composition wins less than %d%% (too hard): %s", r.TargetMin, l))
	}
	if l := pick(strats, byStrat, func(a *agg) bool { return a.easy == a.n }); l != "" {
		out = append(out, fmt.Sprintf("compositions that win more than %d%% against EVERY base they apply to (dominant): %s", r.TargetMax, l))
	}
	if l := pick(strats, byStrat, func(a *agg) bool { return a.hard == a.n }); l != "" {
		out = append(out, fmt.Sprintf("compositions that win less than %d%% against EVERY base they apply to (useless): %s", r.TargetMin, l))
	}
	if len(r.Silent) > 0 {
		out = append(out, "defences that never damaged the attacker in any battle: "+strings.Join(r.Silent, ", "))
	}
	if r.Totals.Rejected > 0 {
		out = append(out, fmt.Sprintf("bot bug: the sim refused %d commands", r.Totals.Rejected))
	}
	if r.Totals.DeployFail > 0 {
		out = append(out, fmt.Sprintf("%d units could not be put on the field (no free landing cell)", r.Totals.DeployFail))
	}
	return out
}

// Write saves report.json and report.md in dir.
func (r *Report) Write(dir string) error {
	b, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "report.json"), append(b, '\n'), 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "report.md"), []byte(r.Markdown()), 0o644)
}

func (r *Report) cell(ref, strat string) *CellStat {
	for i := range r.Cells {
		if r.Cells[i].Ref == ref && r.Cells[i].Strategy == strat {
			return &r.Cells[i]
		}
	}
	return nil
}

func named(l []NamedInt, n int) string {
	var parts []string
	for i, x := range l {
		if i >= n {
			break
		}
		parts = append(parts, fmt.Sprintf("%s %d", x.Name, x.Value))
	}
	return strings.Join(parts, ", ")
}

func mark(c *CellStat, r *Report) string {
	switch c.Verdict {
	case VerdictTooEasy:
		return " ▲"
	case VerdictTooHard:
		return " ▼"
	}
	return ""
}

// Markdown renders the report.
func (r *Report) Markdown() string {
	var sb strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&sb, f, a...) }
	p("# simbattle: win-rate по составам против эталонных баз\n\n")
	p("Сидов на ячейку: %d (с %d), уровни юнитов: `%s`, гарнизон: `%s`, лимит ходов бота: %d. Победа = ≥ %d★. Цель: %d–%d%%. ▲ — выше цели, ▼ — ниже.\n\n",
		r.Meta.Seeds, r.Meta.SeedBase, r.Meta.Levels, r.Meta.Garrison, r.Meta.TurnCap, r.MinStars, r.TargetMin, r.TargetMax)
	p("Боёв: %d, не применимо ячеек: %d, отказов sim на команды бота: %d, не выставлено юнитов: %d.\n\n", r.Totals.Battles, r.Totals.NotApplic, r.Totals.Rejected, r.Totals.DeployFail)

	p("## Итог\n\n")
	if len(r.Findings) == 0 {
		p("- замечаний нет\n")
	}
	for _, f := range r.Findings {
		p("- %s\n", f)
	}
	p("- ячеек выше %d%%: %d; ниже %d%%: %d; в цели: %d\n\n", r.TargetMax, len(r.TooEasy), r.TargetMin, len(r.TooHard),
		len(r.Cells)-r.Totals.NotApplic-len(r.TooEasy)-len(r.TooHard))
	if len(r.TooEasy) > 0 {
		p("Выше цели: %s\n\n", strings.Join(r.TooEasy, "; "))
	}

	var strats []string
	seenS := map[string]bool{}
	for _, c := range r.Cells {
		if !seenS[c.Strategy] {
			seenS[c.Strategy] = true
			strats = append(strats, c.Strategy)
		}
	}
	for _, m := range []string{"plain", "coastal"} {
		var refs []string
		seenR := map[string]bool{}
		for _, c := range r.Cells {
			if c.Map == m && !seenR[c.Ref] {
				seenR[c.Ref] = true
				refs = append(refs, c.Ref)
			}
		}
		if len(refs) == 0 {
			continue
		}
		p("## Матрица win-rate, карта `%s` (win ≥%d★ / ≥2★ / 3★, %%)\n\n| база |", m, r.MinStars)
		for _, s := range strats {
			p(" %s |", s)
		}
		p("\n|---|")
		for range strats {
			p("---|")
		}
		p("\n")
		for _, ref := range refs {
			p("| %s |", ref)
			for _, s := range strats {
				c := r.cell(ref, s)
				if c == nil || c.Verdict == VerdictNA {
					p(" — |")
					continue
				}
				p(" %.0f / %.0f / %.0f%s |", c.WinPct, c.Win2Pct, c.Win3Pct, mark(c, r))
			}
			p("\n")
		}
		p("\n")
	}

	sb.WriteString("## Подробно по ячейкам\n\n| база | состав | боёв | win% | ★0 | ★1 | ★2 | ★3 | разрушение % | ходов | потери % | чем кончился бой | урон по армии в боях < 3★ |\n|---|---|---|---|---|---|---|---|---|---|---|---|---|\n")
	for i := range r.Cells {
		c := &r.Cells[i]
		if c.Verdict == VerdictNA {
			p("| %s | %s | — | n/a: %s |  |  |  |  |  |  |  |  |  |\n", c.Ref, c.Strategy, c.NA)
			continue
		}
		p("| %s | %s | %d | %.0f%%%s | %d | %d | %d | %d | %.0f | %.1f | %.0f | %s | %s |\n", c.Ref, c.Strategy, c.N, c.WinPct, mark(c, r),
			c.Stars[0], c.Stars[1], c.Stars[2], c.Stars[3], c.AvgDestruction, c.AvgTurns, c.AvgLostPct, named(c.Stops, 4), named(c.NotFullDamage, 3))
	}

	p("\n## Топ причин провала (бои без победы)\n\n")
	for _, f := range r.Failures {
		p("- %s: %d боёв\n", f.Name, f.Value)
	}
	p("\n## Кто наносит урон армии атакующего (все бои)\n\n")
	for _, d := range r.Damage {
		p("- %s: %d\n", d.Name, d.Value)
	}
	p("\n## Допущения\n\n")
	for _, a := range r.Assumption {
		p("- %s\n", a)
	}
	return sb.String()
}

// Summary is the short text printed to stdout (and pasted into the PR).
func (r *Report) Summary() string {
	var sb strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&sb, f, a...) }
	p("simbattle: %d battles, %d seeds per cell, levels=%s garrison=%s turn-cap=%d, win=>=%d* target %d-%d%%\n",
		r.Totals.Battles, r.Meta.Seeds, r.Meta.Levels, r.Meta.Garrison, r.Meta.TurnCap, r.MinStars, r.TargetMin, r.TargetMax)
	var strats []string
	seenS := map[string]bool{}
	for _, c := range r.Cells {
		if !seenS[c.Strategy] {
			seenS[c.Strategy] = true
			strats = append(strats, c.Strategy)
		}
	}
	p("%-18s", "base")
	for _, s := range strats {
		p(" %9s", s)
	}
	p("\n")
	seenR := map[string]bool{}
	for _, c := range r.Cells {
		if seenR[c.Ref] {
			continue
		}
		seenR[c.Ref] = true
		p("%-18s", c.Ref)
		for _, s := range strats {
			x := r.cell(c.Ref, s)
			if x == nil || x.Verdict == VerdictNA {
				p(" %9s", "-")
			} else {
				p(" %8.0f%s", x.WinPct, strings.TrimSpace(strings.NewReplacer(" ▲", "^", " ▼", "v").Replace(mark(x, r)+" ")))
			}
		}
		p("\n")
	}
	p("rejected commands: %d, undeployed units: %d\n", r.Totals.Rejected, r.Totals.DeployFail)
	return sb.String()
}
