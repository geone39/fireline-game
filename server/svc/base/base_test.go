package base

import (
	"errors"
	"os"
	"reflect"
	"testing"
)

// testGridSize mirrors the 44x44 grid of docs/01-architecture.md §7; the package itself has no such constant.
const testGridSize = 44

func realCatalog(t *testing.T) *Catalog {
	t.Helper()
	c, err := LoadCatalog("../../../content")
	if err != nil {
		t.Fatalf("LoadCatalog: %v", err)
	}
	return c
}

func rules(t *testing.T, water ...Point) Rules {
	return Rules{Cat: realCatalog(t), Grid: NewGrid(testGridSize, testGridSize, water)}
}

func hqItem(level, x, y int) Item { return Item{UID: "hq", Type: "b_hq", Level: level, X: x, Y: y} }

func base0(hqLevel int) Layout { return Layout{Items: []Item{hqItem(hqLevel, 20, 20)}} }

func wantCode(t *testing.T, err error, code Code) {
	t.Helper()
	var e *Error
	if !errors.As(err, &e) || e.Code != code {
		t.Fatalf("want code %q, got %v", code, err)
	}
}

func build(uid, typ string, x, y int) Command {
	return Command{Op: OpBuild, UID: uid, Type: typ, X: x, Y: y}
}

func TestCatalogFromContent(t *testing.T) {
	c := realCatalog(t)
	hq, _ := c.Def("b_hq")
	if hq.W != 4 || hq.H != 4 || c.HQID() != "b_hq" {
		t.Fatalf("hq def: %+v", hq)
	}
	wall, _ := c.Def("d_wall")
	if wall.Block != BlockAll || wall.Kind != KindPassive {
		t.Fatalf("wall: %+v", wall)
	}
	trap, _ := c.Def("d_tank_trap")
	if trap.Block != BlockVehicles {
		t.Fatalf("tank trap: %+v", trap)
	}
	wire, _ := c.Def("d_wire")
	if wire.Block != BlockNone {
		t.Fatalf("wire: %+v", wire)
	}
	ship, _ := c.Def("b_shipyard")
	if !ship.RequiresWater {
		t.Fatal("shipyard must require water")
	}
	rock, _ := c.Def("o_rock")
	// content may or may not give obstacles an explicit size; either way it must be a real footprint
	if rock.W < 1 || rock.H < 1 {
		t.Fatalf("obstacle footprint: %+v", rock)
	}
}

func TestMaxCountStepTable(t *testing.T) {
	c := realCatalog(t)
	b, _ := c.Def("b_barracks") // {1: 1, 3: 2, 6: 3, 8: 4}
	for hq, want := range map[int]int{1: 1, 2: 1, 3: 2, 5: 2, 6: 3, 8: 4, 10: 4} {
		if got, ok := c.MaxCount(b, hq); !ok || got != want {
			t.Errorf("barracks at HQ %d: got %d,%v want %d", hq, got, ok, want)
		}
	}
	if n, ok := c.MaxCount(b, 0); !ok || n != 0 {
		t.Errorf("below first step must be 0, got %d,%v", n, ok)
	}
	m, _ := c.Def("b_cash_mine")
	// with or without a max_by_hq table in content: unlimited, or a positive limit at HQ 5
	if n, ok := c.MaxCount(m, 5); ok && n < 1 {
		t.Errorf("cash mine at HQ 5: limit %d", n)
	}
	hq, _ := c.Def("b_hq")
	if n, ok := c.MaxCount(hq, 5); !ok || n != 1 {
		t.Error("exactly one HQ")
	}
}

func TestBuildBounds(t *testing.T) {
	r := rules(t)
	l := base0(1)
	for _, p := range []Point{{-1, 0}, {0, -1}, {testGridSize - 2, 0}, {0, testGridSize - 2}} {
		_, _, err := r.Apply(l, build("m", "b_cash_mine", p.X, p.Y)) // 3x3
		wantCode(t, err, CodeOutOfBounds)
	}
	if _, _, err := r.Apply(l, build("m", "b_cash_mine", testGridSize-3, testGridSize-3)); err != nil {
		t.Fatalf("flush with the corner must be fine: %v", err)
	}
}

