// Package sim is the deterministic turn-based battle core (contract 0.2, ADR-003).
// NO I/O, NO time, NO math/rand, NO floats, NO map iteration in logic: the world is an integer tile grid
// and time is counted in turns. Same (Snapshot, Commands, Seed, ContentVersion) MUST give a byte-identical
// Result on every platform (server validation + client WASM). Rules: docs/design/battle-sim.md.
//
// Implemented (SIM-1..5): world, deployment, Move, pathfinding, target selection, Attack with damage /
// splash / destruction, hidden traps, stars and loot basis, the defence phase (bunker release, wire, building
// fire with overheat and chains, garrison AI), and the support layer: Ability (heal, jam, unload, hero abilities),
// Cast (airstrike, medkit, smoke, adrenaline, EMP, breach with zone effects counted in turns), the hero aura.
package sim

// EngineVersion identifies the rule set. Any rule change bumps it and regenerates the golden replays.
const EngineVersion = "0.2.7"

// Game numbers (turn limit, time limit, splash falloff, ...) live in content/battle.yaml and are passed in by
// the caller; there are deliberately no such constants here (AGENTS.md rule 3).

// Cell is a tile of the battle grid. Origin (0,0) is top-left, X grows right, Y grows down.
type Cell struct{ X, Y int16 }

// EntityID identifies a battle entity. Starts at 1; 0 means "none". Assignment order is canonical (spec 2.4).
type EntityID uint32

// EntityKind is the entity class.
type EntityKind uint8

const (
	KindBuilding EntityKind = iota
	KindDefence
	KindWall
	KindTroop
	KindVehicle
	KindShip
	KindHero
	KindDecoy // a unit spawned by a hero ability (spawn_decoy): a target for the defence, never counted as army
)

// Snapshot is the frozen defender base (what the server stored) + attacker army loadout.
// Order of Buildings/Garrison does not matter: the engine normalises it (spec 2.4).
// The client receives a view WITHOUT hidden traps (OQ-7.8); the server recomputes with the full one.
type Snapshot struct {
	ContentVersion string
	BaseID         string
	Map            string // content/base_grid.yaml maps.<id> (water layout)
	Buildings      []PlacedBuilding
	Garrison       []GarrisonUnit
	Army           []ArmySlot
	Spells         []ArmySlot
	Hero           string // "" if none
	HeroLevel      int
	// Rules are the content-derived numbers (per-turn integers, spec 3.2) the caller loads from content/*.yaml;
	// sim does no I/O (OQ-SIM3). Additive field of contract 0.2.1.
	Rules Rules
}

// PlacedBuilding is a defender building; Pos is its top-left tile.
type PlacedBuilding struct {
	ID       string // content id, e.g. d_mg_nest
	Level    int
	Pos      Cell
	Revealed bool // hidden traps (d_mine, d_spring_trap) revealed by a radar; ignored for other buildings
}

type GarrisonUnit struct {
	ID    string
	Level int
}

type ArmySlot struct {
	ID    string
	Level int
	Count int
}

// Op is the command type.
type Op uint8

const (
	OpDeploy  Op = iota // phase 0: put a unit from an Army/Hero slot at To
	OpLoad              // phase 0: Unit (passenger) boards Target (carrier)
	OpStart             // phase 0 -> turn 1
	OpMove              // Unit walks Path
	OpAttack            // Unit attacks Target
	OpAbility           // Unit uses ability ID on Target / To
	OpCast              // spell ID at To
	OpEndTurn           // end of player phase, defence acts
	OpRetreat           // end the battle
)

// Command is the ONLY input during a battle. Log of commands + seed = full replay.
// Turn is 0 for deployment and 1..N afterwards; it never decreases within a log.
type Command struct {
	Turn   int
	Op     Op
	Unit   EntityID // actor (Move, Attack, Ability); passenger for Load
	Target EntityID // Attack, Ability(heal/jam), Load(carrier)
	ID     string   // Deploy: slot id; Cast: spell id; Ability: kind
	Level  int      // Deploy
	To     Cell     // Deploy, Cast, Ability when needed
	Path   []Cell   // Move: cells after the start cell, 8-neighbour steps
}

// EndReason says why the battle ended.
type EndReason uint8

const (
	EndNone            EndReason = iota
	EndFullDestruction           // every counted building destroyed
	EndNoUnits                   // no live attacker units left
	EndTurnLimit                 // battle.max_turns safety fuse
	EndRetreat                   // Retreat command (also server time limit: server appends Retreat)
	EndLogEnd                    // log exhausted without an ending; the server rejects such client results
)

// Star flags in Result.StarFlags.
const (
	StarHQ   uint8 = 1 << 0
	StarHalf uint8 = 1 << 1 // 2*done >= total
	StarFull uint8 = 1 << 2 // done == total
)

// RejectCode explains why a command was refused. A refused command never changes the state.
type RejectCode uint8

const (
	RejectBadPhase RejectCode = iota + 1
	RejectBadTurn
	RejectUnknownEntity
	RejectNotOwner
	RejectNoAP
	RejectCooldown
	RejectBadPath
	RejectPathCost
	RejectOutOfZone
	RejectCellOccupied
	RejectOutOfRange
	RejectBelowMinRange
	RejectInvalidTarget
	RejectCapacity
	RejectNoStock
	RejectHeroLimit
	RejectBattleOver
)

// Rejection is one refused command (Index into the command log).
type Rejection struct {
	Index int
	Code  RejectCode
}

// SlotCount is a counted army/spell slot (used for losses and spent spells).
type SlotCount struct {
	ID    string
	Level int
	Count int
}

// StorageHit reports damage to one storage building for the loot calculation.
type StorageHit struct {
	Entity         EntityID
	ContentID      string
	Level          int
	DamagePermille int // 0..1000, 1000 if destroyed
}

// LootBasis is what economy needs to turn a battle into loot. No maps: lists are sorted.
type LootBasis struct {
	Storages         []StorageHit // sorted by Entity
	PercentDestroyed int
	Stars            int
}

// Result is the outcome of a battle.
type Result struct {
	Stars           int
	Destruction     int   // 0..100
	StarFlags       uint8 // StarHQ|StarHalf|StarFull
	HQDestroyed     bool
	TurnsUsed       int // turns started by the player; 0 on retreat in phase 0
	EndReason       EndReason
	LootBasis       LootBasis
	UnitsLost       []SlotCount // sorted by (ID, Level)
	SpellsUsed      []SlotCount // sorted by (ID, Level)
	HeroDamageTaken int
	HeroSurvived    bool
	Rejected        []Rejection
	EngineVersion   string
	StateHash       uint64 // hash of the final world, compared server vs client
}

// EventKind is an animation event produced by Battle.Apply. Events are not part of StateHash.
type EventKind uint8

const (
	EventMoved EventKind = iota + 1
	EventAttacked
	EventKilled
	EventDestroyed
	EventTrapTriggered
	EventSpellCast
	EventEffectTick
	EventHealed
	EventReleased
	EventTurnEnded
	EventBattleEnded
	EventSpawned // a decoy appeared (SIM-5)
)

// Event is a state change the client animates.
type Event struct {
	Kind          EventKind
	Actor, Target EntityID
	Cell          Cell
	Value         int
}
