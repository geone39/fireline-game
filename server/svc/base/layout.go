package base

import "sort"

// Point is a tile coordinate.
type Point struct{ X, Y int }

// Grid is the base terrain: dimensions and water tiles. Dimensions are supplied by
// the caller (they are not part of content/*.yaml yet, see docs/open-questions.md OQ-B1).
type Grid struct {
	W, H  int
	water []bool
}

// NewGrid returns a w x h grid with the given tiles marked as water.
// Out-of-range water tiles are ignored.
func NewGrid(w, h int, water []Point) Grid {
	g := Grid{W: w, H: h, water: make([]bool, max(w, 0)*max(h, 0))}
	for _, p := range water {
		if g.In(p.X, p.Y) {
			g.water[p.Y*w+p.X] = true
		}
	}
	return g
}

// In reports whether (x,y) is inside the grid.
func (g Grid) In(x, y int) bool { return x >= 0 && y >= 0 && x < g.W && y < g.H }

// Water reports whether (x,y) is a water tile.
func (g Grid) Water(x, y int) bool { return g.In(x, y) && g.water[y*g.W+x] }

// Item is one placed entity. UID is the instance id (client-generated for build),
// Type is the content id, Rot is the number of 90-degree turns (odd swaps w/h).
type Item struct {
	UID   string `json:"uid"`
	Type  string `json:"type"`
	Level int    `json:"level"`
	X     int    `json:"x"`
	Y     int    `json:"y"`
	Rot   int    `json:"rot"`
}

// Layout is the ordered list of placed items. Treat it as an immutable value:
// operations return new layouts.
type Layout struct {
	Items []Item `json:"items"`
}

// Clone returns a copy that shares no memory with l.
func (l Layout) Clone() Layout { return Layout{Items: append([]Item(nil), l.Items...)} }

// Find returns the index of the item with uid, or -1.
func (l Layout) Find(uid string) int {
	for i, it := range l.Items {
		if it.UID == uid {
			return i
		}
	}
	return -1
}

// HQLevel returns the level of the HQ item, or 0 if the layout has none.
func (l Layout) HQLevel(c *Catalog) int {
	for _, it := range l.Items {
		if it.Type == c.hq {
			return it.Level
		}
	}
	return 0
}

// Footprint returns the width and height of d once rotated by rot quarter-turns.
func Footprint(d Def, rot int) (w, h int) {
	if rot%2 != 0 {
		return d.H, d.W
	}
	return d.W, d.H
}

// Tiles returns the occupied tiles of an item in row-major order.
func Tiles(d Def, it Item) []Point {
	w, h := Footprint(d, it.Rot)
	out := make([]Point, 0, w*h)
	for dy := 0; dy < h; dy++ {
		for dx := 0; dx < w; dx++ {
			out = append(out, Point{it.X + dx, it.Y + dy})
		}
	}
	return out
}

// SortedByUID returns a copy ordered by UID (canonical form for hashing/snapshots).
func (l Layout) SortedByUID() Layout {
	c := l.Clone()
	sort.Slice(c.Items, func(i, j int) bool { return c.Items[i].UID < c.Items[j].UID })
	return c
}