func TestBuildRotationSwapsFootprint(t *testing.T) {
	// no non-square type exists in content yet, so use a synthetic catalog
	c, err := ParseCatalog([]byte(`core: [{id: hq, size: [2,2]}]
military: [{id: bar, size: [1,3], unlock_hq: 1}]`), []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	rr := Rules{Cat: c, Grid: NewGrid(5, 5, nil)}
	lay := Layout{Items: []Item{{UID: "hq", Type: "hq", Level: 1, X: 0, Y: 0}}}
	if _, _, err := rr.Apply(lay, Command{Op: OpBuild, UID: "b", Type: "bar", X: 4, Y: 0, Rot: 0}); err != nil {
		t.Fatalf("1x3 at x=4 fits: %v", err)
	}
	_, _, err = rr.Apply(lay, Command{Op: OpBuild, UID: "b", Type: "bar", X: 4, Y: 0, Rot: 1}) // 3x1 sticks out
	wantCode(t, err, CodeOutOfBounds)
	_, _, err = rr.Apply(lay, Command{Op: OpBuild, UID: "b", Type: "bar", X: 0, Y: 0, Rot: 4})
	wantCode(t, err, CodeBadRot)
}

func TestBuildOverlap(t *testing.T) {
	r := rules(t)
	l := base0(1) // HQ at 20..23
	_, _, err := r.Apply(l, build("m", "b_cash_mine", 22, 22))
	wantCode(t, err, CodeOverlap)
	l2, _, err := r.Apply(l, build("m", "b_cash_mine", 24, 20)) // touching is fine
	if err != nil {
		t.Fatalf("adjacent must be fine: %v", err)
	}
	_, _, err = r.Apply(l2, build("n", "b_fuel_refinery", 26, 22))
	wantCode(t, err, CodeOverlap)
}

func TestBuildUnlockByHQ(t *testing.T) {
	r := rules(t)
	_, _, err := r.Apply(base0(1), build("f", "b_alloy_foundry", 0, 0)) // unlock_hq 4
	wantCode(t, err, CodeLocked)
	if _, _, err = r.Apply(base0(4), build("f", "b_alloy_foundry", 0, 0)); err != nil {
		t.Fatalf("HQ4 unlocks foundry: %v", err)
	}
	_, _, err = r.Apply(base0(4), build("t", "d_tesla", 0, 0)) // unlock_hq 9
	wantCode(t, err, CodeLocked)
}

func TestBuildLimitByHQ(t *testing.T) {
	r := rules(t)
	l := base0(1)
	l, _, err := r.Apply(l, build("k1", "b_barracks", 0, 0)) // limit 1 at HQ1
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.Apply(l, build("k2", "b_barracks", 4, 0))
	wantCode(t, err, CodeLimit)

	l3 := Layout{Items: append([]Item(nil), l.Items...)}
	l3.Items[0].Level = 3 // HQ 3 allows 2
	if l3, _, err = r.Apply(l3, build("k2", "b_barracks", 4, 0)); err != nil {
		t.Fatalf("second barracks at HQ3: %v", err)
	}
	_, _, err = r.Apply(l3, build("k3", "b_barracks", 8, 0))
	wantCode(t, err, CodeLimit)
}

func TestBuildLimitDefences(t *testing.T) {
	r := rules(t)
	l, _, err := r.Apply(base0(1), build("n1", "d_mg_nest", 0, 0)) // {1:1,3:3}
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.Apply(l, build("n2", "d_mg_nest", 3, 0))
	wantCode(t, err, CodeLimit)
	// below the first step the limit is zero (mortar needs HQ2)
	_, _, err = r.Apply(base0(1), build("mo", "d_mortar", 0, 0))
	wantCode(t, err, CodeLocked)
}

func TestHQNotBuildableTwice(t *testing.T) {
	r := rules(t)
	_, _, err := r.Apply(base0(5), build("hq2", "b_hq", 0, 0))
	wantCode(t, err, CodeNotPlaceable)
}

func TestObstaclesNotBuildable(t *testing.T) {
	r := rules(t)
	_, _, err := r.Apply(base0(1), build("r", "o_rock", 0, 0))
	wantCode(t, err, CodeNotPlaceable)
}

func TestRequiresWater(t *testing.T) {
	// water column at x=10, y 0..9
	var water []Point
	for y := 0; y < 10; y++ {
		water = append(water, Point{10, y})
	}
	r := rules(t, water...)
	l := base0(4)
	_, _, err := r.Apply(l, build("s", "b_shipyard", 20, 0)) // 4x4 inland
	wantCode(t, err, CodeNeedsWater)
	if _, _, err = r.Apply(l, build("s", "b_shipyard", 11, 0)); err != nil { // shore on the east
		t.Fatalf("shore is fine: %v", err)
	}
	if _, _, err = r.Apply(l, build("s", "b_shipyard", 6, 0)); err != nil { // shore on the west (6..9)
		t.Fatalf("west shore is fine: %v", err)
	}
	_, _, err = r.Apply(l, build("s", "b_shipyard", 8, 0)) // covers x=10 water
	wantCode(t, err, CodeOnWater)
	_, _, err = r.Apply(l, build("c", "d_coastal", 20, 20)) // overlapping HQ anyway; use free spot
	if err == nil {
		t.Fatal("expected failure")
	}
	_, _, err = r.Apply(l, build("c", "d_coastal", 30, 30))
	wantCode(t, err, CodeNeedsWater)
	if _, _, err = r.Apply(l, build("c", "d_coastal", 11, 5)); err != nil {
		t.Fatalf("coastal on shore: %v", err)
	}
}

func TestOrdinaryBuildingCannotStandOnWater(t *testing.T) {
	r := rules(t, Point{5, 5})
	_, _, err := r.Apply(base0(1), build("m", "b_cash_mine", 4, 4))
	wantCode(t, err, CodeOnWater)
}

func TestBuildIdempotent(t *testing.T) {
	r := rules(t)
	l, res, err := r.Apply(base0(1), build("m", "b_cash_mine", 0, 0))
	if err != nil || res.Replayed {
		t.Fatalf("first: %v %+v", err, res)
	}
	l2, res, err := r.Apply(l, build("m", "b_cash_mine", 0, 0))
	if err != nil || !res.Replayed || !reflect.DeepEqual(l, l2) {
		t.Fatalf("replay must be a no-op: %v %+v", err, res)
	}
	// even when the limit is exhausted, a replay still succeeds
	l3, _, _ := r.Apply(base0(1), build("k1", "b_barracks", 0, 0))
	if _, res, err = r.Apply(l3, build("k1", "b_barracks", 0, 0)); err != nil || !res.Replayed {
		t.Fatalf("replay at the limit: %v %+v", err, res)
	}
	_, _, err = r.Apply(l, build("m", "b_cash_mine", 5, 0))
	wantCode(t, err, CodeConflict)
	_, _, err = r.Apply(l, build("m", "b_fuel_refinery", 0, 0))
	wantCode(t, err, CodeConflict)
}

func TestMove(t *testing.T) {
	r := rules(t)
	l, _, _ := r.Apply(base0(1), build("m", "b_cash_mine", 0, 0))
	l, _, _ = r.Apply(l, build("v", "b_vault", 4, 0))

	out, res, err := r.Apply(l, Command{Op: OpMove, UID: "m", X: 10, Y: 10})
	if err != nil || res.Replayed {
		t.Fatalf("move: %v", err)
	}
	if got := out.Items[out.Find("m")]; got.X != 10 || got.Y != 10 || got.Level != 1 {
		t.Fatalf("moved item: %+v", got)
	}
	if l.Items[l.Find("m")].X != 0 {
		t.Fatal("input layout must not be mutated")
	}
	// idempotent
	out2, res, err := r.Apply(out, Command{Op: OpMove, UID: "m", X: 10, Y: 10})
	if err != nil || !res.Replayed || !reflect.DeepEqual(out, out2) {
		t.Fatalf("replay: %v %+v", err, res)
	}
	// can overlap its own old footprint
	if _, _, err = r.Apply(l, Command{Op: OpMove, UID: "m", X: 1, Y: 1}); err != nil {
		t.Fatalf("shift onto own old tiles: %v", err)
	}
	_, _, err = r.Apply(l, Command{Op: OpMove, UID: "m", X: 5, Y: 1})
	wantCode(t, err, CodeOverlap)
	_, _, err = r.Apply(l, Command{Op: OpMove, UID: "m", X: 43, Y: 43})
	wantCode(t, err, CodeOutOfBounds)
	_, _, err = r.Apply(l, Command{Op: OpMove, UID: "nope", X: 1, Y: 1})
	wantCode(t, err, CodeNotFound)
}

func TestMoveShipyardKeepsWaterRule(t *testing.T) {
	r := rules(t, Point{10, 0}, Point{10, 1}, Point{10, 2}, Point{10, 3})
	l, _, err := r.Apply(base0(4), build("s", "b_shipyard", 11, 0))
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = r.Apply(l, Command{Op: OpMove, UID: "s", X: 30, Y: 30})
	wantCode(t, err, CodeNeedsWater)
}

func TestMoveObstacleForbidden(t *testing.T) {
	r := rules(t)
	l := base0(1)
	l.Items = append(l.Items, Item{UID: "rock1", Type: "o_rock", Level: 1, X: 0, Y: 0})
	_, _, err := r.Apply(l, Command{Op: OpMove, UID: "rock1", X: 3, Y: 3})
	wantCode(t, err, CodeNotPlaceable)
}

func TestOtherOpsAreTypedOnly(t *testing.T) {
	r := rules(t)
	for _, op := range []Op{OpUpgrade, OpRemove, OpCollect, OpSpeedup, OpClearObstacle} {
		_, _, err := r.Apply(base0(1), Command{Op: op, UID: "hq"})
		wantCode(t, err, CodeNotImplemented)
	}
	_, _, err := r.Apply(base0(1), Command{Op: "bogus"})
	wantCode(t, err, CodeBadCommand)
}

func TestApplyAllIsAtomic(t *testing.T) {
	r := rules(t)
	l := base0(1)
	out, res, err := r.ApplyAll(l, []Command{build("m", "b_cash_mine", 0, 0), build("v", "b_vault", 4, 0)})
	if err != nil || len(out.Items) != 3 || len(res) != 2 {
		t.Fatalf("ApplyAll ok: %v %+v", err, out)
	}
	out, _, err = r.ApplyAll(l, []Command{build("m", "b_cash_mine", 0, 0), build("x", "b_cash_mine", 1, 1)})
	wantCode(t, err, CodeOverlap)
	if !reflect.DeepEqual(out, l) {
		t.Fatal("failed batch must return the original layout")
	}
	// whole batch replay is idempotent
	again, res, err := r.ApplyAll(out, []Command{build("a", "b_cash_mine", 0, 0)})
	if err != nil {
		t.Fatal(err)
	}
	again2, res2, err := r.ApplyAll(again, []Command{build("a", "b_cash_mine", 0, 0)})
	if err != nil || !res2[0].Replayed || !reflect.DeepEqual(again, again2) || res[0].Replayed {
		t.Fatalf("batch replay: %v", err)
	}
}

// ---- ValidateLayout ----

func TestValidateLayoutOK(t *testing.T) {
	r := rules(t)
	if err := r.ValidateLayout(base0(1)); err != nil {
		t.Fatal(err)
	}
}

func TestValidateLayoutNoHQ(t *testing.T) {
	r := rules(t)
	wantCode(t, r.ValidateLayout(Layout{}), CodeNoHQ)
}

func TestValidateLayoutRules(t *testing.T) {
	r := rules(t)
	with := func(extra ...Item) Layout {
		return Layout{Items: append([]Item{hqItem(1, 20, 20)}, extra...)}
	}
	cases := []struct {
		name string
		l    Layout
		want Code
	}{
		{"duplicate uid", with(Item{UID: "hq", Type: "b_cash_mine", Level: 1}), CodeDuplicateUID},
		{"empty uid", with(Item{Type: "b_cash_mine", Level: 1}), CodeDuplicateUID},
		{"unknown type", with(Item{UID: "z", Type: "nope", Level: 1}), CodeUnknownType},
		{"bounds", with(Item{UID: "m", Type: "b_cash_mine", Level: 1, X: 43, Y: 0}), CodeOutOfBounds},
		{"overlap", with(Item{UID: "m", Type: "b_cash_mine", Level: 1, X: 21, Y: 21}), CodeOverlap},
		{"hq limit", with(Item{UID: "h2", Type: "b_hq", Level: 1, X: 0, Y: 0}), CodeLimit},
		{"locked", with(Item{UID: "f", Type: "b_alloy_foundry", Level: 1, X: 0, Y: 0}), CodeLocked},
		{"count", with(
			Item{UID: "k1", Type: "b_barracks", Level: 1, X: 0, Y: 0},
			Item{UID: "k2", Type: "b_barracks", Level: 1, X: 4, Y: 0}), CodeLimit},
		{"inland coastal", with(Item{UID: "s", Type: "d_coastal", Level: 1, X: 0, Y: 0}), CodeNeedsWater},
		{"level", with(Item{UID: "m", Type: "b_cash_mine", Level: 0}), CodeBadLevel},
		{"rot", with(Item{UID: "m", Type: "b_cash_mine", Level: 1, Rot: -1}), CodeBadRot},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) { wantCode(t, r.ValidateLayout(tc.l), tc.want) })
	}
	// requires_water on a full layout
	l := with()
	l.Items[0].Level = 4
	l.Items = append(l.Items, Item{UID: "s", Type: "b_shipyard", Level: 1, X: 0, Y: 0})
	wantCode(t, r.ValidateLayout(l), CodeNeedsWater)
}

