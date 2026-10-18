// Package sim is the deterministic battle core. NO I/O, NO time.Now, NO map iteration order dependence,
// NO floats: positions are fixed-point (1 tile = 1000 units). Same (Snapshot, Commands, Seed, ContentVersion)
// MUST give byte-identical Result on every platform (server validation + client WASM).
package sim

const (
	TickMs     = 100           // one tick = 100 ms of battle time
	MaxTicks   = 1800          // 180 s hard limit (+30 s deploy-only prelude is not counted)
	TileUnits  = 1000          // fixed-point scale
	StarHQ     = 1
	StarHalf   = 2             // >=50% destruction
	StarFull   = 3             // 100% destruction
)

type Vec struct{ X, Y int32 } // fixed-point, TileUnits per tile

type EntityKind uint8

const (
	KindBuilding EntityKind = iota
	KindDefence
	KindWall
	KindTroop
	KindVehicle
	KindShip
	KindHero
)

// Snapshot is the frozen defender base (what the server stored) + attacker army loadout.
type Snapshot struct {
	ContentVersion string
	BaseID         string
	Buildings      []PlacedBuilding
	Garrison       []GarrisonUnit
	Army           []ArmySlot
	Spells         []ArmySlot
	Hero           string // "" if none
}

type PlacedBuilding struct {
	ID    string // content id, e.g. d_mg_nest
	Level int
	Pos   Vec // top-left tile * TileUnits
}
type GarrisonUnit struct{ ID string; Level int }
type ArmySlot struct{ ID string; Level, Count int }

// Command is the ONLY input during a battle. Log of commands + seed = full replay.
type Command struct {
	Tick int
	Op   Op
	ID   string // unit/spell/ability id
	Pos  Vec
}
type Op uint8

const (
	OpDeploy Op = iota
	OpCast
	OpHeroAbility
	OpRetreat
)

type Result struct {
	Stars       int
	Destruction int // 0..100
	HQDestroyed bool
	TicksUsed   int
	LootBasis   LootBasis // what was destroyed, for the economy module to turn into loot
	StateHash   uint64    // hash of final world, compared server vs client
}
type LootBasis struct{ DestroyedStorages map[string]int; PercentDestroyed int }
