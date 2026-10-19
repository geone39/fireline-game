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
	// SIM-2: stepping on a hidden trap (d_mine, d_spring_trap) interrupts the walk here (spec 5.1, 8.4).
	b.lift(u)
	b.place(u, cur)
	u.apLeft--
	return []Event{{Kind: EventMoved, Actor: u.id, Cell: cur, Value: cost}}, 0
}
