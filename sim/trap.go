package sim

// Hidden traps (spec 8.4): d_mine and d_spring_trap fire when a unit of a matching class steps on them,
// revealed or not. A revealed trap is also a legal Attack target; destroying it that way defuses it.

// trapAt returns the live trap that fires for u standing on c, or nil.
func (b *Battle) trapAt(u *entity, c Cell) *entity {
	id := b.bld[b.idx(c)]
	if id == 0 {
		return nil
	}
	t := &b.ents[id-1]
	if !t.alive || t.bd.Trap == TrapNone || t.bd.Vs&ClassBit(u.class()) == 0 {
		return nil
	}
	if t.bd.Trap == TrapSpring && u.maxHP > t.bd.MaxHPFlung {
		return nil
	}
	return t
}

// fireTrap resolves a trap under u, which has just entered its cell by the step (dx, dy).
func (b *Battle) fireTrap(t, u *entity, dx, dy int) []Event {
	t.alive, t.hp = false, 0 // consumed
	evs := []Event{{Kind: EventTrapTriggered, Actor: t.id, Target: u.id, Cell: t.pos}}
	switch t.bd.Trap {
	case TrapMine:
		// every enemy of the mine of a triggering class within splash_cells, the stepping unit at full strength
		mask := t.bd.Vs
		evs = append(evs, b.blast(t, u, t.pos, t.bd.SplashCells,
			func(*entity) int { return t.bd.Burst },
			func(v *entity) bool { return v.ud != nil && mask&ClassBit(v.ud.Class) != 0 })...)
	case TrapSpring:
		// thrown opposite to the last step, up to fling_cells; walls do not stop the flight, it lands on the
		// farthest free passable cell of the ray (the trap cell itself if there is none)
		land := t.pos
		for k := 1; k <= t.bd.FlingCells; k++ {
			c := Cell{int16(int(t.pos.X) - dx*k), int16(int(t.pos.Y) - dy*k)}
			if ok, occ := b.cellState(u.class(), c, u.id); ok && !occ {
				land = c
			}
		}
		b.lift(u)
		b.place(u, land)
		evs = append(evs, Event{Kind: EventMoved, Actor: u.id, Cell: land})
	}
	return evs
}
