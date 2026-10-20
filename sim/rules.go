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
	// Support (SIM-5, contract 0.2.4, additive; zero = "none").
	DmgGrowthPermille int        // per-level growth of Dmg (spec 3.2 Mdps), 0 = none
	Heal              int        // ability heal: hit points restored per action (u_medic, v_engineer); needs RangeCells >= 1
	HealVs            uint8      // mask of ClassBit the heal works on (medic INF, engineer VEH)
	JamTurns          int        // ability jam (u_saboteur): turns(jam_s), the target stays disabled this many turns
	Ability           AbilityDef // hero ability (spec 5.3), Kind 0 = none; heroes only
	AuraCells         int        // hero aura (h_commander): allies within this many cells get the bonuses below
	AuraDmgPermille   int
	AuraMovePermille  int
}

// AbilityKind is the effect of a hero ability (content heroes.yaml ability.effect.kind, spec 5.3).
type AbilityKind uint8

const (
	AbilityNone       AbilityKind = iota
	AbilityZoneDamage             // zone_damage: an airstrike around the hero (a_cover_fire)
	AbilityDecoy                  // spawn_decoy: a decoy unit the defence treats as a target (a_decoy_turret)
	AbilityStealth                // stealth: untargetable for the defence until the hero attacks (a_vanish)
)

// AbilityDef is the hero ability. The effect-specific numbers are per kind; unused ones stay zero.
type AbilityDef struct {
	ID            string
	Kind          AbilityKind
	CooldownTurns int    // turns(cooldown_s), >= 1
	RadiusCells   int    // zone_damage: blast radius; spawn_decoy: how far from the hero the decoy may stand
	DurationTurns int    // zone_damage: steps; stealth: turns; spawn_decoy: lifetime, 0 = until destroyed
	Dmg           int    // zone_damage: damage over the whole duration
	DecoyUnit     string // spawn_decoy: Units id of the decoy (a stationary ground unit; its numbers are the decoy's)
	BonusPermille int    // stealth: multiplier of the first shot out of hiding (4000 = x4), 0 = none
	BonusVs       VsTag  // stealth: the bonus applies to targets of this group
}

// SpellKind is the effect of a support call (spec 9.1).
type SpellKind uint8

const (
	SpellAirstrike  SpellKind = iota + 1 // c_airstrike
	SpellMedkit                          // c_medkit
	SpellSmoke                           // c_smoke
	SpellAdrenaline                      // c_adrenaline
	SpellEMP                             // c_emp
	SpellBreach                          // c_breach
)

