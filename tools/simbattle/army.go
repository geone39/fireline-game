package main

import (
	"fmt"

	"github.com/OWNER/fireline/sim"
)

// Army is the loadout a bot takes into a battle: what Compose built for one (reference base, strategy, seed).
type Army struct {
	Slots     []sim.ArmySlot
	Spells    []sim.ArmySlot
	Hero      string
	HeroLevel int
	Riders    string // unit id that boards carriers (APC, lander), "" = none
	Budget    Housing
	Used      Housing
}

// Housing is the army capacity by pool.
type Housing struct {
	Camp   int `json:"camp"`
	Garage int `json:"garage"`
	Spell  int `json:"spell"`
}

// capacity is the housing of one camp / garage of the given level: linear from l1 to max over its levels
// (OQ-QA2-1: content only says "4 camps x 20 at L1 -> 280 at HQ10").
func capacity(l1, max, level, levels int) int {
	if levels <= 1 || level <= 1 {
		return l1
	}
	level = min(level, levels)
	return l1 + (max-l1)*(level-1)/(levels-1)
}

// poolBudget is the housing of every building of type id in the base.
func (e *Env) poolBudget(ref *RefBase, id string, l1, max int) int {
	total := 0
	for _, it := range ref.Layout.Items {
		if it.Type == id {
			total += capacity(l1, max, it.Level, e.MaxLevels[id])
		}
	}
	return total
}

// Budgets are the three housing limits of the base.
func (e *Env) Budgets(ref *RefBase) Housing {
	h := Housing{
		Camp:   e.poolBudget(ref, "b_camp", e.A.Housing.CampL1, e.A.Housing.CampMax),
		Garage: e.poolBudget(ref, "b_garage", e.A.Housing.GarageL1, e.A.Housing.GarageMax),
	}
	h.Spell = ref.Count["b_support_bay"] * e.A.Housing.SpellsPerSupportBay
	return h
}

// Available says whether the player of this base can train the unit: HQ level, and the level of the building it
// needs (barracks / factory / shipyard) as the reference base has it.
func (e *Env) Available(ref *RefBase, id string) bool {
	m := e.Meta[id]
	if m == nil || e.Unit[id] == nil {
		return false
	}
	for key, need := range m.Unlock {
		if key == "hq" {
			if ref.HQ < need {
				return false
			}
			continue
		}
		if ref.MaxLevel["b_"+key] < need {
			return false
		}
	}
	if m.Pool == "garage" && ref.Count["b_garage"] == 0 {
		return false
	}
	if e.Unit[id].Class == sim.ClassSEA && !ref.Coastal() {
		return false
	}
	return true
}

// UnitLevel is the level the bot's unit has under the level policy (OQ-QA2-2).
func (e *Env) UnitLevel(ref *RefBase, id string, spell bool) int {
	switch e.A.Levels.Policy {
	case "one":
		return 1
	case "research":
		lvl := 1
		lab := ref.MaxLevel["b_laboratory"]
		for _, n := range e.Research {
			if ((!spell && n.Unit == id) || (spell && n.Spell == id)) && n.Lab <= lab {
				lvl = max(lvl, n.ToLevel)
			}
		}
		return lvl
	}
	return max(1, min(ref.HQ, e.A.Levels.UnitCap))
}

// HeroLevel is the hero level for the policy; cmdLevel is the level of the base's command post.
func (e *Env) HeroLevel(cmdLevel int) int {
	switch e.A.Levels.Policy {
	case "one":
		return 1
	case "research":
		return max(1, cmdLevel/2)
	}
	return max(1, cmdLevel)
}

// fill spends budget on the shares (available ones only), jittered by the seed, then tops up the leftover.
func (e *Env) fill(ref *RefBase, shares []Share, budget int, rng *sim.RNG) (slots []sim.ArmySlot, used int) {
	type ent struct {
		id      string
		w       int
		housing int
		group   int
		count   int
	}
	var es []ent
	total := 0
	for _, s := range shares {
		if !e.Available(ref, s.Unit) {
			continue
		}
		ud := e.Unit[s.Unit]
		g := max(1, ud.Group)
		w := s.Pct * (80 + rng.Intn(41))
		es = append(es, ent{id: s.Unit, w: w, housing: max(1, e.Meta[s.Unit].Housing), group: g})
		total += w
	}
	if total == 0 || budget <= 0 {
		return nil, 0
	}
	left := budget
	for i := range es {
		cnt := budget * es[i].w / total / es[i].housing
		cnt -= cnt % es[i].group
		es[i].count = cnt
		left -= cnt * es[i].housing
	}
	for progress := true; progress; {
		progress = false
		for i := range es {
			if need := es[i].group * es[i].housing; left >= need {
				es[i].count += es[i].group
				left -= need
				progress = true
			}
		}
	}
	for _, x := range es {
		if x.count > 0 {
			slots = append(slots, sim.ArmySlot{ID: x.id, Level: e.UnitLevel(ref, x.id, false), Count: x.count})
		}
	}
	return slots, budget - left
}

