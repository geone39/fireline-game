package main

import (
	"fmt"
	"hash/fnv"
	"sort"

	"github.com/OWNER/fireline/sim"
	"github.com/geone39/fireline/server/svc/raid"
)

// StopReason says why the bot's battle ended (what the report counts as the cause of a failure).
type StopReason string

const (
	StopFullDestruction StopReason = "full_destruction" // 100% destroyed
	StopNoUnits         StopReason = "no_units"         // the army was wiped out
	StopTurnCap         StopReason = "turn_cap"         // the real-time limit (turn cap) ran out: bot retreats
	StopStuck           StopReason = "stuck"            // the army could not act for several turns: bot retreats
	StopFuse            StopReason = "fuse"             // battle.max_turns of the sim
	StopOther           StopReason = "other"
)

// Outcome is one played battle.
type Outcome struct {
	Ref      string `json:"ref"`
	HQ       int    `json:"hq"`
	Map      string `json:"map"`
	Strategy string `json:"strategy"`
	Seed     uint64 `json:"seed"`

	NA string `json:"na,omitempty"` // strategy does not apply to this base

	Result sim.Result    `json:"-"`
	Cmds   []sim.Command `json:"-"`
	Snap   sim.Snapshot  `json:"-"`

	Stars       int        `json:"stars"`
	Destruction int        `json:"destruction"`
	Turns       int        `json:"turns"`
	Stop        StopReason `json:"stop"`
	RejectInfo  []string   `json:"reject_info,omitempty"` // the first refused commands, for debugging
	Rejected    int        `json:"rejected"`              // commands the sim refused: a bot bug, must be 0
	DeployFail  int        `json:"deploy_fail"`           // units the bot could not put on the field
	Units       int        `json:"units"`
	Lost        int        `json:"lost"`
	// DamageTaken is the damage dealt to the attacker's units by source content id (defence type / garrison).
	DamageTaken []NamedInt `json:"damage_taken,omitempty"`
	Army        ArmyInfo   `json:"army"`
}

// NamedInt is a sorted map entry.
type NamedInt struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

// ArmyInfo is the loadout summary for the report.
type ArmyInfo struct {
	Slots  []sim.ArmySlot `json:"slots"`
	Spells []sim.ArmySlot `json:"spells,omitempty"`
	Hero   string         `json:"hero,omitempty"`
	Used   Housing        `json:"used"`
	Budget Housing        `json:"budget"`
}

type bot struct {
	e     *Env
	ref   *RefBase
	strat *Strategy
	army  *Army
	rng   *sim.RNG
	b     *sim.Battle
	w     *world
	cmds  []sim.Command
	turn  int
	over  bool
	end   sim.EndReason

	moved      map[sim.EntityID]bool
	acted      bool
	deployFail int
	dmg        map[string]int
	hqIdx      int32
	spellsLeft map[string]int
}

func hashSeed(seed uint64, parts ...string) uint64 {
	h := fnv.New64a()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return seed*0x9E3779B97F4A7C15 ^ h.Sum64()
}

