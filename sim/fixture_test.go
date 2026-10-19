package sim

// Test fixture rules: NOT balance (spec 12, F01). Numbers are chosen for the scenarios, not taken from content.
func fixtureRules() Rules {
	return Rules{
		GridW: 44, GridH: 44,
		BuildZone: Rect{2, 2, 40, 40},
		Maps: []MapDef{
			{ID: "plain"},
			{ID: "coastal", Water: []Rect{{40, 0, 4, 44}, {36, 28, 4, 12}, {34, 34, 2, 6}}},
		},
		MaxTurns: 30, ParadropEdgeCells: 3,
		Units: []UnitDef{
			{ID: "u_rifleman", Class: ClassINF, HP: 50, AP: 2, MoveCells: 3, RangeCells: 4, Dmg: 10},
			{ID: "u_brawler", Class: ClassINF, HP: 100, AP: 1, MoveCells: 4, RangeCells: 1, Dmg: 30},
			{ID: "u_sniper", Class: ClassINF, HP: 30, AP: 1, MoveCells: 2, RangeCells: 9, Dmg: 28},
			{ID: "u_medic", Class: ClassINF, HP: 60, AP: 1, MoveCells: 2},
			{ID: "u_para", Class: ClassINF, HP: 50, AP: 1, MoveCells: 2, RangeCells: 4, Dmg: 20, Group: 4, Paradrop: true},
			{ID: "v_apc", Class: ClassVEH, HP: 700, AP: 1, MoveCells: 3, RangeCells: 5, Dmg: 16, Carries: 2},
			{ID: "v_tank", Class: ClassVEH, HP: 1100, AP: 1, MoveCells: 2, RangeCells: 7, Dmg: 55},
			{ID: "u_drone", Class: ClassAIR, HP: 70, AP: 1, MoveCells: 5, RangeCells: 1, Dmg: 40},
			{ID: "s_gunboat", Class: ClassSEA, HP: 600, AP: 1, MoveCells: 3, RangeCells: 8, Dmg: 30},
			{ID: "s_lander", Class: ClassSEA, HP: 900, AP: 1, MoveCells: 3, Carries: 3},
			{ID: "v_turret", Class: ClassVEH, HP: 10, AP: 1, MoveCells: 0, RangeCells: 3, Dmg: 5}, // stationary
			{ID: "h_commander", Class: ClassINF, Hero: true, HP: 1600, AP: 1, MoveCells: 2, RangeCells: 5, Dmg: 80},
			{ID: "u_guard", Class: ClassINF, HP: 40, AP: 1, MoveCells: 2, RangeCells: 3, Dmg: 5},
		},
		Buildings: []BuildingDef{
			{ID: "b_hq", Kind: KindBuilding, W: 4, H: 4, HP: 800, HPGrowthPermille: 1220, Counted: true, HQ: true},
			{ID: "b_vault", Kind: KindBuilding, W: 3, H: 3, HP: 500, Counted: true, Storage: true},
			{ID: "d_mg_nest", Kind: KindDefence, W: 2, H: 2, HP: 60, Counted: true, RangeCells: 7, Dmg: 20, Vs: ClassBit(ClassINF) | ClassBit(ClassAIR)},
			{ID: "d_mortar", Kind: KindDefence, W: 3, H: 3, HP: 600, Counted: true, RangeCells: 10, MinRangeCells: 3, Dmg: 22, Vs: ClassBit(ClassINF)},
			{ID: "d_bunker", Kind: KindDefence, W: 3, H: 3, HP: 1400, Counted: true, Garrison: 2},
			{ID: "d_wall", Kind: KindWall, W: 1, H: 1, HP: 900, Block: BlockAll},
			{ID: "d_wire", Kind: KindWall, W: 1, H: 1, HP: 90, Block: BlockNone, SlowPct: 50},
			{ID: "d_tank_trap", Kind: KindWall, W: 1, H: 1, HP: 600, Block: BlockVehicles},
			{ID: "d_mine", Kind: KindWall, W: 1, H: 1, HP: 1, Block: BlockNone, Hidden: true},
			{ID: "x_statue", Kind: KindBuilding, W: 2, H: 2, HP: 1, Untargetable: true},
		},
	}
}

func pb(id string, x, y int) PlacedBuilding {
	return PlacedBuilding{ID: id, Level: 1, Pos: Cell{int16(x), int16(y)}}
}

// f01Snapshot is the spec 12 F01 base on the plain map: nest (12,10), HQ (20,10).
func f01Snapshot() Snapshot {
	return Snapshot{
		ContentVersion: "test-fixture", BaseID: "f01", Map: "plain",
		Buildings: []PlacedBuilding{pb("b_hq", 20, 10), pb("d_mg_nest", 12, 10)},
		Army:      []ArmySlot{{ID: "u_rifleman", Level: 1, Count: 5}, {ID: "u_brawler", Level: 1, Count: 3}},
		Rules:     fixtureRules(),
	}
}

func cell(x, y int) Cell { return Cell{int16(x), int16(y)} }
func path(xy ...int) []Cell {
	var p []Cell
	for i := 0; i+1 < len(xy); i += 2 {
		p = append(p, cell(xy[i], xy[i+1]))
	}
	return p
}

func dep(id string, x, y int) Command { return Command{Op: OpDeploy, ID: id, Level: 1, To: cell(x, y)} }
func mv(turn int, u EntityID, p ...int) Command {
	return Command{Turn: turn, Op: OpMove, Unit: u, Path: path(p...)}
}
func end(turn int) Command { return Command{Turn: turn, Op: OpEndTurn} }

// mustApply feeds commands to a battle and fails the test on engine errors.
func lastReject(b *Battle) RejectCode {
	if n := len(b.rejected); n > 0 {
		return b.rejected[n-1].Code
	}
	return 0
}
