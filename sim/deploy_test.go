package sim

import "testing"

func newB(t *testing.T, mut func(*Snapshot)) *Battle {
	t.Helper()
	s := f01Snapshot()
	if mut != nil {
		mut(&s)
	}
	b, err := NewBattle(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func applyOK(t *testing.T, b *Battle, c Command) {
	t.Helper()
	n := len(b.rejected)
	if _, err := b.Apply(c); err != nil {
		t.Fatal(err)
	}
	if len(b.rejected) != n {
		t.Fatalf("%+v rejected with code %d", c, lastReject(b))
	}
}

// wantReject checks the code and that the world (everything but the rejection log) did not change.
func wantReject(t *testing.T, b *Battle, c Command, code RejectCode) {
	t.Helper()
	before := b.worldHash()
	n := len(b.rejected)
	if _, err := b.Apply(c); err != nil {
		t.Fatal(err)
	}
	if len(b.rejected) != n+1 || lastReject(b) != code {
		t.Fatalf("%+v: want reject %d, got %v", c, code, b.rejected[n:])
	}
	if b.worldHash() != before {
		t.Fatalf("%+v: rejected command changed the state", c)
	}
}

func (b *Battle) worldHash() uint64 {
	saved := b.rejected
	b.rejected = nil
	h := b.stateHash()
	b.rejected = saved
	return h
}

func at(c Command, lvl int) Command { c.Level = lvl; return c }

func TestDeployZoneAndStock(t *testing.T) {
	b := newB(t, func(s *Snapshot) { s.Army = []ArmySlot{{"u_brawler", 1, 2}, {"u_drone", 1, 1}} })
	wantReject(t, b, dep("u_brawler", 2, 10), RejectOutOfZone)  // inside build_zone
	wantReject(t, b, dep("u_brawler", -1, 10), RejectOutOfZone) // off map
	wantReject(t, b, dep("u_brawler", 44, 10), RejectOutOfZone)
	wantReject(t, b, at(dep("u_brawler", 1, 10), 2), RejectNoStock)
	wantReject(t, b, dep("u_rifleman", 1, 10), RejectNoStock) // not in the army
	wantReject(t, b, dep("zzz", 1, 10), RejectNoStock)
	applyOK(t, b, dep("u_brawler", 1, 10))
	wantReject(t, b, dep("u_brawler", 1, 10), RejectCellOccupied)
	applyOK(t, b, dep("u_drone", 1, 10)) // air shares the tile with ground
	applyOK(t, b, dep("u_brawler", 0, 5))
	wantReject(t, b, dep("u_brawler", 0, 6), RejectNoStock) // Count exhausted
}

func TestDeployShipsOnWaterOnly(t *testing.T) {
	b := newB(t, func(s *Snapshot) {
		s.Map = "coastal"
		s.Army = []ArmySlot{{"s_gunboat", 1, 2}, {"u_rifleman", 1, 1}}
	})
	wantReject(t, b, dep("s_gunboat", 1, 10), RejectOutOfZone)   // land
	wantReject(t, b, dep("s_gunboat", 41, 10), RejectOutOfZone)  // water but inside the build zone
	wantReject(t, b, dep("u_rifleman", 42, 10), RejectOutOfZone) // water is not for infantry
	applyOK(t, b, dep("u_rifleman", 1, 10))
	applyOK(t, b, dep("s_gunboat", 42, 10))
	wantReject(t, b, dep("s_gunboat", 42, 10), RejectCellOccupied)
}

func TestDeployHeroOnce(t *testing.T) {
	b := newB(t, func(s *Snapshot) { s.Hero, s.HeroLevel = "h_commander", 2 })
	wantReject(t, b, dep("h_commander", 1, 5), RejectNoStock) // wrong level
	applyOK(t, b, at(dep("h_commander", 1, 5), 2))
	wantReject(t, b, at(dep("h_commander", 1, 6), 2), RejectHeroLimit)
	if h := b.ent(3); h.kind != KindHero || h.maxHP != 1600 || b.heroID != 3 {
		t.Fatalf("%+v", h)
	}
}

func TestDeployParatrooperGroup(t *testing.T) {
	b := newB(t, func(s *Snapshot) { s.Army = []ArmySlot{{"u_para", 1, 8}} })
	wantReject(t, b, dep("u_para", 3, 20), RejectOutOfZone) // 3 cells from the edge: not within paradrop_edge_cells=3
	applyOK(t, b, dep("u_para", 2, 20))                     // inside the build zone but within 3 of the edge
	// anchor + 3 nearest: ring r=1 around (2,20) in (Y,X) order, (1,19) first
	want := []Cell{cell(2, 20), cell(1, 19), cell(2, 19), cell(3, 19)}
	for i, w := range want {
		if e := b.ent(EntityID(3 + i)); e == nil || e.pos != w || e.contentID != "u_para" {
			t.Fatalf("unit %d at %v want %v", 3+i, e.pos, w)
		}
	}
	if b.stock[0].Count != 4 {
		t.Fatalf("group consumes 4 from the slot, left %d", b.stock[0].Count)
	}
	wantReject(t, b, dep("u_para", 2, 20), RejectCellOccupied)
	applyOK(t, b, dep("u_para", 2, 30)) // second group, slot has 4 left
}

func TestLoadCapacityAndOwnership(t *testing.T) {
	b := newB(t, func(s *Snapshot) {
		s.Army = []ArmySlot{{"v_apc", 1, 1}, {"u_rifleman", 1, 4}, {"v_tank", 1, 1}}
	})
	applyOK(t, b, dep("v_apc", 1, 20))      // 3
	applyOK(t, b, dep("u_rifleman", 1, 21)) // 4
	applyOK(t, b, dep("u_rifleman", 1, 22)) // 5
	applyOK(t, b, dep("u_rifleman", 1, 23)) // 6
	applyOK(t, b, dep("v_tank", 1, 24))     // 7
	load := func(p, c EntityID) Command { return Command{Op: OpLoad, Unit: p, Target: c} }
	wantReject(t, b, load(99, 3), RejectUnknownEntity)
	wantReject(t, b, load(1, 3), RejectNotOwner)      // defender building
	wantReject(t, b, load(7, 3), RejectInvalidTarget) // vehicle cannot be a passenger
	wantReject(t, b, load(4, 6), RejectInvalidTarget) // tank carries nobody
	wantReject(t, b, load(3, 3), RejectInvalidTarget) // self
	applyOK(t, b, load(4, 3))
	applyOK(t, b, load(5, 3))
	wantReject(t, b, load(6, 3), RejectCapacity) // carries: 2
	wantReject(t, b, load(4, 3), RejectInvalidTarget)
	if b.unit[b.idx(cell(1, 21))] != 0 || b.ent(4).onField || b.ent(4).aboard != 3 {
		t.Fatal("passenger must leave the grid")
	}
	// boarded passengers cannot move
	applyOK(t, b, Command{Op: OpStart})
	wantReject(t, b, mv(1, 4, 2, 21), RejectInvalidTarget)
}

func TestStartNeedsUnitsAndClosesDeployment(t *testing.T) {
	b := newB(t, nil)
	wantReject(t, b, Command{Op: OpStart}, RejectInvalidTarget)
	applyOK(t, b, dep("u_rifleman", 1, 10))
	applyOK(t, b, Command{Op: OpStart})
	if b.ph != phasePlayer || b.turn != 1 || b.ent(3).apLeft != 2 {
		t.Fatalf("phase=%d turn=%d ap=%d", b.ph, b.turn, b.ent(3).apLeft)
	}
	wantReject(t, b, Command{Turn: 1, Op: OpDeploy, ID: "u_rifleman", Level: 1, To: cell(1, 11)}, RejectBadPhase)
	wantReject(t, b, Command{Turn: 1, Op: OpStart}, RejectBadPhase)
	wantReject(t, b, Command{Turn: 1, Op: OpLoad, Unit: 3, Target: 3}, RejectBadPhase)
}

func TestPhaseAndTurnChecksInDeployment(t *testing.T) {
	b := newB(t, nil)
	applyOK(t, b, dep("u_rifleman", 1, 10))
	wantReject(t, b, mv(0, 3, 2, 10), RejectBadPhase) // Move before Start
	wantReject(t, b, Command{Turn: 1, Op: OpDeploy, ID: "u_rifleman", Level: 1, To: cell(1, 9)}, RejectBadTurn)
	wantReject(t, b, Command{Op: Op(99)}, RejectBadPhase)
	wantReject(t, b, Command{Op: OpEndTurn}, RejectBadPhase)
	applyOK(t, b, Command{Op: OpRetreat})
	wantReject(t, b, dep("u_rifleman", 1, 9), RejectBattleOver)
	if r := b.Result(); r.EndReason != EndRetreat || r.TurnsUsed != 0 || r.HeroSurvived {
		t.Fatalf("%+v", r)
	}
}