// ringAround returns items of type typ surrounding the HQ at (20,20) 4x4 at distance 1.
func ringAround(typ string, gap map[Point]bool) []Item {
	var out []Item
	n := 0
	for x := 19; x <= 24; x++ {
		for y := 19; y <= 24; y++ {
			if x != 19 && x != 24 && y != 19 && y != 24 {
				continue
			}
			p := Point{x, y}
			if gap[p] {
				continue
			}
			n++
			out = append(out, Item{UID: "ring" + string(rune('A'+n/26)) + string(rune('a'+n%26)), Type: typ, Level: 1, X: x, Y: y})
		}
	}
	return out
}

func TestSealedByWallsIsAllowed(t *testing.T) {
	r := rules(t)
	l := Layout{Items: append([]Item{hqItem(4, 20, 20)}, ringAround("d_wall", nil)...)}
	if err := r.ValidateLayout(l); err != nil {
		t.Fatalf("a wall ring is destructible, must be allowed: %v", err)
	}
	a := r.Analyze(l)
	if !a.HQReachable || a.HQOpen {
		t.Fatalf("analysis: %+v", a)
	}
	// with a gap it is open
	l = Layout{Items: append([]Item{hqItem(4, 20, 20)}, ringAround("d_wall", map[Point]bool{{19, 21}: true})...)}
	if a := r.Analyze(l); !a.HQOpen {
		t.Fatalf("gap must open the base: %+v", a)
	}
}

