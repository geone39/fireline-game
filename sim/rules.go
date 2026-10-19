package sim

import (
	"errors"
	"fmt"
)

// ErrBadSnapshot is returned by NewBattle for a Snapshot/Rules pair the engine cannot play (unknown content id,
// building outside the build zone, overlapping footprints, missing rules...). Wrapped with details.
var ErrBadSnapshot = errors.New("sim: bad snapshot")

// UnitClass is the movement/target layer of a unit. Ordinal values are used as bit numbers in Vs masks.
type UnitClass uint8

const (
	ClassINF UnitClass = iota
	ClassVEH
	ClassAIR
	ClassSEA
)

// ClassBit is the Vs-mask bit of a class.
func ClassBit(c UnitClass) uint8 { return 1 << c }

// Rect is a tile rectangle (X, Y = top-left).
type Rect struct{ X, Y, W, H int }

// Contains reports whether the cell lies inside the rectangle.
func (r Rect) Contains(c Cell) bool {
	return int(c.X) >= r.X && int(c.X) < r.X+r.W && int(c.Y) >= r.Y && int(c.Y) < r.Y+r.H
}

// MapDef is a content/base_grid.yaml map: id + water rectangles.
type MapDef struct {
	ID    string
	Water []Rect
}

// BlockKind says whom a building physically blocks (content `blocks`).
type BlockKind uint8

const (
	BlockAll      BlockKind = iota // blocks every ground unit (buildings, walls, decor); the zero value is the safe one
	BlockNone                      // d_wire, hidden traps
	BlockVehicles                  // d_tank_trap
)

// UnitDef is the per-turn integer form of a troop/vehicle/ship/hero (spec 3.2). The CALLER derives it from
// content (the loader is not part of sim: OQ-SIM3); sim only reads the numbers.
type UnitDef struct {
	ID               string
	Class            UnitClass
	Hero             bool
	HP               int // level-1 max hp
	HPGrowthPermille int // per-level growth, 0 = none (M[n] = floor(M[n-1]*g/1000))
	AP               int // actions per turn, >= 1
	MoveCells        int // 0 = stationary
	RangeCells       int // effective range, >= 1 when Dmg > 0 (melee = 1)
	MinRangeCells    int
	Dmg              int // 0 = cannot attack (unless Burst > 0)
	SplashCells      int // splash radius of an attack (0 = single target)
	Burst            int // one-shot detonation (u_breacher): replaces Dmg, the unit is destroyed after it
	WallMult         int // Burst multiplier against walls / wire (content wall_mult), 0 = 1
	VsMult           []VsMult
	Carries          int // passenger slots (INF only)
	Group            int // units per Deploy, 0/1 = single (u_paratrooper = 4)
	Paradrop         bool
}

// BuildingDef is a defender structure: building, defence, wall, trap, decor, obstacle.
type BuildingDef struct {
	ID               string
	Kind             EntityKind // KindBuilding, KindDefence or KindWall
	W, H             int
	HP               int
	HPGrowthPermille int
	Block            BlockKind
	SlowPct          int  // ground INF crossing cost, content `slow_pct` (0 = none)
	Counted          bool // counts for Destruction%
	Weight           int  // destruction_weight, 0 = 1
	HQ               bool
	Storage          bool // b_vault / b_fuel_tank / b_alloy_depot: reported in LootBasis
	Hidden           bool // d_mine / d_spring_trap: invisible and untargetable until Revealed
	Untargetable     bool // decor, obstacles
	Garrison         int  // bunker capacity
	RangeCells       int  // defence attack; 0 with Dmg == 0 = passive
	MinRangeCells    int
	Dmg              int
	Vs               uint8 // mask of ClassBit the defence can shoot; for traps: classes that trigger it
	SplashCells      int   // blast radius (d_mine)
	Trap             TrapKind
	Burst            int // d_mine blast damage at the centre
	MaxHPFlung       int // d_spring_trap: heaviest INF (max_hp) it throws
	FlingCells       int // d_spring_trap: throw distance
	// Defence phase (SIM-3, contract 0.2.3, additive; zero = "none").
	TriggerCells      int // d_bunker: an attacker within this many cells of the footprint releases the garrison (0 = the longest range among the garrison units, spec 7.2)
	DotDmg            int // d_wire: damage per defence phase to every attacker INF unit standing on it (dot_dps x turn_s, whole)
	OverheatFireTurns int // d_flame_trap: consecutive turns of fire before it overheats (0 = never)
	CoolTurns         int // d_flame_trap: turns without fire after overheating
	Chain             int // d_tesla: the arc hits up to Chain targets (<= 1 = single target)
	ChainCells        int // d_tesla: reach of each jump to the next target
}

// VsTag is a target group for a unit's damage multiplier (content `vs_mult`, spec 6.1).
type VsTag uint8

const (
	VsWire     VsTag = iota + 1 // slowing passive obstacle (d_wire)
	VsWall                      // anything that blocks movement among passive obstacles (walls, fences, tank traps)
	VsBuilding                  // KindBuilding
	VsDefence                   // KindDefence
	VsINF
	VsVEH
	VsAIR
	VsSEA
)

// VsMult scales the damage against a target group: Permille 1500 = x1.5. Several matching entries add up
// their (Permille-1000) bonuses (spec 6.1: bonuses are summed, then one floor).
type VsMult struct {
	Tag      VsTag
	Permille int
}