// Play plays one battle of a strategy against a reference base. Everything the bot decides is a function of
// (content, base, strategy, seed): the sim has no randomness in its rules (spec 1.6), so the seed only varies the
// bot's choices (landing side, composition jitter, spell timing is not random).
func (e *Env) Play(ref *RefBase, s *Strategy, seed uint64) (*Outcome, error) {
	out := &Outcome{Ref: ref.Short(), HQ: ref.HQ, Map: ref.Map, Strategy: s.ID, Seed: seed}
	rng := sim.NewRNG(hashSeed(seed, ref.ID, s.ID))
	army, na := e.Compose(ref, s, rng)
	if na != "" {
		out.NA = na
		return out, nil
	}
	snap, err := e.C.FullSnapshot(raid.SnapshotInput{BaseID: ref.ID, Map: ref.Map, Layout: ref.Layout,
		Garrison: e.Garrison(ref), Army: army.Slots, Spells: army.Spells, Hero: army.Hero, HeroLevel: army.HeroLevel})
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", ref.ID, s.ID, err)
	}
	b, err := sim.NewBattle(snap, seed)
	if err != nil {
		return nil, fmt.Errorf("%s/%s: %w", ref.ID, s.ID, err)
	}
	bt := &bot{e: e, ref: ref, strat: s, army: army, rng: rng, b: b, w: newWorld(e, e.Water[ref.Map]),
		dmg: map[string]int{}, spellsLeft: map[string]int{}}
	for _, sl := range army.Spells {
		bt.spellsLeft[sl.ID] += sl.Count
	}
	stop := bt.run()

	res := b.Result()
	out.Result, out.Cmds, out.Snap = res, bt.cmds, snap
	out.Stars, out.Destruction, out.Turns, out.Stop = res.Stars, res.Destruction, res.TurnsUsed, stop
	out.Rejected, out.DeployFail = len(res.Rejected), bt.deployFail
	for k, rj := range res.Rejected {
		if k >= 3 {
			break
		}
		c := bt.cmds[rj.Index]
		out.RejectInfo = append(out.RejectInfo, fmt.Sprintf("#%d code=%d op=%d turn=%d unit=%d target=%d id=%s to=%v path=%v", rj.Index, rj.Code, c.Op, c.Turn, c.Unit, c.Target, c.ID, c.To, c.Path))
	}
	for _, sl := range army.Slots {
		out.Units += sl.Count
	}
	for _, l := range res.UnitsLost {
		out.Lost += l.Count
	}
	for name, v := range bt.dmg { // sorted below, order of the map is irrelevant
		out.DamageTaken = append(out.DamageTaken, NamedInt{name, v})
	}
	sort.Slice(out.DamageTaken, func(i, j int) bool {
		if out.DamageTaken[i].Value != out.DamageTaken[j].Value {
			return out.DamageTaken[i].Value > out.DamageTaken[j].Value
		}
		return out.DamageTaken[i].Name < out.DamageTaken[j].Name
	})
	out.Army = ArmyInfo{Slots: army.Slots, Spells: army.Spells, Hero: army.Hero, Used: army.Used, Budget: army.Budget}
	return out, nil
}

// send applies one command and records it. ok = the sim accepted it (a refused command returns no events).
func (bt *bot) send(c sim.Command) ([]sim.Event, bool) {
	if bt.over {
		return nil, false // the battle ended: nothing more to say (the sim would refuse as battle_over)
	}
	c.Turn = bt.turn
	evs, err := bt.b.Apply(c)
	if err != nil {
		panic(err) // the engine cannot continue: a bug in sim, surface it loudly
	}
	bt.cmds = append(bt.cmds, c)
	for _, ev := range evs {
		if ev.Kind == sim.EventBattleEnded {
			bt.over, bt.end = true, sim.EndReason(ev.Value)
		}
	}
	return evs, len(evs) > 0
}

func (bt *bot) refresh() { bt.w.load(bt.b.State()) }

// observe collects the damage dealt to the attacker's units, by the content id of the source.
func (bt *bot) observe(evs []sim.Event) {
	for _, ev := range evs {
		if ev.Kind != sim.EventAttacked {
			continue
		}
		ti, ai := bt.w.get(ev.Target), bt.w.get(ev.Actor)
		if ti < 0 || bt.w.ents[ti].Owner != 1 {
			continue
		}
		name := "?"
		if ai >= 0 {
			name = bt.w.ents[ai].ContentID
			if bt.w.ents[ai].Kind == sim.KindTroop {
				name += " (garrison)"
			}
		} else if ev.Actor == 0 {
			name = "airstrike/zone"
		} else {
			name = "trap"
		}
		bt.dmg[name] += ev.Value
	}
}

