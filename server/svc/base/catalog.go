// Package base holds the pure base-layout logic: grid, placement and layout
// validation. It has no UI, DB or gateway dependencies; all game numbers come
// from the content catalog loaded from content/*.yaml.
package base

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Kind classifies a placeable entity.
type Kind string

const (
	KindCore     Kind = "core"     // HQ
	KindBuilding Kind = "building" // resource / military buildings
	KindPassive  Kind = "passive"  // walls, wire, mines and other passive defences
	KindActive   Kind = "active"   // turrets and other active defences
	KindDecor    Kind = "decor"
	KindObstacle Kind = "obstacle"
)

// Block says whom an entity physically blocks.
type Block int

const (
	BlockNone     Block = iota // does not stop anyone
	BlockVehicles              // stops vehicles only
	BlockAll                   // stops everyone
)

// LimitStep is one row of a per-HQ-level table: from HQ level At the limit is Max.
type LimitStep struct{ At, Max int }

// Def is a placeable definition resolved from content.
type Def struct {
	ID            string
	Kind          Kind
	W, H          int
	UnlockHQ      int
	RequiresWater bool
	Block         Block
	// Limits is sorted by At. Nil means "no per-HQ limit table" (see Catalog.MaxCount).
	Limits []LimitStep
	// DefaultSize is true when content has no size and the loader substituted 1x1.
	DefaultSize bool
}

// Catalog is the read-only set of definitions.
type Catalog struct {
	defs map[string]Def
	ids  []string
	hq   string
}

// HQID is the id of the headquarters definition (the single KindCore entry).
func (c *Catalog) HQID() string { return c.hq }

// Def returns the definition for id.
func (c *Catalog) Def(id string) (Def, bool) { d, ok := c.defs[id]; return d, ok }

// IDs returns all ids in a stable (sorted) order.
func (c *Catalog) IDs() []string { return append([]string(nil), c.ids...) }

// MaxCount returns the maximum number of instances of d allowed at hq level.
// ok=false means unlimited. The HQ itself is limited to exactly one.
func (c *Catalog) MaxCount(d Def, hq int) (n int, ok bool) {
	if d.Kind == KindCore {
		return 1, true
	}
	if d.Limits == nil {
		return 0, false
	}
	for _, s := range d.Limits {
		if s.At <= hq {
			n = s.Max
		}
	}
	return n, true
}

type blockField Block

func (b *blockField) UnmarshalYAML(n *yaml.Node) error {
	switch n.Value {
	case "true":
		*b = blockField(BlockAll)
	case "false", "":
		*b = blockField(BlockNone)
	case "vehicles":
		*b = blockField(BlockVehicles)
	default:
		return fmt.Errorf("blocks: unsupported value %q", n.Value)
	}
	return nil
}

type rawDef struct {
	ID            string      `yaml:"id"`
	Size          []int       `yaml:"size"`
	UnlockHQ      int         `yaml:"unlock_hq"`
	RequiresWater bool        `yaml:"requires_water"`
	Blocks        *blockField `yaml:"blocks"`
	MaxByHQ       map[int]int `yaml:"max_by_hq"`
	MaxCountByHQ  map[int]int `yaml:"max_count_by_hq"`
}

type buildingsFile struct {
	Core      []rawDef `yaml:"core"`
	Resources []rawDef `yaml:"resources"`
	Military  []rawDef `yaml:"military"`
	Decor     []rawDef `yaml:"decor"`
	Obstacles []rawDef `yaml:"obstacles"`
}

type defencesFile struct {
	Passive []rawDef `yaml:"passive_obstacles"`
	Active  []rawDef `yaml:"active"`
}

// ParseCatalog builds a Catalog from the raw bytes of buildings.yaml and defences.yaml.
//
// ASSUMPTION: entries without `size` (decor, obstacles) occupy 1x1 (Def.DefaultSize).
// ASSUMPTION: entries without a per-HQ table have no count limit. Blocking defaults:
// buildings, active defences and obstacles block everyone, decor blocks none.
func ParseCatalog(buildings, defences []byte) (*Catalog, error) {
	var bf buildingsFile
	if err := yaml.Unmarshal(buildings, &bf); err != nil {
		return nil, fmt.Errorf("base: buildings.yaml: %w", err)
	}
	var df defencesFile
	if err := yaml.Unmarshal(defences, &df); err != nil {
		return nil, fmt.Errorf("base: defences.yaml: %w", err)
	}
	c := &Catalog{defs: map[string]Def{}}
	steps := []struct {
		rs []rawDef
		k  Kind
		b  Block
	}{
		{bf.Core, KindCore, BlockAll}, {bf.Resources, KindBuilding, BlockAll}, {bf.Military, KindBuilding, BlockAll},
		{bf.Decor, KindDecor, BlockNone}, {bf.Obstacles, KindObstacle, BlockAll},
		{df.Passive, KindPassive, BlockNone}, {df.Active, KindActive, BlockAll},
	}
	for _, s := range steps {
		for _, r := range s.rs {
			d, err := r.resolve(s.k, s.b)
			if err != nil {
				return nil, err
			}
			if _, dup := c.defs[d.ID]; dup {
				return nil, fmt.Errorf("base: duplicate id %q", d.ID)
			}
			c.defs[d.ID] = d
			c.ids = append(c.ids, d.ID)
			if s.k == KindCore {
				if c.hq != "" {
					return nil, fmt.Errorf("base: more than one core building")
				}
				c.hq = d.ID
			}
		}
	}
	if c.hq == "" {
		return nil, fmt.Errorf("base: no core (HQ) building in content")
	}
	sort.Strings(c.ids)
	return c, nil
}

func (r rawDef) resolve(k Kind, defBlock Block) (Def, error) {
	if r.ID == "" {
		return Def{}, fmt.Errorf("base: entry without id")
	}
	d := Def{ID: r.ID, Kind: k, UnlockHQ: r.UnlockHQ, RequiresWater: r.RequiresWater, Block: defBlock}
	switch len(r.Size) {
	case 0:
		d.W, d.H, d.DefaultSize = 1, 1, true
	case 2:
		d.W, d.H = r.Size[0], r.Size[1]
	default:
		return Def{}, fmt.Errorf("base: %s: size must be [w,h]", r.ID)
	}
	if d.W <= 0 || d.H <= 0 {
		return Def{}, fmt.Errorf("base: %s: non-positive size", r.ID)
	}
	if r.Blocks != nil {
		d.Block = Block(*r.Blocks)
	}
	tbl := r.MaxByHQ
	if tbl == nil {
		tbl = r.MaxCountByHQ
	}
	for at, mx := range tbl { // map order is irrelevant: result is sorted below
		d.Limits = append(d.Limits, LimitStep{at, mx})
	}
	sort.Slice(d.Limits, func(i, j int) bool { return d.Limits[i].At < d.Limits[j].At })
	return d, nil
}

// LoadCatalog reads buildings.yaml and defences.yaml from a content directory.
func LoadCatalog(contentDir string) (*Catalog, error) {
	b, err := os.ReadFile(filepath.Join(contentDir, "buildings.yaml"))
	if err != nil {
		return nil, err
	}
	d, err := os.ReadFile(filepath.Join(contentDir, "defences.yaml"))
	if err != nil {
		return nil, err
	}
	return ParseCatalog(b, d)
}