// NotApplicable explains why a strategy cannot be played against the base ("" = it can).
func (e *Env) NotApplicable(ref *RefBase, s *Strategy) string {
	if s.Naval && !ref.Coastal() {
		return "нужна карта с водой"
	}
	for _, id := range s.Requires {
		if !e.Available(ref, id) {
			m := e.Meta[id]
			if m == nil {
				return "неизвестный юнит " + id
			}
			if m.Unlock["hq"] > ref.HQ {
				return fmt.Sprintf("%s открывается с HQ%d", id, m.Unlock["hq"])
			}
			return fmt.Sprintf("%s недоступен в этой базе (нет здания)", id)
		}
	}
	return ""
}

// Compose builds the army of a strategy for a reference base inside its housing limits. The seed jitters the
// shares by +-20%. The second result is "" or the reason the strategy does not apply.
func (e *Env) Compose(ref *RefBase, s *Strategy, rng *sim.RNG) (*Army, string) {
	if why := e.NotApplicable(ref, s); why != "" {
		return nil, why
	}
	a := &Army{Budget: e.Budgets(ref), Riders: s.Riders}
	// vehicles / ships first: carriers decide how many riders the camps must keep
	gs, gu := e.fill(ref, s.Garage, a.Budget.Garage, rng)
	a.Slots = append(a.Slots, gs...)
	a.Used.Garage = gu
	campBudget := a.Budget.Camp
	if s.Riders != "" && e.Available(ref, s.Riders) {
		seats := 0
		for _, sl := range gs {
			seats += e.Unit[sl.ID].Carries * sl.Count
		}
		rh := max(1, e.Meta[s.Riders].Housing)
		n := min(seats, campBudget/2/rh) // never more than half of the camps ride
		if n > 0 {
			a.Slots = append(a.Slots, sim.ArmySlot{ID: s.Riders, Level: e.UnitLevel(ref, s.Riders, false), Count: n})
			campBudget -= n * rh
			a.Used.Camp += n * rh
		}
	}
	cs, cu := e.fill(ref, s.Camp, campBudget, rng)
	a.Slots = append(a.Slots, cs...)
	a.Used.Camp += cu

	left := a.Budget.Spell
	for _, w := range s.Spells {
		m, ok := e.SpellMeta[w.ID]
		if !ok || m.UnlockHQ > ref.HQ || ref.Count["b_support_bay"] == 0 || e.Spell[w.ID] == nil {
			continue
		}
		n := min(w.Count, left/max(1, m.Housing))
		if n > 0 {
			a.Spells = append(a.Spells, sim.ArmySlot{ID: w.ID, Level: e.UnitLevel(ref, w.ID, true), Count: n})
			left -= n * m.Housing
		}
	}
	a.Used.Spell = a.Budget.Spell - left

	if cmd := ref.MaxLevel["b_command"]; cmd > 0 {
		for _, h := range s.Hero {
			if need, ok := e.HeroUnlock[h]; ok && need <= cmd && e.Unit[h] != nil {
				a.Hero, a.HeroLevel = h, e.HeroLevel(cmd)
				break
			}
		}
	}
	if len(a.Slots) == 0 && a.Hero == "" {
		return nil, "пустой состав (нет доступных юнитов)"
	}
	return a, ""
}

// Garrison fills every bunker of the base with the garrison unit (OQ-QA2-4), the level of the defence barracks.
func (e *Env) Garrison(ref *RefBase) []sim.GarrisonUnit {
	if e.A.Garrison.Policy != "rifle" {
		return nil
	}
	lvl := max(1, ref.MaxLevel["b_defense_barracks"])
	var out []sim.GarrisonUnit
	for _, it := range ref.Layout.Items {
		if d := e.Def[it.Type]; d != nil && d.Garrison > 0 {
			for i := 0; i < d.Garrison; i++ {
				out = append(out, sim.GarrisonUnit{ID: e.A.Garrison.Unit, Level: lvl})
			}
		}
	}
	return out
}
