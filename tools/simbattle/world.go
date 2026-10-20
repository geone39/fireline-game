package main

import (
	"github.com/OWNER/fireline/sim"
)

// world is the bot's picture of the battle, rebuilt from sim.State (the client view: no hidden traps). It carries
// grids for pathing; the bot updates it in place after its own moves and rebuilds it after anything bigger.
type world struct {
	e     *Env
	w, h  int
	water []bool
	ents  []sim.EntityState
	idx   []int32 // EntityID -> index in ents, -1
	bld   []int32 // cell -> index of the live building / wall / defence covering it, -1
	land  []int32 // cell -> index of the ground unit standing there, -1
	air   []int32
	sea   []int32
	def   []*sim.BuildingDef // by ents index; nil for units
	ud    []*sim.UnitDef     // by ents index; nil for buildings
	st    sim.State

	// scratch for dijkstra
	dist   []int32
	parent []int32
	brk    []int32
	heap   []heapItem
}

type heapItem struct{ cost, idx int32 }

func newWorld(e *Env, water []bool) *world {
	r := &e.C.Rules
	n := r.GridW * r.GridH
	return &world{e: e, w: r.GridW, h: r.GridH, water: water,
		bld: make([]int32, n), land: make([]int32, n), air: make([]int32, n), sea: make([]int32, n),
		dist: make([]int32, n), parent: make([]int32, n), brk: make([]int32, n)}
}

func (w *world) inMap(x, y int) bool { return x >= 0 && y >= 0 && x < w.w && y < w.h }
func (w *world) cidx(c sim.Cell) int { return int(c.Y)*w.w + int(c.X) }
func (w *world) cell(i int) sim.Cell { return sim.Cell{X: int16(i % w.w), Y: int16(i / w.w)} }

// load rebuilds everything from a State.
func (w *world) load(st sim.State) {
	w.st = st
	// The index of an entity is ID-1 for the whole battle: State leaves out live hidden traps, so a plain copy would
	// shift every index when one is revealed. Missing IDs are inert placeholders (Owner 2, dead).
	maxID := 0
	for i := range st.Entities {
		maxID = max(maxID, int(st.Entities[i].ID))
	}
	w.ents = make([]sim.EntityState, maxID)
	for i := range w.ents {
		w.ents[i] = sim.EntityState{ID: sim.EntityID(i + 1), Owner: 2}
	}
	for _, e := range st.Entities {
		w.ents[e.ID-1] = e
	}
	if cap(w.idx) < maxID+1 {
		w.idx = make([]int32, maxID+1)
	}
	w.idx = w.idx[:maxID+1]
	for i := range w.idx {
		w.idx[i] = -1
	}
	for i := range w.bld {
		w.bld[i], w.land[i], w.air[i], w.sea[i] = -1, -1, -1, -1
	}
	n := len(w.ents)
	if cap(w.def) < n {
		w.def, w.ud = make([]*sim.BuildingDef, n), make([]*sim.UnitDef, n)
	}
	w.def, w.ud = w.def[:n], w.ud[:n]
	for i := range w.ents {
		e := &w.ents[i]
		w.idx[e.ID] = int32(i)
		w.def[i], w.ud[i] = nil, nil
		switch e.Kind {
		case sim.KindBuilding, sim.KindDefence, sim.KindWall:
			w.def[i] = w.e.Def[e.ContentID]
		default:
			w.ud[i] = w.e.Unit[e.ContentID]
		}
		if !e.Alive || !e.OnField {
			continue
		}
		if w.def[i] != nil {
			for y := int(e.Pos.Y); y < int(e.Pos.Y)+e.H; y++ {
				for x := int(e.Pos.X); x < int(e.Pos.X)+e.W; x++ {
					if w.inMap(x, y) {
						w.bld[y*w.w+x] = int32(i)
					}
				}
			}
		} else if w.ud[i] != nil {
			w.setUnit(int32(i), e.Pos)
		}
	}
}

func (w *world) layer(i int32) []int32 {
	switch w.ud[i].Class {
	case sim.ClassAIR:
		return w.air
	case sim.ClassSEA:
		return w.sea
	}
	return w.land
}

func (w *world) setUnit(i int32, c sim.Cell) {
	if w.inMap(int(c.X), int(c.Y)) {
		w.layer(i)[w.cidx(c)] = i
	}
}

// moveUnit updates the grids after the bot moved a unit.
func (w *world) moveUnit(i int32, to sim.Cell) {
	e := &w.ents[i]
	if l := w.layer(i); l[w.cidx(e.Pos)] == i {
		l[w.cidx(e.Pos)] = -1
	}
	e.Pos = to
	w.setUnit(i, to)
}