func (bt *bot) run() StopReason {
	bt.refresh()
	bt.hqIdx = -1
	for i := range bt.w.ents {
		if d := bt.w.def[i]; d != nil && d.HQ {
			bt.hqIdx = int32(i)
		}
	}
	bt.deploy()
	if bt.over {
		return bt.stopReason(StopOther)
	}
	if _, ok := bt.send(sim.Command{Op: sim.OpStart}); !ok && bt.b.State().Phase == sim.PhaseDeploy {
		bt.send(sim.Command{Op: sim.OpRetreat})
		return StopOther
	}
	bt.turn = 1
	idle := 0
	for ; bt.turn <= bt.e.A.Battle.TurnCap; bt.turn++ {
		bt.refresh()
		bt.moved = map[sim.EntityID]bool{}
		bt.acted = false
		bt.playTurn()
		if bt.over {
			return bt.stopReason(StopOther)
		}
		if bt.acted {
			idle = 0
		} else if idle++; idle >= bt.e.A.Battle.StuckTurns {
			bt.send(sim.Command{Op: sim.OpRetreat})
			return StopStuck
		}
		if bt.turn == bt.e.A.Battle.TurnCap {
			break // the time is up: the player stops, the defence does not get another turn
		}
		evs, _ := bt.send(sim.Command{Op: sim.OpEndTurn})
		bt.refreshObserve(evs)
		if bt.over {
			return bt.stopReason(StopOther)
		}
	}
	bt.turn = min(bt.turn, bt.e.A.Battle.TurnCap)
	bt.send(sim.Command{Op: sim.OpRetreat})
	return StopTurnCap
}

// refreshObserve counts the defence phase damage; the world is rebuilt before the next turn anyway.
func (bt *bot) refreshObserve(evs []sim.Event) {
	bt.observe(evs)
}

func (bt *bot) stopReason(def StopReason) StopReason {
	switch bt.end {
	case sim.EndFullDestruction:
		return StopFullDestruction
	case sim.EndNoUnits:
		return StopNoUnits
	case sim.EndTurnLimit:
		return StopFuse
	}
	return def
}

// ---- deployment ----

type deployItem struct {
	id    string
	level int
	hp    int
	group int
	hero  bool
}

func (bt *bot) deploy() {
	w, e := bt.w, bt.e
	var items []deployItem
	for _, sl := range bt.army.Slots {
		ud := e.Unit[sl.ID]
		g := max(1, ud.Group)
		for k := 0; k < sl.Count/g; k++ {
			items = append(items, deployItem{id: sl.ID, level: sl.Level, hp: ud.HP, group: g})
		}
	}
	// the tough ones go first: they land nearest to the anchor and lead
	sort.SliceStable(items, func(i, j int) bool { return items[i].hp > items[j].hp })
	if bt.army.Hero != "" {
		items = append(items, deployItem{id: bt.army.Hero, level: bt.army.HeroLevel, hero: true, group: 1})
	}
	landAnchor := bt.landingAnchor(false)
	seaAnchor := bt.landingAnchor(true)
	landCells := bt.rimCells(false, landAnchor)
	seaCells := bt.rimCells(true, seaAnchor)
	pcands := bt.paradropCells()
	pc := 0
	// the layer of the world a unit class stands on; a unit placed now is marked with a dummy index until the refresh
	layer := func(cl sim.UnitClass) []int32 {
		switch cl {
		case sim.ClassSEA:
			return w.sea
		case sim.ClassAIR:
			return w.air
		}
		return w.land
	}
	for _, it := range items {
		ud := e.Unit[it.id]
		ok := false
		switch {
		case ud.Paradrop:
			for tries := 0; tries < 60 && pc < len(pcands) && !ok; tries++ {
				c := pcands[pc]
				pc++
				if w.land[w.cidx(c)] >= 0 {
					continue
				}
				if _, ok = bt.send(sim.Command{Op: sim.OpDeploy, ID: it.id, Level: it.level, To: c}); ok {
					bt.refresh() // a squad takes several cells
				}
			}
		default:
			cells := landCells
			if ud.Class == sim.ClassSEA {
				cells = seaCells
			}
			occ := layer(ud.Class)
			for _, c := range cells {
				if occ[w.cidx(c)] >= 0 {
					continue
				}
				if _, ok = bt.send(sim.Command{Op: sim.OpDeploy, ID: it.id, Level: it.level, To: c}); ok {
					occ[w.cidx(c)] = 0
					break
				}
			}
		}
		if !ok {
			bt.deployFail += it.group
		}
	}
	bt.refresh()
	bt.loadRiders()
}

