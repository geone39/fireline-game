package base

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"

	"github.com/geone39/fireline/server/svc/economy"
)

// PriceBook holds every number the construction commands need: price and duration per
// building level, the gems speed-up curve, the cancel refund and the builder table.
// All values come from content/building_costs.yaml and economy.yaml; the package has
// no game constants of its own.
type PriceBook struct {
	// Entries is keyed by content id. Level n costs ceil(Cost*CostGrowth^(n-1)) and
	// takes ceil(TimeS*TimeGrowth^(n-1)) seconds (the formula in the buildings.yaml header).
	entries     map[string]priceEntry
	costGrowth  float64
	timeGrowth  float64
	speedFactor float64
	speedExp    float64
	refundPct   int64
	builders    []LimitStep // HQ level -> builders, sorted by At
}

type priceEntry struct {
	Cost       map[string]int64 `yaml:"cost"`
	TimeS      int64            `yaml:"time_s"`
	CostGrowth float64          `yaml:"cost_growth"`
	TimeGrowth float64          `yaml:"time_growth"`
	MaxLevel   int              `yaml:"max_level"`
}

type priceFile struct {
	Schema          int                                `yaml:"schema"`
	CostGrowth      float64                            `yaml:"cost_growth"`
	TimeGrowth      float64                            `yaml:"time_growth"`
	SpeedupGems     struct{ Factor, Exponent float64 } `yaml:"speedup_gems"`
	CancelRefundPct int64                              `yaml:"cancel_refund_pct"`
	Buildings       map[string]priceEntry              `yaml:"buildings"`
}

type buildersFile struct {
	Builders struct {
		ByHQ map[int]int `yaml:"hq_level"`
	} `yaml:"builders"`
}

// ParsePriceBook builds a PriceBook from the bytes of a pricing file (schema in
// docs/open-questions.md, OQ-B6) and economy.yaml (builders).
func ParsePriceBook(pricing, economyYAML []byte) (*PriceBook, error) {
	var pf priceFile
	if err := yaml.Unmarshal(pricing, &pf); err != nil {
		return nil, fmt.Errorf("base: pricing: %w", err)
	}
	var bf buildersFile
	if err := yaml.Unmarshal(economyYAML, &bf); err != nil {
		return nil, fmt.Errorf("base: economy.yaml: %w", err)
	}
	if pf.SpeedupGems.Factor <= 0 || pf.SpeedupGems.Exponent <= 0 {
		return nil, fmt.Errorf("base: pricing: speedup_gems.factor/exponent must be > 0")
	}
	if pf.CancelRefundPct < 0 || pf.CancelRefundPct > 100 {
		return nil, fmt.Errorf("base: pricing: cancel_refund_pct must be 0..100")
	}
	if len(bf.Builders.ByHQ) == 0 {
		return nil, fmt.Errorf("base: economy.yaml: builders.hq_level is empty")
	}
	pb := &PriceBook{
		entries: map[string]priceEntry{}, costGrowth: pf.CostGrowth, timeGrowth: pf.TimeGrowth,
		speedFactor: pf.SpeedupGems.Factor, speedExp: pf.SpeedupGems.Exponent, refundPct: pf.CancelRefundPct,
	}
	for id, e := range pf.Buildings {
		if e.CostGrowth == 0 {
			e.CostGrowth = pf.CostGrowth
		}
		if e.TimeGrowth == 0 {
			e.TimeGrowth = pf.TimeGrowth
		}
		if e.TimeS <= 0 || e.CostGrowth < 1 || e.TimeGrowth < 1 {
			return nil, fmt.Errorf("base: pricing: %s: need time_s > 0 and growth >= 1", id)
		}
		for r, a := range e.Cost {
			if a < 0 || r == "" {
				return nil, fmt.Errorf("base: pricing: %s: bad cost", id)
			}
		}
		pb.entries[id] = e
	}
	for at, n := range bf.Builders.ByHQ {
		pb.builders = append(pb.builders, LimitStep{at, n})
	}
	sort.Slice(pb.builders, func(i, j int) bool { return pb.builders[i].At < pb.builders[j].At })
	return pb, nil
}

// LoadPriceBook reads <contentDir>/building_costs.yaml and <contentDir>/economy.yaml.
func LoadPriceBook(contentDir string) (*PriceBook, error) {
	p, err := os.ReadFile(filepath.Join(contentDir, "building_costs.yaml"))
	if err != nil {
		return nil, err
	}
	e, err := os.ReadFile(filepath.Join(contentDir, "economy.yaml"))
	if err != nil {
		return nil, err
	}
	return ParsePriceBook(p, e)
}

// Has reports whether the type has a price entry (can be built / upgraded).
func (p *PriceBook) Has(typ string) bool { _, ok := p.entries[typ]; return ok }

// MaxLevel is the highest level of the type: the catalog's `levels`, else the pricing
// file's max_level, else 1.
func (p *PriceBook) MaxLevel(d Def) int {
	if d.Levels > 0 {
		return d.Levels
	}
	if e, ok := p.entries[d.ID]; ok && e.MaxLevel > 0 {
		return e.MaxLevel
	}
	return 1
}

// Cost is the price paid to reach `level` (1 = initial build).
func (p *PriceBook) Cost(typ string, level int) map[string]int64 {
	e := p.entries[typ]
	out := make(map[string]int64, len(e.Cost))
	k := math.Pow(e.CostGrowth, float64(level-1))
	for r, a := range e.Cost {
		out[r] = int64(math.Ceil(float64(a) * k))
	}
	return out
}

// Seconds is the construction time to reach `level`.
func (p *PriceBook) Seconds(typ string, level int) int64 {
	e := p.entries[typ]
	return int64(math.Ceil(float64(e.TimeS) * math.Pow(e.TimeGrowth, float64(level-1))))
}

// SpeedupGems is the gems price to finish a timer with secsLeft remaining:
// ceil(hours^exponent * factor), at least 1 while anything is left.
func (p *PriceBook) SpeedupGems(secsLeft int64) int64 {
	if secsLeft <= 0 {
		return 0
	}
	g := int64(math.Ceil(math.Pow(float64(secsLeft)/3600, p.speedExp) * p.speedFactor))
	return max(g, 1)
}

// Refund is the part of paid returned when a construction is cancelled.
func (p *PriceBook) Refund(paid map[string]int64) map[string]int64 {
	out := map[string]int64{}
	for r, a := range paid {
		if v := a * p.refundPct / 100; v > 0 {
			out[r] = v
		}
	}
	return out
}

// Builders is the number of parallel constructions at an HQ level.
func (p *PriceBook) Builders(hq int) int {
	n := 0
	for _, s := range p.builders {
		if s.At <= hq {
			n = s.Max
		}
	}
	return n
}

// Spend turns a price into negative ledger deltas (sorted: deterministic order).
func Spend(cost map[string]int64) []economy.Delta { return deltas(cost, -1) }

// Credit turns an amount map into positive ledger deltas.
func Credit(m map[string]int64) []economy.Delta { return deltas(m, 1) }

func deltas(m map[string]int64, sign int64) []economy.Delta {
	keys := make([]string, 0, len(m))
	for r, a := range m {
		if a != 0 {
			keys = append(keys, r)
		}
	}
	sort.Strings(keys)
	out := make([]economy.Delta, 0, len(keys))
	for _, r := range keys {
		out = append(out, economy.Delta{Resource: r, Amount: sign * m[r]})
	}
	return out
}