func (w *world) get(id sim.EntityID) int32 {
	if int(id) >= len(w.idx) {
		return -1
	}
	return w.idx[id]
}

// footDist is the Chebyshev distance from cell c to the footprint of entity t.
func (w *world) footDist(c sim.Cell, t *sim.EntityState) int {
	dx := max(int(t.Pos.X)-int(c.X), 0, int(c.X)-(int(t.Pos.X)+t.W-1))
	dy := max(int(t.Pos.Y)-int(c.Y), 0, int(c.Y)-(int(t.Pos.Y)+t.H-1))
	return max(dx, dy)
}

// enemyTarget says whether entity i is something the attacker may shoot at.
func (w *world) enemyTarget(i int32) bool {
	e := &w.ents[i]
	if e.Owner != 0 || !e.Alive || !e.OnField {
		return false
	}
	if d := w.def[i]; d != nil && d.Untargetable {
		return false
	}
	return true
}

// passive says the entity is a wall / fence / wire / tank trap: an obstacle, not a goal.
func (w *world) passive(i int32) bool {
	d := w.def[i]
	return d != nil && d.Kind == sim.KindWall
}

// slowCost is the cost of entering a slowing cell for infantry: ceil(100 / (100 - slow_pct)).
func slowCost(pct int) int {
	d := 100 - pct
	if d <= 0 {
		return 1000
	}
	return (100 + d - 1) / d
}

// step describes entering cell (x,y) for a unit of the class: ok=false if it cannot ever (map edge, water,
// another unit, decor); brk >= 0 if a destroyable obstacle stands there (the walker would have to shoot it first).
func (w *world) step(cl sim.UnitClass, self int32, x, y int) (cost int, brk int32, ok bool) {
	if !w.inMap(x, y) {
		return 0, -1, false
	}
	ci := y*w.w + x
	switch cl {
	case sim.ClassAIR:
		if o := w.air[ci]; o >= 0 && o != self {
			return 0, -1, false
		}
		return 1, -1, true
	case sim.ClassSEA:
		if !w.water[ci] {
			return 0, -1, false
		}
		if o := w.sea[ci]; o >= 0 && o != self {
			return 0, -1, false
		}
		return 1, -1, true
	}
	if w.water[ci] {
		return 0, -1, false
	}
	if o := w.land[ci]; o >= 0 && o != self {
		return 0, -1, false
	}
	cost, brk = 1, -1
	if bi := w.bld[ci]; bi >= 0 {
		d := w.def[bi]
		switch d.Block {
		case sim.BlockAll:
			if d.Untargetable {
				return 0, -1, false
			}
			brk = bi
		case sim.BlockVehicles:
			if cl == sim.ClassVEH {
				brk = bi
			}
		default:
			if d.SlowPct > 0 && cl == sim.ClassINF {
				cost = slowCost(d.SlowPct)
			}
		}
	}
	return cost, brk, true
}

func (w *world) blocked(cl sim.UnitClass, self int32, x, y int) bool {
	_, brk, ok := w.step(cl, self, x, y)
	return !ok || brk >= 0
}

var dirs = [8][2]int{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}

func (w *world) push(it heapItem) {
	w.heap = append(w.heap, it)
	i := len(w.heap) - 1
	for i > 0 {
		p := (i - 1) / 2
		if !hless(w.heap[i], w.heap[p]) {
			break
		}
		w.heap[i], w.heap[p] = w.heap[p], w.heap[i]
		i = p
	}
}

func hless(a, b heapItem) bool { return a.cost < b.cost || (a.cost == b.cost && a.idx < b.idx) }

func (w *world) pop() heapItem {
	top := w.heap[0]
	n := len(w.heap) - 1
	w.heap[0] = w.heap[n]
	w.heap = w.heap[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && hless(w.heap[l], w.heap[m]) {
			m = l
		}
		if r < n && hless(w.heap[r], w.heap[m]) {
			m = r
		}
		if m == i {
			break
		}
		w.heap[i], w.heap[m] = w.heap[m], w.heap[i]
		i = m
	}
	return top
}