// loadRiders boards the riders onto the carriers (APC, lander) before the start.
func (bt *bot) loadRiders() {
	if bt.army.Riders == "" {
		return
	}
	w := bt.w
	var riders []int32
	for i := range w.ents {
		if e := &w.ents[i]; e.Owner == 1 && e.Alive && e.OnField && e.ContentID == bt.army.Riders && e.Aboard == 0 {
			riders = append(riders, int32(i))
		}
	}
	next := 0
	for i := range w.ents {
		e := &w.ents[i]
		ud := w.ud[i]
		if e.Owner != 1 || !e.Alive || ud == nil || ud.Carries == 0 {
			continue
		}
		for seats := ud.Carries; seats > 0 && next < len(riders); seats-- {
			if _, ok := bt.send(sim.Command{Op: sim.OpLoad, Unit: w.ents[riders[next]].ID, Target: e.ID}); !ok {
				break
			}
			next++
		}
	}
	bt.refresh()
}

// rimCells lists the landing cells (outside the build zone) nearest to the anchor first.
func (bt *bot) rimCells(sea bool, anchor sim.Cell) []sim.Cell {
	r := &bt.e.C.Rules
	var cells []sim.Cell
	for y := 0; y < r.GridH; y++ {
		for x := 0; x < r.GridW; x++ {
			c := sim.Cell{X: int16(x), Y: int16(y)}
			if r.BuildZone.Contains(c) || bt.w.water[y*r.GridW+x] != sea {
				continue
			}
			cells = append(cells, c)
		}
	}
	d2 := func(c sim.Cell) int {
		dx, dy := int(c.X)-int(anchor.X), int(c.Y)-int(anchor.Y)
		return dx*dx + dy*dy
	}
	sort.Slice(cells, func(i, j int) bool {
		if a, b := d2(cells[i]), d2(cells[j]); a != b {
			return a < b
		}
		if cells[i].Y != cells[j].Y {
			return cells[i].Y < cells[j].Y
		}
		return cells[i].X < cells[j].X
	})
	return cells
}

// paradropCells are the free land cells within paradrop_edge_cells of the map edge, nearest to the HQ first
// (paratroopers drop as close to the target as the rule allows).
func (bt *bot) paradropCells() []sim.Cell {
	r := &bt.e.C.Rules
	w := bt.w
	var hq sim.Cell
	if bt.hqIdx >= 0 {
		hq = w.ents[bt.hqIdx].Pos
	}
	var cells []sim.Cell
	for y := 0; y < r.GridH; y++ {
		for x := 0; x < r.GridW; x++ {
			ed := min(x, y, r.GridW-1-x, r.GridH-1-y)
			ci := y*r.GridW + x
			if ed >= r.ParadropEdgeCells || w.water[ci] || w.bld[ci] >= 0 || w.land[ci] >= 0 {
				continue
			}
			cells = append(cells, sim.Cell{X: int16(x), Y: int16(y)})
		}
	}
	d := func(c sim.Cell) int { return max(abs(int(c.X)-int(hq.X)), abs(int(c.Y)-int(hq.Y))) }
	sort.SliceStable(cells, func(i, j int) bool { return d(cells[i]) < d(cells[j]) })
	// every 5th cell: a squad needs room around it
	var out []sim.Cell
	for i := 0; i < len(cells); i += 3 {
		out = append(out, cells[i])
	}
	return out
}