func TestSealedWithoutDestructibleWallsIsRejected(t *testing.T) {
	r := rules(t)
	// a ring of 1x1 obstacles (never walkable, not walls) around the HQ
	l := Layout{Items: append([]Item{hqItem(4, 20, 20)}, ringAround("o_rock", nil)...)}
	wantCode(t, r.ValidateLayout(l), CodeSealed)
	// open one gap: fine
	l = Layout{Items: append([]Item{hqItem(4, 20, 20)}, ringAround("o_rock", map[Point]bool{{19, 21}: true})...)}
	if err := r.ValidateLayout(l); err != nil {
		t.Fatalf("gap: %v", err)
	}
	// a ring with one wall segment among rocks is breakable
	ring := ringAround("o_rock", nil)
	ring[3].Type = "d_wall"
	if err := r.ValidateLayout(Layout{Items: append([]Item{hqItem(4, 20, 20)}, ring...)}); err != nil {
		t.Fatalf("one wall in the ring makes it breakable: %v", err)
	}
}

func TestNonWallsAreWalkableForReachability(t *testing.T) {
	r := rules(t)
	// wire (blocks:false) and tank traps (vehicles only) do not seal infantry out
	for _, typ := range []string{"d_wire", "d_tank_trap"} {
		l := Layout{Items: append([]Item{hqItem(5, 20, 20)}, ringAround(typ, nil)...)}
		if a := r.Analyze(l); !a.HQOpen {
			t.Errorf("%s ring must stay open: %+v", typ, a)
		}
	}
}

