package sim

import (
	"reflect"
	"testing"
)

func TestAttackerTargetOrderDistHPID(t *testing.T) {
	// ids by (Y, X): wall(1) at (9,10), nest(2) at (12,10) 2x2, statue(3) decor, mine(4) hidden, hq(5, far away), unit(6)
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_mg_nest", 12, 10), pb("d_wall", 9, 10), pb("x_statue", 9, 12), pb("d_mine", 9, 14), pb("b_hq", 30, 30)}
	}, du{"u_rifleman", 1, 10})
	u := b.ent(6)
	b.lift(u)
	b.place(u, cell(8, 10))
	// range 4: wall dist 1, nest (x 12..13) dist 4; the decor statue and the hidden mine never count
	if got, want := b.targetsInRange(u), []EntityID{1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
	// a revealed mine becomes a target
	b.ent(4).revealed = true
	b.lift(u)
	b.place(u, cell(9, 16))
	if got := b.targetsInRange(u); !reflect.DeepEqual(got, []EntityID{4}) {
		t.Fatalf("revealed mine: %v", got)
	}
	b.ent(4).alive = false
	if got := b.targetsInRange(u); len(got) != 0 {
		t.Fatalf("dead target listed: %v", got)
	}
}

func TestTieBreakHPThenID(t *testing.T) {
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wall", 9, 9), pb("d_wall", 9, 10), pb("d_wall", 9, 11), pb("d_wall", 11, 10)}
	}, du{"u_rifleman", 1, 10})
	// canonical ids by (Y, X): (9,9)=1, (9,10)=2, (11,10)=3, (9,11)=4; unit = 5
	u := b.ent(5)
	b.lift(u)
	b.place(u, cell(10, 10)) // all four walls at dist 1
	b.ent(4).hp = 5
	b.ent(1).hp = 50
	if got, want := b.targetsInRange(u), []EntityID{4, 1, 2, 3}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestDefenceTargets(t *testing.T) {
	// mortar (3x3 at (20,20)) vs INF with min_range 3; nest (2x2 at (30,30)) vs INF|AIR
	b := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_mortar", 20, 20), pb("d_mg_nest", 30, 30)}
	}, du{"u_rifleman", 1, 1}, du{"u_rifleman", 1, 2}, du{"v_tank", 1, 3}, du{"u_drone", 1, 4})
	mortar, nest := b.ent(1), b.ent(2)
	inf1, inf2, tank, drone := b.ent(3), b.ent(4), b.ent(5), b.ent(6)
	put := func(e *entity, x, y int) { b.lift(e); b.place(e, cell(x, y)) }
	put(inf1, 23, 21) // dist 1 to the footprint 20..22: below min range
	put(inf2, 26, 21) // dist 4
	put(tank, 24, 21) // dist 2, but the mortar cannot shoot vehicles
	if got := b.targetsInRange(mortar); !reflect.DeepEqual(got, []EntityID{4}) {
		t.Fatalf("mortar: %v", got)
	}
	put(inf1, 25, 19) // dist 3: inside [3,10]
	if got := b.targetsInRange(mortar); !reflect.DeepEqual(got, []EntityID{3, 4}) {
		t.Fatalf("mortar: %v", got)
	}
	put(drone, 29, 29) // air, dist 1 to the nest
	put(tank, 31, 32)  // dist 1 but not INF/AIR
	if got := b.targetsInRange(nest); !reflect.DeepEqual(got, []EntityID{6}) {
		t.Fatalf("nest: %v", got)
	}
	drone.onField = false // aboard / not on the grid: never a target
	if got := b.targetsInRange(nest); len(got) != 0 {
		t.Fatalf("nest saw an off-field unit: %v", got)
	}
}

func TestChebAndFootDist(t *testing.T) {
	if Cheb(cell(0, 0), cell(3, -4)) != 4 || Cheb(cell(5, 5), cell(5, 5)) != 0 {
		t.Fatal("cheb")
	}
	a := &entity{pos: cell(12, 10), w: 2, h: 2}
	if d := footCell(cell(5, 12), a); d != 7 { // spec 12 F01 table: (5,12) -> (12,11) = 7
		t.Fatalf("F01 dist %d", d)
	}
	if d := footCell(cell(13, 11), a); d != 0 {
		t.Fatalf("inside footprint: %d", d)
	}
	if !lineOfFire(cell(0, 0), cell(40, 40)) {
		t.Fatal("LOS stub must be true in R1")
	}
}
