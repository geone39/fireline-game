// Package army is the training side of the army (docs/design/army-lab.md): per-building
// training queues, housing pools, escrow for raids. Every game number comes from
// content/*.yaml; the package keeps no balance constants.
package army

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

// Pool names are content ids (army.yaml pool_by_class / pool_spell values).

// Unit is a trainable item: a troop, a vehicle, a ship or a spell.
type Unit struct {
	ID       string
	Spell    bool
	Class    string // INF/AIR/VEH/SEA; empty for spells
	Pool     string
	Producer string // building type that trains it
	Housing  int
	TrainS   int64
	Cost     map[string]int64
	Group    int // >1: trained and deployed in multiples of Group
	NeedHQ   int
	// NeedBuilding is the minimum level of the producing building (0 = none).
	NeedBuilding int
}

// Node is the part of a research node the army cares about.
type Node struct {
	ID      string
	Item    string // unit or spell id; empty for perks
	ToLevel int
	Perk    string // perk key; empty for unit/spell nodes
	Value   int    // perk value (bool perks: 1 for true)

	Lab      int      // minimum laboratory level
	Requires []string // explicit prerequisites; empty = every lower node of the same item
	Soon     bool     // shown as "coming soon", cannot be started
	Fuel     int64    // price from tiers[tier]
	Hours    int64    // duration from tiers[tier]
}

// Content is everything the service reads from content/*.yaml.
type Content struct {
	units          map[string]Unit
	poolByBuilding map[string]string // pool_building inverted: building type -> pool
	buildingByPool map[string]string
	capacity       map[string][]int // building type -> capacity_by_level
	PerkKey        string           // housing perk key
	perkPools      map[string]bool
	MaxOrders      int
	RefundUnstart  int64
	RefundStarted  int64
	nodes          map[string]Node
	nodeOrder      []string // research.yaml order
	LabBuilding    string
	LabRefund      int64
}

type armyFile struct {
	PoolByClass  map[string]string `yaml:"pool_by_class"`
	PoolSpell    string            `yaml:"pool_spell"`
	PoolBuilding map[string]string `yaml:"pool_building"`
	HousingPerk  struct {
		Key   string   `yaml:"key"`
		Pools []string `yaml:"pools"`
	} `yaml:"housing_perk"`
	Producers map[string]string `yaml:"producers"`
	Queue     struct {
		MaxOrders int `yaml:"max_orders_per_building"`
	} `yaml:"queue"`
	RefundUnstarted int64  `yaml:"cancel_refund_unstarted_pct"`
	RefundStarted   int64  `yaml:"cancel_refund_started_pct"`
	LabBuilding     string `yaml:"lab_building"`
	LabRefund       int64  `yaml:"lab_cancel_refund_pct"`
}

type rawUnit struct {
	ID      string           `yaml:"id"`
	Class   string           `yaml:"class"`
	Housing int              `yaml:"housing"`
	TrainS  int64            `yaml:"train_s"`
	Cost    map[string]int64 `yaml:"cost"`
	Group   int              `yaml:"group"`
	Unlock  map[string]int   `yaml:"unlock"`
}

type troopsFile struct {
	Troops   []rawUnit `yaml:"troops"`
	Vehicles []rawUnit `yaml:"vehicles"`
	Ships    []rawUnit `yaml:"ships"`
	Spells   []rawUnit `yaml:"spells"`
}

type researchFile struct {
	Tiers map[int]struct {
		Fuel  int64 `yaml:"fuel"`
		Hours int64 `yaml:"hours"`
	} `yaml:"tiers"`
	Nodes []struct {
		ID       string   `yaml:"id"`
		Unit     string   `yaml:"unit"`
		Spell    string   `yaml:"spell"`
		ToLevel  int      `yaml:"to_level"`
		Tier     int      `yaml:"tier"`
		Lab      int      `yaml:"lab"`
		Requires []string `yaml:"requires"`
		Soon     bool     `yaml:"soon"`
		Perk     *struct {
			Key   string `yaml:"key"`
			Value any    `yaml:"value"`
		} `yaml:"perk"`
	} `yaml:"nodes"`
}