func TestSealedByBuildingsIsRejected(t *testing.T) {
	// synthetic: 5x5 grid, HQ 1x1 in the middle, 1x1 "wall-less" buildings around it
	c, err := ParseCatalog([]byte(`core: [{id: hq, size: [1,1]}]
resources: [{id: blk, size: [1,1], unlock_hq: 1}]`), []byte("{}"))
	if err != nil {
		t.Fatal(err)
	}
	r := Rules{Cat: c, Grid: NewGrid(3, 3, nil)}
	l := Layout{Items: []Item{{UID: "hq", Type: "hq", Level: 1, X: 1, Y: 1}}}
	for i, p := range []Point{{1, 0}, {0, 1}, {2, 1}, {1, 2}} {
		l.Items = append(l.Items, Item{UID: string(rune('a' + i)), Type: "blk", Level: 1, X: p.X, Y: p.Y})
	}
	wantCode(t, r.ValidateLayout(l), CodeSealed)
	l.Items = l.Items[:4] // remove one blocker
	if err := r.ValidateLayout(l); err != nil {
		t.Fatalf("open side: %v", err)
	}
}

func TestWaterBlocksReachability(t *testing.T) {
	// HQ on a 3x3 island of land in water: unreachable on foot -> sealed
	var water []Point
	for x := 0; x < 5; x++ {
		for y := 0; y < 5; y++ {
			if x == 0 || y == 0 || x == 4 || y == 4 {
				water = append(water, Point{x, y})
			}
		}
	}
	c, _ := ParseCatalog([]byte(`core: [{id: hq, size: [1,1]}]`), []byte("{}"))
	r := Rules{Cat: c, Grid: NewGrid(5, 5, water)}
	l := Layout{Items: []Item{{UID: "hq", Type: "hq", Level: 1, X: 2, Y: 2}}}
	wantCode(t, r.ValidateLayout(l), CodeSealed)
}

