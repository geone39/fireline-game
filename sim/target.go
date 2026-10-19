package sim

import "sort"

// Target selection (spec 5.2, 7.3): candidates inside [min_range, range] by Chebyshev distance to the footprint,
// best = min (dist, hp, EntityID). Damage and the defence AI that use it are SIM-2/3.

// lineOfFire is the R1 stub (OQ-7.5: no LOS): always true. Changing it needs a spec change.
func lineOfFire(from, to Cell) bool { return true }

// isCandidate says whether t is a legal target kind for shooter s, ignoring range.
func (b *Battle) isCandidate(s, t *entity) bool {
	if t.owner == s.owner || !t.alive || !t.onField {
		return false
	}
	if s.owner == ownerAttacker {
		if t.bd != nil {
			if t.bd.Untargetable || (t.bd.Hidden && !t.revealed) {
				return false
			}
		}
		return true
	}
	return true
}

// shooterStats returns attack numbers of an entity; ok=false if it cannot attack.
func shooterStats(s *entity) (lo, hi int, vs uint8, ok bool) {
	if s.ud != nil {
		return s.ud.MinRangeCells, s.ud.RangeCells, 0xff, s.ud.Dmg > 0
	}
	return s.bd.MinRangeCells, s.bd.RangeCells, s.bd.Vs, s.bd.Dmg > 0
}

// targetsInRange lists the targets s can attack from where it stands, best first.
func (b *Battle) targetsInRange(s *entity) []EntityID {
	lo, hi, vs, ok := shooterStats(s)
	if !ok || !s.alive || !s.onField {
		return nil
	}
	type cand struct {
		dist, hp int
		id       EntityID
	}
	var cs []cand
	for i := range b.ents {
		t := &b.ents[i]
		if !b.isCandidate(s, t) {
			continue
		}
		if s.owner == ownerDefender && (t.ud == nil || vs&ClassBit(t.ud.Class) == 0) {
			continue
		}
		d := footDist(s, t)
		if d < lo || d > hi || !lineOfFire(s.pos, t.pos) {
			continue
		}
		cs = append(cs, cand{d, t.hp, t.id})
	}
	sort.Slice(cs, func(i, j int) bool {
		a, c := cs[i], cs[j]
		if a.dist != c.dist {
			return a.dist < c.dist
		}
		if a.hp != c.hp {
			return a.hp < c.hp
		}
		return a.id < c.id
	})
	out := make([]EntityID, len(cs))
	for i, c := range cs {
		out[i] = c.id
	}
	return out
}
