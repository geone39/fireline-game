package base

// Rules bundles the immutable inputs of validation: content catalog and terrain.
type Rules struct {
	Cat  *Catalog
	Grid Grid
}

// occupancy maps each tile to index+1 of the occupying item (0 = free).
type occupancy []int

func (r Rules) occupy(l Layout) (occupancy, *Error) {
	occ := make(occupancy, r.Grid.W*r.Grid.H)
	for i, it := range l.Items {
		d, ok := r.Cat.Def(it.Type)
		if !ok {
			return nil, fail(CodeUnknownType, it.UID, "unknown type %q", it.Type)
		}
		for _, p := range Tiles(d, it) {
			if !r.Grid.In(p.X, p.Y) {
				return nil, fail(CodeOutOfBounds, it.UID, "tile (%d,%d) is outside the grid", p.X, p.Y)
			}
			if j := occ[p.Y*r.Grid.W+p.X]; j != 0 {
				return nil, fail(CodeOverlap, it.UID, "overlaps %s at (%d,%d)", l.Items[j-1].UID, p.X, p.Y)
			}
			occ[p.Y*r.Grid.W+p.X] = i + 1
		}
	}
	return occ, nil
}

// checkShape validates one item in isolation: type, rotation, level, bounds, terrain.
func (r Rules) checkShape(it Item) *Error {
	d, ok := r.Cat.Def(it.Type)
	if !ok {
		return fail(CodeUnknownType, it.UID, "unknown type %q", it.Type)
	}
	if it.Rot < 0 || it.Rot > 3 {
		return fail(CodeBadRot, it.UID, "rotation %d not in 0..3", it.Rot)
	}
	if it.Level < 1 {
		return fail(CodeBadLevel, it.UID, "level %d < 1", it.Level)
	}
	tiles := Tiles(d, it)
	for _, p := range tiles {
		if !r.Grid.In(p.X, p.Y) {
			return fail(CodeOutOfBounds, it.UID, "tile (%d,%d) is outside the %dx%d grid", p.X, p.Y, r.Grid.W, r.Grid.H)
		}
		if r.Grid.Water(p.X, p.Y) {
			return fail(CodeOnWater, it.UID, "tile (%d,%d) is water; only the shore may be built on", p.X, p.Y)
		}
	}
	if d.RequiresWater && !r.touchesWater(d, it) {
		return fail(CodeNeedsWater, it.UID, "%s must touch the shore", it.Type)
	}
	return nil
}

// touchesWater: some footprint tile has a 4-neighbour water tile.
// ASSUMPTION: "у берега" = footprint edge-adjacent to at least one water tile.
func (r Rules) touchesWater(d Def, it Item) bool {
	for _, p := range Tiles(d, it) {
		for _, n := range [4]Point{{p.X + 1, p.Y}, {p.X - 1, p.Y}, {p.X, p.Y + 1}, {p.X, p.Y - 1}} {
			if r.Grid.Water(n.X, n.Y) {
				return true
			}
		}
	}
	return false
}

// checkUnlock enforces unlock_hq for a newly placed item.
func (r Rules) checkUnlock(it Item, hq int) *Error {
	d, _ := r.Cat.Def(it.Type)
	if hq < d.UnlockHQ {
		return fail(CodeLocked, it.UID, "%s needs HQ %d, have %d", it.Type, d.UnlockHQ, hq)
	}
	return nil
}

// checkCount enforces the per-HQ limit for the type of it given the whole layout.
func (r Rules) checkCount(l Layout, typ string, hq int) *Error {
	d, _ := r.Cat.Def(typ)
	maxN, limited := r.Cat.MaxCount(d, hq)
	if !limited {
		return nil
	}
	n := 0
	for _, it := range l.Items {
		if it.Type == typ {
			n++
		}
	}
	if n > maxN {
		return fail(CodeLimit, "", "%s: %d placed, limit %d at HQ %d", typ, n, maxN, hq)
	}
	return nil
}