const gridYAML = `schema: 1
grid: {w: 10, h: 8}
default_map: plain
maps:
  plain:   {water: []}
  coastal: {water: [[8, 0, 2, 8], [0, 7, 3, 1], [9, 7, 5, 5]]}
`

func TestParseGrid(t *testing.T) {
	g, err := ParseGrid([]byte(gridYAML), "")
	if err != nil || g.W != 10 || g.H != 8 || g.Water(8, 0) {
		t.Fatalf("default map: %+v %v", g, err)
	}
	g, err = ParseGrid([]byte(gridYAML), "coastal")
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range []Point{{8, 0}, {9, 7}, {0, 7}, {2, 7}} {
		if !g.Water(p.X, p.Y) {
			t.Errorf("%v should be water", p)
		}
	}
	for _, p := range []Point{{7, 0}, {3, 7}, {0, 6}} {
		if g.Water(p.X, p.Y) {
			t.Errorf("%v should be land", p)
		}
	}
	if _, err = ParseGrid([]byte(gridYAML), "nope"); err == nil {
		t.Error("unknown map must fail")
	}
	if _, err = ParseGrid([]byte("grid: {w: 0, h: 5}"), ""); err == nil {
		t.Error("zero size must fail")
	}
	if _, err = ParseGrid([]byte("grid: {w: 5, h: 5}\ndefault_map: a\nmaps: {a: {water: [[1,2,3]]}}"), ""); err == nil {
		t.Error("bad rect must fail")
	}
}

func TestLoadGridFromContent(t *testing.T) {
	g, err := LoadGrid("../../../content", "")
	if errors.Is(err, os.ErrNotExist) {
		t.Skip("content/base_grid.yaml not present yet")
	}
	if err != nil {
		t.Fatal(err)
	}
	if g.W < 1 || g.H < 1 {
		t.Fatalf("grid: %dx%d", g.W, g.H)
	}
	c, err := LoadGrid("../../../content", "coastal")
	if err != nil {
		t.Fatal(err)
	}
	water := 0
	for y := 0; y < c.H; y++ {
		for x := 0; x < c.W; x++ {
			if c.Water(x, y) {
				water++
			}
		}
	}
	if water == 0 {
		t.Error("coastal map should have water for the shipyard")
	}
}

func TestParseCatalogErrors(t *testing.T) {
	if _, err := ParseCatalog([]byte(`resources: [{id: a, size: [1,1]}]`), []byte("{}")); err == nil {
		t.Error("no core must fail")
	}
	if _, err := ParseCatalog([]byte(`core: [{id: a, size: [1]}]`), []byte("{}")); err == nil {
		t.Error("bad size must fail")
	}
	if _, err := ParseCatalog([]byte(`core: [{id: a, size: [1,1]}, {id: a, size: [1,1]}]`), []byte("{}")); err == nil {
		t.Error("duplicate id must fail")
	}
}

func TestDeterministicFirstError(t *testing.T) {
	r := rules(t)
	l := base0(1)
	l.Items = append(l.Items,
		Item{UID: "a", Type: "d_mg_nest", Level: 1, X: 0, Y: 0},
		Item{UID: "b", Type: "d_mg_nest", Level: 1, X: 3, Y: 0},
		Item{UID: "k1", Type: "b_barracks", Level: 1, X: 0, Y: 5},
		Item{UID: "k2", Type: "b_barracks", Level: 1, X: 4, Y: 5})
	first := r.ValidateLayout(l).Error()
	for i := 0; i < 20; i++ {
		if got := r.ValidateLayout(l).Error(); got != first {
			t.Fatalf("non-deterministic: %q vs %q", got, first)
		}
	}
}
