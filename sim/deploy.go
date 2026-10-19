package sim

// edgeDist is the distance from c to the nearest map border (0 = on the border row/column).
func (b *Battle) edgeDist(c Cell) int {
	return min(int(c.X), int(c.Y), b.w-1-int(c.X), b.h-1-int(c.Y))
}

// cmdDeploy puts a unit from an Army / Hero slot on the landing zone (spec 4.1).
func (b *Battle) cmdDeploy(c Command) ([]Event, RejectCode) {
	hero := b.snap.Hero != "" && c.ID == b.snap.Hero
	var ud *UnitDef
	slot := -1
	group := 1
	if hero {
		if c.Level != b.snap.HeroLevel {
			return nil, RejectNoStock
		}
		if b.heroUsed {
			return nil, RejectHeroLimit
		}
		ud = &b.rules.Units[b.unitIdx[c.ID]]
	} else {
		ui, ok := b.unitIdx[c.ID]
		if !ok || b.rules.Units[ui].Hero {
			return nil, RejectNoStock
		}
		ud = &b.rules.Units[ui]
		if ud.Group > 1 {
			group = ud.Group
		}
		for i := range b.stock {
			if b.stock[i].ID == c.ID && b.stock[i].Level == c.Level && b.stock[i].Count >= group {
				slot = i
				break
			}
		}
		if slot < 0 {
			return nil, RejectNoStock
		}
	}

	var cells []Cell
	if ud.Paradrop {
		// any free land cell within paradrop_edge_cells of the map edge; the rest of the group takes the nearest
		// free cells by cheb rings around To, inside a ring in (Y, X) order (spec 4.1)
		if !b.inMap(c.To) || b.edgeDist(c.To) >= b.rules.ParadropEdgeCells {
			return nil, RejectOutOfZone
		}
		ok, occ := b.cellState(ud.Class, c.To, 0)
		if !ok {
			return nil, RejectOutOfZone
		}
		if occ {
			return nil, RejectCellOccupied
		}
		cells = append(cells, c.To)
		for r := 1; len(cells) < group && r <= max(b.w, b.h); r++ {
			for y := int(c.To.Y) - r; y <= int(c.To.Y)+r && len(cells) < group; y++ {
				for x := int(c.To.X) - r; x <= int(c.To.X)+r && len(cells) < group; x++ {
					if max(abs(x-int(c.To.X)), abs(y-int(c.To.Y))) != r {
						continue
					}
					cc := Cell{int16(x), int16(y)}
					if ok, occ := b.cellState(ud.Class, cc, 0); ok && !occ {
						cells = append(cells, cc)
					}
				}
			}
		}
		if len(cells) < group {
			return nil, RejectCellOccupied // the whole group or nothing
		}
	} else {
		if !b.inMap(c.To) || b.rules.BuildZone.Contains(c.To) {
			return nil, RejectOutOfZone
		}
		if b.water[b.idx(c.To)] != (ud.Class == ClassSEA) {
			return nil, RejectOutOfZone // ships only on water, everything else only on land
		}
		if b.layer(ud.Class)[b.idx(c.To)] != 0 {
			return nil, RejectCellOccupied
		}
		cells = []Cell{c.To}
	}

	var evs []Event
	for _, cc := range cells {
		e := entity{id: EntityID(len(b.ents) + 1), kind: unitKind(ud), owner: ownerAttacker, contentID: ud.ID,
			level: c.Level, w: 1, h: 1, ap: ud.AP, alive: true, slot: slot, ud: ud}
		e.maxHP = scaledHP(ud.HP, ud.HPGrowthPermille, c.Level)
		e.hp = e.maxHP
		b.ents = append(b.ents, e)
		p := &b.ents[len(b.ents)-1]
		b.place(p, cc)
		if hero {
			b.heroUsed, b.heroID = true, p.id
		}
		evs = append(evs, Event{Kind: EventMoved, Actor: p.id, Cell: cc})
	}
	if !hero {
		b.stock[slot].Count -= group
	}
	return evs, 0
}

// cmdLoad boards a deployed infantry unit onto a carrier; the passenger leaves the grid (spec 4.1).
func (b *Battle) cmdLoad(c Command) ([]Event, RejectCode) {
	p, k := b.ent(c.Unit), b.ent(c.Target)
	if p == nil || k == nil {
		return nil, RejectUnknownEntity
	}
	if p.owner != ownerAttacker || k.owner != ownerAttacker {
		return nil, RejectNotOwner
	}
	if p == k || !p.alive || !k.alive || !p.onField || !k.onField || p.class() != ClassINF || k.ud.Carries == 0 {
		return nil, RejectInvalidTarget
	}
	if len(k.cargo) >= k.ud.Carries {
		return nil, RejectCapacity
	}
	b.lift(p)
	p.aboard = k.id
	k.cargo = append(k.cargo, p.id)
	return []Event{{Kind: EventMoved, Actor: p.id, Target: k.id, Cell: k.pos}}, 0
}