// dijkstra fills w.dist / w.parent / w.brk from the unit's cell. Entering a destroyable obstacle costs
// 1 + breakCost(obstacle index) and the path may continue through it (the unit has to shoot it down first).
func (w *world) dijkstra(self int32, breakCost func(bi int32) int) {
	e := &w.ents[self]
	cl := w.ud[self].Class
	for i := range w.dist {
		w.dist[i], w.parent[i], w.brk[i] = -1, -1, -1
	}
	w.heap = w.heap[:0]
	src := int32(w.cidx(e.Pos))
	w.dist[src] = 0
	w.push(heapItem{0, src})
	for len(w.heap) > 0 {
		it := w.pop()
		if it.cost > w.dist[it.idx] {
			continue
		}
		fx, fy := int(it.idx)%w.w, int(it.idx)/w.w
		for _, d := range dirs {
			nx, ny := fx+d[0], fy+d[1]
			cost, brk, ok := w.step(cl, self, nx, ny)
			if !ok {
				continue
			}
			if d[0] != 0 && d[1] != 0 && cl != sim.ClassAIR &&
				w.blocked(cl, self, nx, fy) && w.blocked(cl, self, fx, ny) {
				continue // no squeezing through a staircase (spec 2.3)
			}
			if brk >= 0 {
				cost += breakCost(brk)
			}
			nc := it.cost + int32(cost)
			ni := ny*w.w + nx
			if w.dist[ni] < 0 || nc < w.dist[ni] {
				w.dist[ni], w.parent[ni], w.brk[ni] = nc, it.idx, brk
				w.push(heapItem{nc, int32(ni)})
			}
		}
	}
}

// chainTo returns the cells after the start up to and including goal from the last dijkstra.
func (w *world) chainTo(goal int32) []sim.Cell {
	var rev []sim.Cell
	for i := goal; w.parent[i] >= 0; i = w.parent[i] {
		rev = append(rev, w.cell(int(i)))
	}
	out := make([]sim.Cell, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// plan is where a unit goes and what it shoots at.
type plan struct {
	target  int32 // entity to shoot (the goal, or the obstacle in the way when interim)
	goal    int32 // the entity the unit is heading for
	interim bool
	path    []sim.Cell
	stand   sim.Cell
	inRange bool // target is within [lo,hi] from stand
	ok      bool
}

// approach plans the walk of unit self to the cheapest candidate target: cost = path cost to a standing cell
// from which the target is in [lo,hi] + penalty(target). The walk is cut at move budget and at the first
// obstacle (which then becomes the interim target).
func (w *world) approach(self int32, cand []int32, lo, hi, budget int, pen func(t int32) int, breakCost func(bi int32) int) plan {
	u := &w.ents[self]
	w.dijkstra(self, breakCost)
	bestKey := int32(1 << 30)
	bestCell, bestT := int32(-1), int32(-1)
	bestRD := -1
	for _, ti := range cand {
		t := &w.ents[ti]
		p := int32(pen(ti))
		x0, x1 := max(0, int(t.Pos.X)-hi), min(w.w-1, int(t.Pos.X)+t.W-1+hi)
		y0, y1 := max(0, int(t.Pos.Y)-hi), min(w.h-1, int(t.Pos.Y)+t.H-1+hi)
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				ci := int32(y*w.w + x)
				d := w.dist[ci]
				if d < 0 || w.brk[ci] >= 0 {
					continue
				}
				rd := w.footDist(sim.Cell{X: int16(x), Y: int16(y)}, t)
				if rd < lo || rd > hi {
					continue
				}
				key := d + p
				if key < bestKey || (key == bestKey && (rd > bestRD || (rd == bestRD && ci < bestCell))) {
					bestKey, bestCell, bestT, bestRD = key, ci, ti, rd
				}
			}
		}
	}
	if bestT < 0 {
		return plan{}
	}
	pl := plan{target: bestT, goal: bestT, stand: u.Pos, ok: true}
	spent := 0
	if bestCell != int32(w.cidx(u.Pos)) {
		for _, c := range w.chainTo(bestCell) {
			ci := w.cidx(c)
			if b := w.brk[ci]; b >= 0 {
				pl.target, pl.interim = b, true
				break
			}
			cl := w.ud[self].Class
			step, _, _ := w.step(cl, self, int(c.X), int(c.Y))
			if spent+step > budget {
				break
			}
			spent += step
			pl.path = append(pl.path, c)
		}
	}
	if len(pl.path) > 0 {
		pl.stand = pl.path[len(pl.path)-1]
	}
	// stop as soon as the (interim) target is in range: no need to walk up to it
	tgt := &w.ents[pl.target]
	if pl.interim {
		if w.rangeOK(u.Pos, tgt, lo, hi) {
			pl.path, pl.stand = nil, u.Pos
		} else {
			for i, c := range pl.path {
				if w.rangeOK(c, tgt, lo, hi) {
					pl.path, pl.stand = pl.path[:i+1], c
					break
				}
			}
		}
	}
	pl.inRange = w.rangeOK(pl.stand, tgt, lo, hi)
	return pl
}

func (w *world) rangeOK(c sim.Cell, t *sim.EntityState, lo, hi int) bool {
	d := w.footDist(c, t)
	return d >= lo && d <= hi
}
