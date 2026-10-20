package army

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Pick is a unit or spell id with a count: what the client selects for a raid and what
// the battle result reports as lost/used. The client never sends levels.
type Pick struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
}

// Formation is the server-built army of a raid: the levels are the ones in force when
// the army was formed and stay frozen for that raid (army-lab.md §6.2). The raid module
// maps it onto sim.Army / sim.Snapshot.
type Formation struct {
	Units  []Slot `json:"units"`
	Spells []Slot `json:"spells"`
}

// Outcome is the part of sim.Result the army needs (army-lab.md §6.3). The raid module
// reports lost units and used spells; survivors and unused spells return to the stock.
type Outcome struct {
	UnitsLost  []Pick
	SpellsUsed []Pick
}

// FormArmy moves the picked units and spells from the stock into the escrow of raidID,
// inside the caller's transaction (the one that creates the RaidTicket). It checks
// army-lab.md §6.1 items 1-4: stock, group multiples, pool capacity and ships only against
// water (hasWater). The escrow keeps occupying housing. Repeating the call for the same
// raidID returns the escrow that already exists without taking anything again. Hero
// checks belong to the raid module. expires is the ticket's expires_at.
func (s *Service) FormArmy(ctx context.Context, tx pgx.Tx, player uuid.UUID, raidID string, units, spells []Pick, hasWater bool, expires time.Time) (Formation, error) {
	w, err := s.load(ctx, tx, player, s.cfg.Now())
	if err != nil {
		return Formation{}, err
	}
	for _, d := range w.deployed {
		if d.RaidID == raidID {
			return Formation{Units: nonNil(d.Units), Spells: nonNil(d.Spells)}, nil
		}
	}
	var f Formation
	if f.Units, err = w.pick(units, false, hasWater); err != nil {
		return Formation{}, err
	}
	if f.Spells, err = w.pick(spells, true, hasWater); err != nil {
		return Formation{}, err
	}
	for _, sl := range append(append([]Slot(nil), f.Units...), f.Spells...) {
		w.stock[sl.ID] -= sl.Count
	}
	w.deployed = append(w.deployed, deployment{RaidID: raidID, Expires: expires, Units: f.Units, Spells: f.Spells})
	return f, s.save(ctx, tx, player, w)
}

// pick validates one selection against the stock and turns it into level-stamped slots.
func (w *world) pick(ps []Pick, spells, hasWater bool) ([]Slot, error) {
	merged := map[string]int{}
	var order []string
	for _, p := range ps {
		u, ok := w.c.units[p.ID]
		if !ok || u.Spell != spells || p.Count < 1 {
			return nil, fail("invalid", "bad_selection", "unknown %q or non-positive count", p.ID)
		}
		if _, seen := merged[p.ID]; !seen {
			order = append(order, p.ID)
		}
		merged[p.ID] += p.Count
	}
	out := make([]Slot, 0, len(order))
	for _, id := range order {
		u, n := w.c.units[id], merged[id]
		switch {
		case n > w.stock[id]:
			return nil, fail("conflict", "no_stock", "only %d of %s at home", w.stock[id], id).with("id", id).with("have", w.stock[id])
		case u.Group > 1 && n%u.Group != 0:
			return nil, fail("invalid", "not_multiple_of_group", "%s goes in multiples of %d", id, u.Group)
		case u.Class == "SEA" && !hasWater:
			return nil, fail("invalid", "ship_no_water", "ships need a target with water")
		}
		out = append(out, Slot{ID: id, Level: w.level(id), Count: n})
	}
	return out, nil
}

// Resolve closes the escrow of raidID in the caller's transaction (the one that
// finishes the raid): survivors and unused spells return to the stock, lost units
// disappear. It is idempotent per raidID — a second call finds no escrow and returns
// false. Release (a ticket that expired without finish) is Resolve with an empty Outcome.
func (s *Service) Resolve(ctx context.Context, tx pgx.Tx, player uuid.UUID, raidID string, out Outcome) (bool, error) {
	w, err := s.load(ctx, tx, player, s.cfg.Now())
	if err != nil {
		return false, err
	}
	i := -1
	for j, d := range w.deployed {
		if d.RaidID == raidID {
			i = j
		}
	}
	if i < 0 {
		return false, s.save(ctx, tx, player, w)
	}
	d := w.deployed[i]
	for _, g := range []struct {
		slots []Slot
		gone  []Pick
	}{{d.Units, out.UnitsLost}, {d.Spells, out.SpellsUsed}} {
		gone := map[string]int{}
		for _, p := range g.gone {
			gone[p.ID] += p.Count
		}
		for _, sl := range g.slots {
			if gone[sl.ID] < 0 || gone[sl.ID] > sl.Count {
				return false, fail("invalid", "bad_outcome", "%s: %d lost of %d sent", sl.ID, gone[sl.ID], sl.Count)
			}
			w.stock[sl.ID] += sl.Count - gone[sl.ID]
			delete(gone, sl.ID)
		}
		if len(gone) > 0 {
			return false, fail("invalid", "bad_outcome", "outcome names units that were not sent")
		}
	}
	w.deployed = append(w.deployed[:i], w.deployed[i+1:]...)
	return true, s.save(ctx, tx, player, w)
}

// Release returns the whole escrow of raidID to the stock (army-lab.md §6.4, OQ-ARMY-7).
func (s *Service) Release(ctx context.Context, tx pgx.Tx, player uuid.UUID, raidID string) (bool, error) {
	return s.Resolve(ctx, tx, player, raidID, Outcome{})
}