// SpellDef is a support call from the Support Bay in per-turn integers (content troops.yaml spells).
type SpellDef struct {
	ID            string
	Kind          SpellKind
	RadiusCells   int
	DurationTurns int // turns(duration_s): airstrike, medkit, smoke, adrenaline, emp
	DmgTotal      int // airstrike: damage to each enemy over the whole duration
	HealTotal     int // medkit: hit points to each infantry unit over the whole duration
	MovePermille  int // adrenaline: bonus to move_cells (floor, at least +1 cell)
	DmgPermille   int // adrenaline: bonus to damage
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
	// Support (SIM-5, contract 0.2.4, additive; zero = "none").
	// Level growth (SIM-8, contract 0.2.6, additive; zero = "none").
	DmgGrowthPermille int // per-level growth of Dmg (defences.yaml level_growth.dps_permille), 0 = none; Burst/DotDmg do not grow
	AuraCells int // d_jammer: a spell centred within this many cells of the footprint is weakened
	JamPct    int // d_jammer: airstrike damage / EMP duration lose this percent (0..100)
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
	VsHQ // b_hq (SIM-5: the stealth bonus of a_vanish)
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
	SpellsPerTurn     int // battle.spells_per_turn (SIM-5): spells per player turn, 0 = no limit
	Spells            []SpellDef
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
			if m.Tag < VsWire || m.Tag > VsHQ || m.Permille < 0 {
				return bad("unit %q: bad vs_mult", u.ID)
			}
		}
		if u.DmgGrowthPermille < 0 || u.Heal < 0 || u.JamTurns < 0 || u.AuraCells < 0 || u.AuraDmgPermille < 0 || u.AuraMovePermille < 0 {
			return bad("unit %q has invalid support numbers", u.ID)
		}
		if u.Heal > 0 && (u.RangeCells < 1 || u.HealVs == 0) {
			return bad("unit %q: heal needs range >= 1 and heal_vs", u.ID)
		}
		if (u.AuraCells > 0) != (u.AuraDmgPermille+u.AuraMovePermille > 0) {
			return bad("unit %q: aura needs aura_cells and a bonus", u.ID)
		}
	}
	for i := range r.Units {
		if err := r.validateAbility(&r.Units[i]); err != nil {
			return err
		}
	}
	if r.SpellsPerTurn < 0 {
		return bad("spells_per_turn < 0")
	}
	for _, sp := range r.Spells {
		if seen["c:"+sp.ID] {
			return bad("duplicate spell %q", sp.ID)
		}
		seen["c:"+sp.ID] = true
		if sp.RadiusCells < 0 || sp.DurationTurns < 0 || sp.DmgTotal < 0 || sp.HealTotal < 0 || sp.MovePermille < 0 || sp.DmgPermille < 0 {
			return bad("spell %q has invalid numbers", sp.ID)
		}
		ok := false
		switch sp.Kind {
		case SpellAirstrike:
			ok = sp.DurationTurns >= 1 && sp.DmgTotal >= 1
		case SpellMedkit:
			ok = sp.DurationTurns >= 1 && sp.HealTotal >= 1
		case SpellSmoke, SpellEMP:
			ok = sp.DurationTurns >= 1
		case SpellAdrenaline:
			ok = sp.DurationTurns >= 1 && sp.MovePermille+sp.DmgPermille > 0
		case SpellBreach:
			ok = true
		}
		if !ok {
			return bad("spell %q (kind %d) lacks its effect numbers", sp.ID, sp.Kind)
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
		if d.DmgGrowthPermille < 0 || d.AuraCells < 0 || d.JamPct < 0 || d.JamPct > 100 {
			return bad("building %q has invalid jammer numbers", d.ID)
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

// validateAbility checks the hero ability of u against the rest of the rules.
func (r *Rules) validateAbility(u *UnitDef) error {
	a := &u.Ability
	bad := func(format string, x ...any) error {
		return fmt.Errorf("%w: rules: unit %q ability: %s", ErrBadSnapshot, u.ID, fmt.Sprintf(format, x...))
	}
	if a.Kind == AbilityNone {
		if *a != (AbilityDef{}) {
			return bad("numbers without a kind")
		}
		return nil
	}
	if !u.Hero {
		return bad("only heroes have abilities")
	}
	if a.CooldownTurns < 1 || a.RadiusCells < 0 || a.DurationTurns < 0 || a.Dmg < 0 || a.BonusPermille < 0 {
		return bad("invalid numbers")
	}
	switch a.Kind {
	case AbilityZoneDamage:
		if a.Dmg < 1 || a.DurationTurns < 1 {
			return bad("zone_damage needs dmg and duration")
		}
	case AbilityDecoy:
		ok := false
		for i := range r.Units {
			if d := &r.Units[i]; d.ID == a.DecoyUnit && !d.Hero && d.Class != ClassAIR && d.Class != ClassSEA {
				ok = true
			}
		}
		if !ok {
			return bad("spawn_decoy needs a ground decoy unit, got %q", a.DecoyUnit)
		}
	case AbilityStealth:
		if a.DurationTurns < 1 || (a.BonusPermille > 0 && (a.BonusVs < VsWire || a.BonusVs > VsHQ)) {
			return bad("stealth needs duration and a valid bonus target")
		}
	default:
		return bad("kind %d", a.Kind)
	}
	return nil
}
