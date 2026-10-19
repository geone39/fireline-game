package sim

import "testing"

// started deploys the given (id, x, y) units and starts the battle; ids are 1..len(Buildings) for the base,
// then the units in order.
type du struct {
	id   string
	x, y int
}

func started(t *testing.T, mut func(*Snapshot), units ...du) *Battle {
	t.Helper()
	b := newB(t, func(s *Snapshot) {
		s.Buildings = nil
		s.Army = []ArmySlot{{"u_rifleman", 1, 9}, {"u_brawler", 1, 9}, {"v_apc", 1, 9}, {"v_tank", 1, 9},
			{"u_drone", 1, 9}, {"v_turret", 1, 9}, {"s_gunboat", 1, 9}}
		if mut != nil {
			mut(s)
		}
	})
	for _, u := range units {
		applyOK(t, b, dep(u.id, u.x, u.y))
	}
	applyOK(t, b, Command{Op: OpStart})
	return b
}

func TestMoveBasics(t *testing.T) {
	b := started(t, func(s *Snapshot) { s.Buildings = []PlacedBuilding{pb("d_wall", 5, 5)} },
		du{"u_brawler", 1, 10}, du{"u_brawler", 1, 11}) // ids: wall=1, units 2, 3
	wantReject(t, b, mv(1, 99, 2, 10), RejectUnknownEntity)
	wantReject(t, b, mv(1, 1, 5, 6), RejectNotOwner)
	wantReject(t, b, mv(1, 2), RejectBadPath)                                     // empty
	wantReject(t, b, mv(1, 2, 3, 10), RejectBadPath)                              // not adjacent
	wantReject(t, b, mv(1, 2, 1, 10), RejectBadPath)                              // zero step: not a step
	wantReject(t, b, mv(1, 2, 2, 10, 4, 10), RejectBadPath)                       // gap
	wantReject(t, b, mv(1, 2, 2, 10, 3, 10, 4, 10, 5, 10, 6, 10), RejectPathCost) // 5 > move 4
	wantReject(t, b, mv(1, 2, 1, 11), RejectCellOccupied)                         // other unit
	wantReject(t, b, mv(1, 2, 0, 10, 0, 9, 0, 8, -1, 8), RejectBadPath)           // off map
	applyOK(t, b, mv(1, 2, 2, 10, 3, 10, 4, 10, 5, 10))                           // exactly move_cells
	if b.ent(2).pos != cell(5, 10) || b.unit[b.idx(cell(1, 10))] != 0 || b.ent(2).apLeft != 0 {
		t.Fatal("move bookkeeping")
	}
	wantReject(t, b, mv(1, 2, 6, 10), RejectNoAP)
	applyOK(t, b, mv(1, 3, 1, 12))
	applyOK(t, b, end(1))
	if b.ent(2).apLeft != 1 {
		t.Fatal("AP refresh at turn start")
	}
}

func TestMoveWallsAndStaircase(t *testing.T) {
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wall", 3, 11), pb("d_wall", 2, 12), pb("d_wall", 6, 5), pb("d_wall", 7, 6), pb("d_tank_trap", 4, 20)}
	}, du{"u_brawler", 1, 11}, du{"u_drone", 1, 12}, du{"v_tank", 1, 20}, du{"u_brawler", 1, 21})
	// walls (3,11) and (2,12): the diagonal (2,11)->(3,12) is a squeeze
	wantReject(t, b, mv(1, 6, 2, 11, 3, 12), RejectBadPath)
	wantReject(t, b, mv(1, 6, 2, 11, 3, 11), RejectBadPath) // through a wall
	applyOK(t, b, mv(1, 6, 2, 11))
	// one blocked orthogonal is fine: walls (6,5),(7,6): from (6,6)? orthogonals of (6,6)->(7,5) are (7,6) wall and (6,5) wall -> squeeze
	// the drone flies over everything
	applyOK(t, b, mv(1, 7, 2, 12, 3, 11, 4, 10, 5, 9))
	// tank trap blocks vehicles, not infantry
	wantReject(t, b, mv(1, 8, 2, 20, 3, 20, 4, 20), RejectBadPath)
	applyOK(t, b, mv(1, 9, 2, 21, 3, 20, 4, 20))
}

func TestMoveDiagonalWithOneBlockedSideIsLegal(t *testing.T) {
	b := started(t, func(s *Snapshot) { s.Buildings = []PlacedBuilding{pb("d_wall", 3, 11)} }, du{"u_brawler", 1, 11})
	applyOK(t, b, mv(1, 2, 2, 11, 3, 12)) // (3,11) wall but (2,12) free
}

func TestMoveWireCost(t *testing.T) {
	// ids: wire=1, mine=2, wire=3; units 4.. (infantry x2, tank)
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wire", 3, 12), pb("d_mine", 3, 13), pb("d_wire", 3, 15)}
	}, du{"u_rifleman", 1, 12}, du{"u_drone", 1, 13}, du{"v_tank", 1, 15})
	wantReject(t, b, mv(1, 4, 2, 12, 3, 12, 4, 12), RejectPathCost) // 1 + 2 + 1 = 4 > 3
	applyOK(t, b, mv(1, 4, 2, 12, 3, 12))                           // 1 + ceil(100/50) = 3
	applyOK(t, b, mv(1, 5, 2, 13, 3, 13, 4, 13))                    // a hidden mine neither blocks nor costs, and does not fire for air
	applyOK(t, b, mv(1, 6, 2, 15, 3, 15))                           // only infantry is slowed
}

func TestMoveStationaryAndShips(t *testing.T) {
	b := started(t, func(s *Snapshot) { s.Map = "coastal" }, du{"v_turret", 1, 10}, du{"s_gunboat", 42, 20})
	wantReject(t, b, mv(1, 1, 2, 10), RejectBadPath)                  // move_cells == 0
	wantReject(t, b, mv(1, 2, 41, 20, 40, 20, 39, 20), RejectBadPath) // 39 is land
	applyOK(t, b, mv(1, 2, 41, 20, 40, 20))                           // water only
}

func TestMoveLandUnitCannotEnterWater(t *testing.T) {
	b := started(t, func(s *Snapshot) { s.Map = "coastal" }, du{"u_rifleman", 1, 20})
	// walk to x=38, then step onto the water column at x=40 is not reachable in one go; place the unit by hand
	b.lift(b.ent(1))
	b.place(b.ent(1), cell(39, 20))
	wantReject(t, b, mv(1, 1, 40, 20), RejectBadPath)
}

func TestAirIgnoresStaircaseButNotOtherAir(t *testing.T) {
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wall", 3, 11), pb("d_wall", 2, 12)}
	}, du{"u_drone", 1, 11}, du{"u_drone", 1, 12})
	applyOK(t, b, mv(1, 3, 2, 11, 3, 12)) // squeeze rule does not apply to air
	wantReject(t, b, mv(1, 4, 1, 12), RejectBadPath)
	wantReject(t, b, mv(1, 4, 2, 11, 3, 12), RejectCellOccupied)
}