// TrapKind marks a trap that triggers when a unit steps on it (spec 8.4).
type TrapKind uint8

const (
	TrapNone   TrapKind = iota
	TrapMine            // d_mine: blast Burst + splash, consumed
	TrapSpring          // d_spring_trap: throws a light INF unit backwards, consumed
)

// Rules is the content-derived constant set of a battle. It is carried inside the Snapshot, is read-only for
// the engine and must not be mutated while a Battle uses it.
type Rules struct {
	GridW, GridH      int
	BuildZone         Rect
	Maps              []MapDef
	MaxTurns          int // battle.max_turns safety fuse
	ParadropEdgeCells int // battle.paradrop_edge_cells
	SplashFalloffPct  int // battle.splash_falloff_pct (0..100): damage loss at the splash edge
	Units             []UnitDef
	Buildings         []BuildingDef
}

func (r *Rules) validate() error {
	bad := func(format string, a ...any) error {
		return fmt.Errorf("%w: rules: %s", ErrBadSnapshot, fmt.Sprintf(format, a...))
	}
	if r.GridW <= 0 || r.GridH <= 0 || r.GridW > 32767 || r.GridH > 32767 {
		return bad("grid %dx%d", r.GridW, r.GridH)
	}
	if r.MaxTurns <= 0 {
		return bad("max_turns must be > 0")
	}
	if r.ParadropEdgeCells < 0 {
		return bad("paradrop_edge_cells < 0")
	}
	if r.SplashFalloffPct < 0 || r.SplashFalloffPct > 100 {
		return bad("splash_falloff_pct %d", r.SplashFalloffPct)
	}
	seen := map[string]bool{}
	for _, m := range r.Maps {
		if seen["m:"+m.ID] {
			return bad("duplicate map %q", m.ID)
		}
		seen["m:"+m.ID] = true
	}
	for _, u := range r.Units {
		if seen["u:"+u.ID] || seen["b:"+u.ID] {
			return bad("duplicate id %q", u.ID)
		}
		seen["u:"+u.ID] = true
		if u.HP <= 0 || u.AP < 1 || u.MoveCells < 0 || u.RangeCells < 0 || u.MinRangeCells < 0 || u.Dmg < 0 || u.Carries < 0 || u.Group < 0 {
			return bad("unit %q has invalid numbers", u.ID)
		}
		if u.SplashCells < 0 || u.Burst < 0 || u.WallMult < 0 {
			return bad("unit %q has invalid attack numbers", u.ID)
		}
		if (u.Dmg > 0 || u.Burst > 0) && u.RangeCells < 1 {
			return bad("unit %q: attack needs range >= 1", u.ID)
		}
		for _, m := range u.VsMult {
			if m.Tag < VsWire || m.Tag > VsSEA || m.Permille < 0 {
				return bad("unit %q: bad vs_mult", u.ID)
			}
		}
	}
	for _, d := range r.Buildings {
		if seen["u:"+d.ID] || seen["b:"+d.ID] {
			return bad("duplicate id %q", d.ID)
		}
		seen["b:"+d.ID] = true
		if d.W <= 0 || d.H <= 0 || d.HP <= 0 || d.SlowPct < 0 || d.SlowPct >= 100 || d.Weight < 0 || d.Garrison < 0 {
			return bad("building %q has invalid numbers", d.ID)
		}
		if d.Kind != KindBuilding && d.Kind != KindDefence && d.Kind != KindWall {
			return bad("building %q has kind %d", d.ID, d.Kind)
		}
		if d.Dmg > 0 && d.RangeCells < 1 {
			return bad("building %q: attack needs range >= 1", d.ID)
		}
		if d.SplashCells < 0 || d.Burst < 0 || d.MaxHPFlung < 0 || d.FlingCells < 0 {
			return bad("building %q has invalid trap numbers", d.ID)
		}
		if d.TriggerCells < 0 || d.DotDmg < 0 || d.OverheatFireTurns < 0 || d.CoolTurns < 0 || d.Chain < 0 || d.ChainCells < 0 {
			return bad("building %q has invalid defence numbers", d.ID)
		}
		if d.OverheatFireTurns > 0 && d.CoolTurns < 1 {
			return bad("building %q: overheat needs cool_turns", d.ID)
		}
		if d.Chain > 1 && d.ChainCells < 1 {
			return bad("building %q: chain needs chain_cells", d.ID)
		}
		switch d.Trap {
		case TrapNone:
		case TrapMine:
			if d.Burst < 1 || d.Vs == 0 {
				return bad("mine %q needs burst and vs", d.ID)
			}
		case TrapSpring:
			if d.MaxHPFlung < 1 || d.FlingCells < 1 || d.Vs == 0 {
				return bad("spring trap %q needs max_hp_flung, fling_cells and vs", d.ID)
			}
		default:
			return bad("building %q has trap kind %d", d.ID, d.Trap)
		}
	}
	return nil
}

// scaledHP is max_hp(L) = floor(hp * M[L] / 1000), M[1] = 1000, M[n] = floor(M[n-1] * g / 1000) (spec 3.2).
func scaledHP(hp, growthPermille, level int) int {
	if growthPermille <= 0 {
		growthPermille = 1000
	}
	m := 1000
	for l := 2; l <= level; l++ {
		m = m * growthPermille / 1000
	}
	return hp * m / 1000
}
