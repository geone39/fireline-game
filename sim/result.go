package sim

import "sort"

// destruction returns the weights of destroyed and of all counted buildings (spec 10.2).
func (b *Battle) destruction() (done, total int) {
	for i := range b.ents {
		e := &b.ents[i]
		if e.bd == nil || !e.bd.Counted {
			continue
		}
		w := e.bd.Weight
		if w == 0 {
			w = 1
		}
		total += w
		if !e.alive {
			done += w
		}
	}
	return
}

// Result returns the outcome so far (final once the battle has ended).
func (b *Battle) Result() Result {
	r := Result{EngineVersion: EngineVersion, EndReason: b.endReason, TurnsUsed: b.turn, StateHash: b.stateHash()}
	if len(b.rejected) > 0 {
		r.Rejected = append([]Rejection(nil), b.rejected...)
	}
	done, total := b.destruction()
	if total > 0 {
		r.Destruction = 100 * done / total
		if done == total {
			r.StarFlags |= StarFull
		}
		if 2*done >= total {
			r.StarFlags |= StarHalf
		}
	}
	for i := range b.ents {
		e := &b.ents[i]
		if e.bd == nil {
			continue
		}
		if e.bd.HQ && !e.alive {
			r.HQDestroyed = true
			r.StarFlags |= StarHQ
		}
		if e.bd.Storage {
			p := 1000
			if e.alive {
				p = 1000 * (e.maxHP - e.hp) / e.maxHP
			}
			r.LootBasis.Storages = append(r.LootBasis.Storages, StorageHit{Entity: e.id, ContentID: e.contentID, Level: e.level, DamagePermille: p})
		}
	}
	for f := r.StarFlags; f != 0; f &= f - 1 {
		r.Stars++
	}
	r.LootBasis.PercentDestroyed, r.LootBasis.Stars = r.Destruction, r.Stars
	for i := range b.ents {
		e := &b.ents[i]
		if e.owner == ownerAttacker && !e.alive {
			r.UnitsLost = addSlot(r.UnitsLost, e.contentID, e.level)
		}
	}
	r.HeroDamageTaken = b.heroDmg
	if b.heroID != 0 {
		r.HeroSurvived = b.ents[b.heroID-1].alive
	}
	return r
}

func addSlot(s []SlotCount, id string, level int) []SlotCount {
	for i := range s {
		if s[i].ID == id && s[i].Level == level {
			s[i].Count++
			return s
		}
	}
	s = append(s, SlotCount{ID: id, Level: level, Count: 1})
	sort.Slice(s, func(i, j int) bool {
		if s[i].ID != s[j].ID {
			return s[i].ID < s[j].ID
		}
		return s[i].Level < s[j].Level
	})
	return s
}
