package sim

import (
	"reflect"
	"testing"
)

func TestShortestPathOpenFieldTieBreak(t *testing.T) {
	b := started(t, nil, du{"u_brawler", 1, 10})
	b.lift(b.ent(1))
	b.place(b.ent(1), cell(10, 10))
	p, cost, ok := b.shortestPath(b.ent(1), cell(12, 11))
	// two cheapest routes via (11,10) or (11,11); the lower cell index (11,10) pops first and stays the parent
	if !ok || cost != 2 || !reflect.DeepEqual(p, path(11, 10, 12, 11)) {
		t.Fatalf("%v %d %v", p, cost, ok)
	}
	p2, _, _ := b.shortestPath(b.ent(1), cell(12, 11))
	if !reflect.DeepEqual(p, p2) {
		t.Fatal("not deterministic")
	}
}

func TestShortestPathAroundWallsAndStaircase(t *testing.T) {
	// wall column x=10, y=5..15: the way from (8,10) to (12,10) goes around its end
	var bs []PlacedBuilding
	for y := 5; y <= 15; y++ {
		bs = append(bs, pb("d_wall", 10, y))
	}
	b := started(t, func(s *Snapshot) { s.Buildings = bs }, du{"u_brawler", 1, 10})
	u := b.ent(EntityID(len(bs) + 1))
	b.lift(u)
	b.place(u, cell(8, 10))
	p, cost, ok := b.shortestPath(u, cell(12, 10))
	// (8,10) -> (9,11..15) -> (10,16) -> (11,15..) -> (12,10): 6 steps down + 6 steps up diagonal-ish = cheb based
	if !ok || cost != 12 {
		t.Fatalf("cost %d path %v", cost, p)
	}
	for _, c := range p {
		if c.X == 10 && c.Y >= 5 && c.Y <= 15 {
			t.Fatalf("path crosses a wall at %v", c)
		}
	}

	// staircase of two walls: the diagonal (10,10)->(11,11) is forbidden
	b2 := started(t, func(s *Snapshot) {
		s.Buildings = []PlacedBuilding{pb("d_wall", 11, 10), pb("d_wall", 10, 11)}
	}, du{"u_brawler", 1, 1})
	u2 := b2.ent(3)
	b2.lift(u2)
	b2.place(u2, cell(10, 10))
	p2, c2, _ := b2.shortestPath(u2, cell(11, 11))
	if c2 != 3 || p2[0] == cell(11, 11) { // 3 steps around instead of 1 through the squeeze
		t.Fatalf("staircase: cost %d path %v", c2, p2)
	}
}

func TestShortestPathUnreachableAndUnitsAreObstacles(t *testing.T) {
	var bs []PlacedBuilding // walls fully enclose (30,30)
	for x := 29; x <= 31; x++ {
		for y := 29; y <= 31; y++ {
			if x != 30 || y != 30 {
				bs = append(bs, pb("d_wall", x, y))
			}
		}
	}
	b := started(t, func(s *Snapshot) { s.Buildings = bs }, du{"u_brawler", 1, 1}, du{"u_brawler", 1, 2})
	u := b.ent(EntityID(len(bs) + 1))
	if _, _, ok := b.shortestPath(u, cell(30, 30)); ok {
		t.Fatal("enclosed cell must be unreachable")
	}
	if _, _, ok := b.shortestPath(u, cell(-1, 0)); ok {
		t.Fatal("off map")
	}
	p, _, _ := b.shortestPath(u, cell(1, 3)) // the other brawler at (1,2) is an obstacle
	for _, c := range p {
		if c == cell(1, 2) {
			t.Fatal("path through a unit")
		}
	}
}

func TestApproachPicksCheapestTargetAndTruncates(t *testing.T) {
	b := newB(t, nil) // F01 base: nest (12,10) id 1, hq (20,10) id 2
	applyOK(t, b, dep("u_rifleman", 1, 10))
	applyOK(t, b, Command{Op: OpStart})
	u := b.ent(3)
	p, target, ok := b.approach(u)
	if !ok || target != 1 {
		t.Fatalf("target %d ok %v", target, ok)
	}
	// standing cell: cost 7 (cheb from (1,10)), range 4 to the nest footprint (12..13 x 10..11):
	// x = 8, and among the equal-cost cells the lowest index, y = 6
	full, cost, _ := b.shortestPath(u, cell(8, 6))
	if cost != 7 || len(p) != 3 || !reflect.DeepEqual(p, full[:3]) { // move_cells = 3
		t.Fatalf("p=%v full=%v cost=%d", p, full, cost)
	}
	// already in range: the chosen cell is the unit's own, nothing to walk
	b.lift(u)
	b.place(u, cell(8, 10))
	p, target, ok = b.approach(u)
	if !ok || target != 1 || len(p) != 0 {
		t.Fatalf("in range: p=%v target=%d", p, target)
	}
}

func BenchmarkPath44x44(b *testing.B) {
	// 44x44 with a comb of wall columns so Dijkstra has to wind around them
	var bs []PlacedBuilding
	for x := 6; x <= 38; x += 4 {
		for y := 3; y <= 39; y++ {
			if ((x/4)%2 == 0 && y < 38) || ((x/4)%2 == 1 && y > 4) {
				bs = append(bs, pb("d_wall", x, y))
			}
		}
	}
	s := f01Snapshot()
	s.Buildings = bs
	bt, err := NewBattle(s, 1)
	if err != nil {
		b.Fatal(err)
	}
	bt.Apply(dep("u_rifleman", 0, 0))
	bt.Apply(Command{Op: OpStart})
	u := bt.ent(EntityID(len(bs) + 1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, _, ok := bt.shortestPath(u, cell(43, 43)); !ok {
			b.Fatal("unreachable")
		}
	}
}