func abs(a int) int {
	if a < 0 {
		return -a
	}
	return a
}

// landingAnchor picks where the army lands: for ground units the rim side from which the HQ is cheapest to reach
// (walls cost a penalty), for ships the water rim cell nearest to the HQ. The seed moves the choice.
func (bt *bot) landingAnchor(sea bool) sim.Cell {
	r := &bt.e.C.Rules
	w := bt.w
	hqC := sim.Cell{X: int16(r.GridW / 2), Y: int16(r.GridH / 2)}
	if bt.hqIdx >= 0 {
		hqC = w.ents[bt.hqIdx].Pos
	}
	if sea {
		best, bd := sim.Cell{X: int16(r.GridW - 1), Y: int16(r.GridH / 2)}, 1<<30
		for y := 0; y < r.GridH; y++ {
			for x := 0; x < r.GridW; x++ {
				c := sim.Cell{X: int16(x), Y: int16(y)}
				if !w.water[y*r.GridW+x] || r.BuildZone.Contains(c) {
					continue
				}
				if d := max(abs(x-int(hqC.X)), abs(y-int(hqC.Y))); d < bd {
					best, bd = c, d
				}
			}
		}
		return best
	}
	mx, my := r.GridW/2, r.GridH/2
	type side struct {
		c    sim.Cell
		cost int32
	}
	var sides []side
	for _, c := range []sim.Cell{{X: int16(mx), Y: 0}, {X: int16(r.GridW - 1), Y: int16(my)}, {X: int16(mx), Y: int16(r.GridH - 1)}, {X: 0, Y: int16(my)}} {
		if w.water[w.cidx(c)] || bt.hqIdx < 0 {
			continue
		}
		sides = append(sides, side{c, bt.probeCost(c, bt.hqIdx)})
	}
	if len(sides) == 0 {
		return sim.Cell{X: 0, Y: int16(my)}
	}
	sort.SliceStable(sides, func(i, j int) bool { return sides[i].cost < sides[j].cost })
	pick := 0
	switch roll := bt.rng.Intn(10); {
	case roll >= 9 && len(sides) > 2:
		pick = 2
	case roll >= 6 && len(sides) > 1:
		pick = 1
	}
	a := sides[pick].c
	shift := bt.rng.Intn(21) - 10 // slide along the side
	if a.X == 0 || int(a.X) == r.GridW-1 {
		a.Y = int16(min(max(int(a.Y)+shift, 2), r.GridH-3))
	} else {
		a.X = int16(min(max(int(a.X)+shift, 2), r.GridW-3))
	}
	return a
}

// probeCost is the path cost for an infantry walker from c to the cell next to the target, walls costing a flat
// penalty. Used only to rank landing sides before the battle.
func (bt *bot) probeCost(c sim.Cell, target int32) int32 {
	w := bt.w
	n := len(w.ents)
	w.ents = append(w.ents, sim.EntityState{ID: sim.EntityID(len(w.idx)), Owner: 1, Kind: sim.KindTroop, ContentID: "u_rifleman",
		Pos: c, W: 1, H: 1, Alive: true, OnField: true})
	w.def = append(w.def, nil)
	w.ud = append(w.ud, bt.e.Unit["u_rifleman"])
	defer func() { w.ents, w.def, w.ud = w.ents[:n], w.def[:n], w.ud[:n] }()
	self := int32(n)
	w.dijkstra(self, func(int32) int { return 15 })
	t := &w.ents[target]
	best := int32(1 << 29)
	for y := max(0, int(t.Pos.Y)-1); y <= min(w.h-1, int(t.Pos.Y)+t.H); y++ {
		for x := max(0, int(t.Pos.X)-1); x <= min(w.w-1, int(t.Pos.X)+t.W); x++ {
			if d := w.dist[y*w.w+x]; d >= 0 && w.brk[y*w.w+x] < 0 && d < best {
				best = d
			}
		}
	}
	return best
}
