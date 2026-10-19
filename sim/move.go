package sim

// cmdMove walks a unit along an explicit Path (spec 5.1): 1 AP, every step an 8-neighbour of the previous cell,
// total cost <= move_cells. The first violation in path order decides the RejectCode.
func (b *Battle) cmdMove(c Command) ([]Event, RejectCode) {
	u := b.ent(c.Unit)
	if u == nil {
		return nil, RejectUnknownEntity
	}
	if u.owner != ownerAttacker {
		return nil, RejectNotOwner
	}
	if !u.alive || !u.onField {
		return nil, RejectInvalidTarget // dead, or aboard a carrier
	}
	if u.apLeft < 1 {
		return nil, RejectNoAP
	}
	if u.ud.MoveCells == 0 || len(c.Path) == 0 {
		return nil, RejectBadPath
	}
	cur, cost := u.pos, 0
	for _, to := range c.Path {
		if Cheb(cur, to) != 1 {
			return nil, RejectBadPath
		}
		if code := b.canStep(u, cur, to); code != 0 {
			return nil, code
		}
		cost += b.stepCost(u, to)
		if cost > u.ud.MoveCells {
			return nil, RejectPathCost
		}
		cur = to
	}
	// The walk stops on the first trap that fires; the rest of the path is dropped, the AP is spent (spec 5.1).
	var trap *entity
	walked, stop := 0, len(c.Path)
	for i, to := range c.Path {
		walked += b.stepCost(u, to)
		if trap = b.trapAt(u, to); trap != nil {
			stop = i + 1
			break
		}
	}
	end, prev := c.Path[stop-1], u.pos
	if stop > 1 {
		prev = c.Path[stop-2]
	}
	b.lift(u)
	b.place(u, end)
	u.apLeft--
	evs := []Event{{Kind: EventMoved, Actor: u.id, Cell: end, Value: walked}}
	if trap != nil {
		evs = append(evs, b.fireTrap(trap, u, int(end.X)-int(prev.X), int(end.Y)-int(prev.Y))...)
	}
	return evs, 0
}