// ParseContent builds Content from the bytes of army.yaml, troops.yaml, buildings.yaml
// and research.yaml.
func ParseContent(armyY, troopsY, buildingsY, researchY []byte) (*Content, error) {
	var af armyFile
	if err := yaml.Unmarshal(armyY, &af); err != nil {
		return nil, fmt.Errorf("army: army.yaml: %w", err)
	}
	if len(af.PoolByClass) == 0 || af.PoolSpell == "" || len(af.PoolBuilding) == 0 || len(af.Producers) == 0 {
		return nil, fmt.Errorf("army: army.yaml: pool_by_class, pool_spell, pool_building and producers are required")
	}
	if af.Queue.MaxOrders < 1 {
		return nil, fmt.Errorf("army: army.yaml: queue.max_orders_per_building must be >= 1")
	}
	if af.LabBuilding == "" {
		return nil, fmt.Errorf("army: army.yaml: lab_building is required")
	}
	for _, p := range []int64{af.RefundUnstarted, af.RefundStarted, af.LabRefund} {
		if p < 0 || p > 100 {
			return nil, fmt.Errorf("army: army.yaml: refund percents must be 0..100")
		}
	}
	c := &Content{
		units: map[string]Unit{}, poolByBuilding: map[string]string{}, buildingByPool: af.PoolBuilding,
		capacity: map[string][]int{}, PerkKey: af.HousingPerk.Key, perkPools: map[string]bool{},
		MaxOrders: af.Queue.MaxOrders, RefundUnstart: af.RefundUnstarted, RefundStarted: af.RefundStarted,
		nodes: map[string]Node{}, LabBuilding: af.LabBuilding, LabRefund: af.LabRefund,
	}
	for pool, b := range af.PoolBuilding {
		c.poolByBuilding[b] = pool
	}
	for _, p := range af.HousingPerk.Pools {
		c.perkPools[p] = true
	}

	var tf troopsFile
	if err := yaml.Unmarshal(troopsY, &tf); err != nil {
		return nil, fmt.Errorf("army: troops.yaml: %w", err)
	}
	add := func(r rawUnit, spell bool) error {
		u := Unit{ID: r.ID, Spell: spell, Class: r.Class, Housing: r.Housing, TrainS: r.TrainS, Cost: r.Cost, Group: r.Group}
		key := r.Class
		if spell {
			key, u.Pool = "spell", af.PoolSpell
		} else {
			u.Pool = af.PoolByClass[r.Class]
		}
		u.Producer = af.Producers[key]
		if r.ID == "" || u.Pool == "" || u.Producer == "" || u.Housing < 1 || u.TrainS < 1 || len(u.Cost) == 0 {
			return fmt.Errorf("army: troops.yaml: %q: need id, class with pool/producer, housing, train_s and cost", r.ID)
		}
		u.NeedHQ = r.Unlock["hq"]
		for k, v := range r.Unlock { // `barracks: 2` -> level of b_barracks
			if k != "hq" {
				if "b_"+k != u.Producer {
					return fmt.Errorf("army: troops.yaml: %s: unlock %q does not match producer %s", r.ID, k, u.Producer)
				}
				u.NeedBuilding = v
			}
		}
		if _, dup := c.units[u.ID]; dup {
			return fmt.Errorf("army: troops.yaml: duplicate id %q", u.ID)
		}
		c.units[u.ID] = u
		return nil
	}
	for _, g := range []struct {
		rs    []rawUnit
		spell bool
	}{{tf.Troops, false}, {tf.Vehicles, false}, {tf.Ships, false}, {tf.Spells, true}} {
		for _, r := range g.rs {
			if err := add(r, g.spell); err != nil {
				return nil, err
			}
		}
	}

	var bs struct {
		Military []struct {
			ID       string `yaml:"id"`
			Capacity []int  `yaml:"capacity_by_level"`
		} `yaml:"military"`
	}
	if err := yaml.Unmarshal(buildingsY, &bs); err != nil {
		return nil, fmt.Errorf("army: buildings.yaml: %w", err)
	}
	for _, b := range bs.Military {
		if _, ok := c.poolByBuilding[b.ID]; ok {
			if len(b.Capacity) == 0 {
				return nil, fmt.Errorf("army: buildings.yaml: %s has no capacity_by_level", b.ID)
			}
			c.capacity[b.ID] = b.Capacity
		}
	}
	for b := range c.poolByBuilding {
		if c.capacity[b] == nil {
			return nil, fmt.Errorf("army: buildings.yaml: pool building %s not found in military", b)
		}
	}

	var rf researchFile
	if err := yaml.Unmarshal(researchY, &rf); err != nil {
		return nil, fmt.Errorf("army: research.yaml: %w", err)
	}
	for _, n := range rf.Nodes {
		nd := Node{ID: n.ID, Item: n.Unit + n.Spell, ToLevel: n.ToLevel, Lab: n.Lab, Requires: n.Requires, Soon: n.Soon}
		tier, ok := rf.Tiers[n.Tier]
		if n.ID == "" || !ok || tier.Fuel < 1 || tier.Hours < 1 || n.Lab < 1 {
			return nil, fmt.Errorf("army: research.yaml: node %q needs id, lab >= 1 and a known tier", n.ID)
		}
		if _, dup := c.nodes[n.ID]; dup {
			return nil, fmt.Errorf("army: research.yaml: duplicate node %q", n.ID)
		}
		nd.Fuel, nd.Hours = tier.Fuel, tier.Hours
		if n.Perk != nil {
			nd.Perk = n.Perk.Key
			switch v := n.Perk.Value.(type) {
			case int:
				nd.Value = v
			case bool:
				if v {
					nd.Value = 1
				}
			}
		}
		c.nodes[nd.ID] = nd
		c.nodeOrder = append(c.nodeOrder, nd.ID)
	}
	for _, nd := range c.nodes {
		for _, r := range nd.Requires {
			if _, ok := c.nodes[r]; !ok {
				return nil, fmt.Errorf("army: research.yaml: %s requires unknown node %q", nd.ID, r)
			}
		}
		if nd.Item != "" {
			if _, ok := c.units[nd.Item]; !ok {
				return nil, fmt.Errorf("army: research.yaml: %s: unknown item %q", nd.ID, nd.Item)
			}
		}
	}
	return c, nil
}

// LoadContent reads the four files from a content directory.
func LoadContent(dir string) (*Content, error) {
	var bs [4][]byte
	for i, f := range []string{"army.yaml", "troops.yaml", "buildings.yaml", "research.yaml"} {
		b, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			return nil, err
		}
		bs[i] = b
	}
	return ParseContent(bs[0], bs[1], bs[2], bs[3])
}

// Unit returns the unit or spell with id.
func (c *Content) Unit(id string) (Unit, bool) { u, ok := c.units[id]; return u, ok }

// Pools lists pool names in a stable order.
func (c *Content) Pools() []string {
	ps := make([]string, 0, len(c.buildingByPool))
	for p := range c.buildingByPool {
		ps = append(ps, p)
	}
	sort.Strings(ps)
	return ps
}
