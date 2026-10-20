package raid

import (
	"errors"
	"fmt"

	"github.com/OWNER/fireline/sim"
	"github.com/geone39/fireline/server/svc/base"
)

// ErrBadInput is returned by the snapshot builders for a layout or loadout the rules cannot describe.
var ErrBadInput = errors.New("raid: bad snapshot input")

// SnapshotInput is what raids/start knows: the defender's frozen defence snapshot (base layout) and the
// attacker's loadout. Game numbers are not here: they come from Content.Rules.
type SnapshotInput struct {
	BaseID    string
	Map       string // content/base_grid.yaml maps.<id>, required
	Layout    base.Layout
	Garrison  []sim.GarrisonUnit // defender infantry sitting in the bunkers (defence barracks)
	Army      []sim.ArmySlot
	Spells    []sim.ArmySlot
	Hero      string
	HeroLevel int
	// Revealed lists the UIDs of hidden traps the attacker's radar has uncovered (OQ-7.8). Only the client
	// snapshot looks at it; the full one carries every trap, revealed or not (Revealed flag set accordingly).
	Revealed map[string]bool
}

// FullSnapshot is the authoritative snapshot (every hidden trap included) the server plays Verify on.
func (c *Content) FullSnapshot(in SnapshotInput) (sim.Snapshot, error) { return c.snapshot(in, false) }

// ClientSnapshot is the view sent in raids/start: hidden traps are left out unless revealed by a radar (OQ-7.8),
// revealed ones are included as ordinary buildings with Revealed set. Its StateHash will not match the server's.
func (c *Content) ClientSnapshot(in SnapshotInput) (sim.Snapshot, error) { return c.snapshot(in, true) }

func (c *Content) snapshot(in SnapshotInput, client bool) (sim.Snapshot, error) {
	bad := func(format string, a ...any) (sim.Snapshot, error) {
		return sim.Snapshot{}, fmt.Errorf("%w: %s", ErrBadInput, fmt.Sprintf(format, a...))
	}
	defs := make(map[string]*sim.BuildingDef, len(c.Rules.Buildings))
	for i := range c.Rules.Buildings {
		defs[c.Rules.Buildings[i].ID] = &c.Rules.Buildings[i]
	}
	units := make(map[string]*sim.UnitDef, len(c.Rules.Units))
	for i := range c.Rules.Units {
		units[c.Rules.Units[i].ID] = &c.Rules.Units[i]
	}
	spells := make(map[string]bool, len(c.Rules.Spells))
	for _, s := range c.Rules.Spells {
		spells[s.ID] = true
	}
	mapID := in.Map
	if mapID == "" {
		return bad("map is empty")
	}
	known := false
	for _, m := range c.Rules.Maps {
		known = known || m.ID == mapID
	}
	if !known {
		return bad("unknown map %q", mapID)
	}

	snap := sim.Snapshot{ContentVersion: c.Version, BaseID: in.BaseID, Map: mapID, Hero: in.Hero, HeroLevel: in.HeroLevel, Rules: c.Rules}
	for _, it := range in.Layout.Items {
		d, ok := defs[it.Type]
		if !ok {
			return bad("item %s: unknown type %q", it.UID, it.Type)
		}
		if it.Level < 1 {
			return bad("item %s: level %d", it.UID, it.Level)
		}
		if it.Rot%2 != 0 && d.W != d.H {
			return bad("item %s: rotated non-square %q is not representable (PlacedBuilding has no rotation)", it.UID, it.Type)
		}
		pb := sim.PlacedBuilding{ID: it.Type, Level: it.Level, Pos: sim.Cell{X: int16(it.X), Y: int16(it.Y)}}
		if d.Hidden {
			pb.Revealed = in.Revealed[it.UID]
			if client && !pb.Revealed {
				continue
			}
		}
		snap.Buildings = append(snap.Buildings, pb)
	}
	for _, g := range in.Garrison {
		if u, ok := units[g.ID]; !ok || u.Hero || u.Class != sim.ClassINF {
			return bad("garrison unit %q is not infantry", g.ID)
		}
		snap.Garrison = append(snap.Garrison, g)
	}
	for _, a := range in.Army {
		if _, ok := units[a.ID]; !ok || a.Count < 1 || a.Level < 1 {
			return bad("army slot %q level %d count %d", a.ID, a.Level, a.Count)
		}
		snap.Army = append(snap.Army, a)
	}
	for _, s := range in.Spells {
		if !spells[s.ID] || s.Count < 1 || s.Level < 1 {
			return bad("spell slot %q level %d count %d", s.ID, s.Level, s.Count)
		}
		snap.Spells = append(snap.Spells, s)
	}
	if in.Hero != "" {
		if u, ok := units[in.Hero]; !ok || !u.Hero || in.HeroLevel < 1 {
			return bad("hero %q level %d", in.Hero, in.HeroLevel)
		}
	}
	return snap, nil
}