// ValidateLayout checks a full layout: unique uids, exactly one HQ, shapes, overlaps,
// HQ unlocks and limits, and that the HQ is not completely sealed in.
func (r Rules) ValidateLayout(l Layout) error {
	seen := make(map[string]struct{}, len(l.Items)) // membership only, never iterated
	hqCount := 0
	for _, it := range l.Items {
		if _, dup := seen[it.UID]; dup || it.UID == "" {
			return fail(CodeDuplicateUID, it.UID, "empty or duplicate uid")
		}
		seen[it.UID] = struct{}{}
		if err := r.checkShape(it); err != nil {
			return err
		}
		if it.Type == r.Cat.hq {
			hqCount++
		}
	}
	if hqCount == 0 {
		return fail(CodeNoHQ, "", "layout has no HQ")
	}
	if _, err := r.occupy(l); err != nil {
		return err
	}
	hq := l.HQLevel(r.Cat)
	for _, it := range l.Items {
		if err := r.checkUnlock(it, hq); err != nil {
			return err
		}
	}
	for _, id := range r.Cat.ids { // sorted: deterministic first error
		if err := r.checkCount(l, id, hq); err != nil {
			return err
		}
	}
	if a := r.Analyze(l); !a.HQReachable {
		return fail(CodeSealed, "", "HQ is completely enclosed without destructible walls")
	}
	return nil
}

// Analysis describes how attackers can reach the HQ on foot.
type Analysis struct {
	// HQReachable: a path from the grid border to a tile next to the HQ exists when
	// destructible walls (passive defences that block everyone) may be broken through.
	HQReachable bool
	// HQOpen: the same path exists without breaking anything (an open gate).
	HQOpen bool
}

// Analyze computes infantry reachability of the HQ over 4-connected tiles.
// Free tiles and non-blocking items (wire, mines, decor, anti-vehicle traps) are
// walkable; walls are walkable only in the "breaching" pass; buildings, active
// defences and obstacles are never walkable.
// ASSUMPTION: only the HQ must stay reachable; other buildings may be walled in by
// neighbours. The layout is assumed to have passed occupy() (no overlaps/out of bounds).
func (r Rules) Analyze(l Layout) Analysis {
	occ, err := r.occupy(l)
	hqIdx := l.Find(r.hqUID(l))
	if err != nil || hqIdx < 0 {
		return Analysis{}
	}
	return Analysis{
		HQReachable: r.reach(l, occ, hqIdx, true),
		HQOpen:      r.reach(l, occ, hqIdx, false),
	}
}

func (r Rules) hqUID(l Layout) string {
	for _, it := range l.Items {
		if it.Type == r.Cat.hq {
			return it.UID
		}
	}
	return ""
}

func (r Rules) reach(l Layout, occ occupancy, hqIdx int, breach bool) bool {
	w, h := r.Grid.W, r.Grid.H
	walk := func(x, y int) bool {
		if r.Grid.Water(x, y) {
			return false
		}
		j := occ[y*w+x]
		if j == 0 {
			return true
		}
		d, _ := r.Cat.Def(l.Items[j-1].Type)
		switch {
		case d.Block != BlockAll:
			return true
		case d.Kind == KindPassive:
			return breach
		}
		return false
	}
	seen := make([]bool, w*h)
	var queue []Point
	push := func(x, y int) {
		if r.Grid.In(x, y) && !seen[y*w+x] && walk(x, y) {
			seen[y*w+x] = true
			queue = append(queue, Point{x, y})
		}
	}
	for x := 0; x < w; x++ {
		push(x, 0)
		push(x, h-1)
	}
	for y := 0; y < h; y++ {
		push(0, y)
		push(w-1, y)
	}
	for i := 0; i < len(queue); i++ {
		p := queue[i]
		for _, n := range [4]Point{{p.X + 1, p.Y}, {p.X - 1, p.Y}, {p.X, p.Y + 1}, {p.X, p.Y - 1}} {
			if r.Grid.In(n.X, n.Y) && occ[n.Y*w+n.X] == hqIdx+1 {
				return true
			}
			push(n.X, n.Y)
		}
	}
	return false
}
