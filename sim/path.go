package sim

// Pathfinding (spec 7.4): Dijkstra on tiles, steps ordered N,NE,E,SE,S,SW,W,NW, queue key (cost, cell index),
// on equal cost the first parent is kept. Other units are obstacles; the walker's own cell is free.

var dirs = [8][2]int{{0, -1}, {1, -1}, {1, 0}, {1, 1}, {0, 1}, {-1, 1}, {-1, 0}, {-1, -1}}

const unreached = -1

type heapItem struct{ cost, idx int32 }

type pathfinder struct {
	dist   []int32
	parent []int32
	heap   []heapItem
}

func (p *pathfinder) less(a, b heapItem) bool {
	return a.cost < b.cost || (a.cost == b.cost && a.idx < b.idx)
}

func (p *pathfinder) push(it heapItem) {
	p.heap = append(p.heap, it)
	i := len(p.heap) - 1
	for i > 0 {
		par := (i - 1) / 2
		if !p.less(p.heap[i], p.heap[par]) {
			break
		}
		p.heap[i], p.heap[par] = p.heap[par], p.heap[i]
		i = par
	}
}

func (p *pathfinder) pop() heapItem {
	top := p.heap[0]
	n := len(p.heap) - 1
	p.heap[0] = p.heap[n]
	p.heap = p.heap[:n]
	i := 0
	for {
		l, r, m := 2*i+1, 2*i+2, i
		if l < n && p.less(p.heap[l], p.heap[m]) {
			m = l
		}
		if r < n && p.less(p.heap[r], p.heap[m]) {
			m = r
		}
		if m == i {
			break
		}
		p.heap[i], p.heap[m] = p.heap[m], p.heap[i]
		i = m
	}
	return top
}

// dijkstra fills b.pf.dist / parent with the cheapest cost from u.pos to every reachable cell. Costs are NOT
// capped by move_cells: callers truncate the chain themselves.
func (b *Battle) dijkstra(u *entity) {
	p := &b.pf
	n := b.w * b.h
	if len(p.dist) != n {
		p.dist = make([]int32, n)
		p.parent = make([]int32, n)
	}
	for i := range p.dist {
		p.dist[i] = unreached
		p.parent[i] = -1
	}
	p.heap = p.heap[:0]
	src := int32(b.idx(u.pos))
	p.dist[src] = 0
	p.push(heapItem{0, src})
	for len(p.heap) > 0 {
		it := p.pop()
		if it.cost > p.dist[it.idx] {
			continue // stale
		}
		from := b.cellOf(int(it.idx))
		for _, d := range dirs {
			to := Cell{int16(int(from.X) + d[0]), int16(int(from.Y) + d[1])}
			if !b.inMap(to) || b.canStep(u, from, to) != 0 {
				continue
			}
			nc := it.cost + int32(b.stepCost(u, to))
			ti := b.idx(to)
			if p.dist[ti] == unreached || nc < p.dist[ti] {
				p.dist[ti] = nc
				p.parent[ti] = it.idx
				p.push(heapItem{nc, int32(ti)})
			}
		}
	}
}

// chainTo returns the cells after the start up to and including goal, from the last dijkstra run.
func (b *Battle) chainTo(goal int32) []Cell {
	var rev []Cell
	for i := goal; b.pf.parent[i] >= 0; i = b.pf.parent[i] {
		rev = append(rev, b.cellOf(int(i)))
	}
	out := make([]Cell, len(rev))
	for i := range rev {
		out[i] = rev[len(rev)-1-i]
	}
	return out
}

// shortestPath returns the cheapest path for u to goal (cells after the start) and its cost.
func (b *Battle) shortestPath(u *entity, goal Cell) ([]Cell, int, bool) {
	if !b.inMap(goal) {
		return nil, 0, false
	}
	b.dijkstra(u)
	gi := b.idx(goal)
	if b.pf.dist[gi] == unreached {
		return nil, 0, false
	}
	return b.chainTo(int32(gi)), int(b.pf.dist[gi]), true
}

// approach implements spec 7.4 step 2 for a unit with no target in range: the cheapest reachable standing cell
// from which some candidate target is in range; target = min (cost, EntityID), cell = min (cost, cell index).
// Returns the walk truncated to move_cells and the chosen target.
func (b *Battle) approach(u *entity) ([]Cell, EntityID, bool) {
	b.dijkstra(u)
	bestCost, bestCell := int32(-1), int32(-1)
	var bestTarget EntityID
	for i := range b.ents {
		t := &b.ents[i]
		if !b.isCandidate(u, t) || b.concealed(u, t, 2) { // hidden or smoked units are not walked to (spec 7.4)
			continue
		}
		lo, hi := u.ud.MinRangeCells, u.ud.RangeCells
		x0, x1 := max(0, int(t.pos.X)-hi), min(b.w-1, int(t.pos.X)+t.w-1+hi)
		y0, y1 := max(0, int(t.pos.Y)-hi), min(b.h-1, int(t.pos.Y)+t.h-1+hi)
		tc, ti := int32(-1), int32(-1)
		for y := y0; y <= y1; y++ {
			for x := x0; x <= x1; x++ {
				ci := y*b.w + x
				d := b.pf.dist[ci]
				if d == unreached || (tc >= 0 && d > tc) {
					continue
				}
				if r := footCell(Cell{int16(x), int16(y)}, t); r < lo || r > hi {
					continue
				}
				if tc < 0 || d < tc || (d == tc && int32(ci) < ti) {
					tc, ti = d, int32(ci)
				}
			}
		}
		if tc >= 0 && (bestCost < 0 || tc < bestCost) { // ascending id scan: ties keep the smaller EntityID
			bestCost, bestCell, bestTarget = tc, ti, t.id
		}
	}
	if bestCost < 0 {
		return nil, 0, false
	}
	chain := b.chainTo(bestCell)
	n := 0
	for n < len(chain) && int(b.pf.dist[b.idx(chain[n])]) <= u.ud.MoveCells {
		n++
	}
	return chain[:n], bestTarget, true
}

// footCell is the Chebyshev distance from a cell to the footprint of t.
func footCell(c Cell, t *entity) int {
	one := entity{pos: c, w: 1, h: 1}
	return footDist(&one, t)
}
