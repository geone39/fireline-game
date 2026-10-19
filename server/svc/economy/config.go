// Package economy implements the Ledger (the only writer of wallets) and the pure
// collector/storage rules. All numbers come from content/economy.yaml.
package economy

import (
	"fmt"
	"math"
	"os"
	"time"

	"go.yaml.in/yaml/v3"
)

type growth struct {
	Base    float64 `yaml:"base"`
	Growth  float64 `yaml:"growth"`
	RoundTo int64   `yaml:"round_to"`
}

type countRule struct {
	Base      int `yaml:"base"`
	HQDivisor int `yaml:"hq_divisor"`
	Max       int `yaml:"max"`
}

// Config is the slice of content/economy.yaml used by the server economy.
type Config struct {
	Resources map[string]struct {
		Kind string `yaml:"kind"`
	} `yaml:"resources"`
	Progression struct {
		HQMax int `yaml:"hq_max_release1"`
	} `yaml:"progression"`
	Passive struct {
		Collectors      countRule      `yaml:"collectors_count"`
		Rate            growth         `yaml:"rate_per_hour_each"`
		Storage         growth         `yaml:"storage_per_resource"`
		ShareByResource map[string]int `yaml:"production_share_pct"`
		OfflineCapPct   int64          `yaml:"offline_accrual_cap_pct_of_storage"`
	} `yaml:"passive"`
	StartGrantAmounts map[string]int64 `yaml:"start_grant"`
}

// Parse decodes and validates economy.yaml bytes.
func Parse(b []byte) (*Config, error) {
	var c Config
	if err := yaml.Unmarshal(b, &c); err != nil {
		return nil, fmt.Errorf("economy: parse config: %w", err)
	}
	p := c.Passive
	switch {
	case len(c.Resources) == 0:
		return nil, fmt.Errorf("economy: config has no resources")
	case c.Progression.HQMax < 1:
		return nil, fmt.Errorf("economy: progression.hq_max_release1 must be >= 1")
	case p.Collectors.HQDivisor < 1 || p.Collectors.Max < 1:
		return nil, fmt.Errorf("economy: invalid passive.collectors_count")
	case p.Rate.Base <= 0 || p.Rate.Growth < 1 || p.Storage.Base <= 0 || p.Storage.Growth < 1:
		return nil, fmt.Errorf("economy: invalid passive rate/storage")
	case p.OfflineCapPct < 1 || p.OfflineCapPct > 100:
		return nil, fmt.Errorf("economy: offline_accrual_cap_pct_of_storage must be 1..100")
	}
	for r, a := range c.StartGrantAmounts {
		if _, ok := c.Resources[r]; !ok || a < 0 {
			return nil, fmt.Errorf("economy: bad start_grant for %q", r)
		}
	}
	for r, pct := range p.ShareByResource {
		if _, ok := c.Resources[r]; !ok || pct < 0 {
			return nil, fmt.Errorf("economy: bad production_share_pct for %q", r)
		}
	}
	return &c, nil
}

// Load reads and parses a config file.
func Load(path string) (*Config, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("economy: %w", err)
	}
	return Parse(b)
}

// HasResource reports whether id is a known resource.
func (c *Config) HasResource(id string) bool { _, ok := c.Resources[id]; return ok }

func (c *Config) clampHQ(hq int) int {
	return min(max(hq, 1), c.Progression.HQMax)
}

func roundTo(v float64, step int64) int64 {
	if step <= 1 {
		return int64(math.Round(v))
	}
	return int64(math.Round(v/float64(step))) * step
}

// CollectorCount is the number of collectors at an HQ level.
func (c *Config) CollectorCount(hq int) int {
	r := c.Passive.Collectors
	return min(r.Base+c.clampHQ(hq)/r.HQDivisor, r.Max)
}

// RatePerHour is the base yield of one collector per hour.
func (c *Config) RatePerHour(hq int) int64 {
	hq = c.clampHQ(hq)
	if hq == 1 {
		return int64(c.Passive.Rate.Base)
	}
	r := c.Passive.Rate
	return roundTo(r.Base*math.Pow(r.Growth, float64(hq-1)), r.RoundTo)
}

// StorageCap is the wallet ceiling per resource for collected income.
func (c *Config) StorageCap(hq int) int64 {
	s := c.Passive.Storage
	return roundTo(s.Base*math.Pow(s.Growth, float64(c.clampHQ(hq)-1)), s.RoundTo)
}

// PoolCap is the maximum a collector pool holds while the player is away.
func (c *Config) PoolCap(hq int) int64 {
	return c.StorageCap(hq) * c.Passive.OfflineCapPct / 100
}

// productionPct is the percent of the base rate collectors yield for a resource (0 = none).
func (c *Config) productionPct(resource string) int64 {
	return int64(c.Passive.ShareByResource[resource])
}

// Pool is the uncollected output of the collectors for one resource.
type Pool struct {
	Amount int64
	At     time.Time // last time Amount was brought up to date
}

// Accrue advances the pool to now. Output is count*rate*share per hour in integer
// seconds, capped at PoolCap. A clock that goes backwards leaves the pool unchanged.
func (c *Config) Accrue(hq int, resource string, p Pool, now time.Time) Pool {
	secs := int64(now.Sub(p.At) / time.Second)
	if secs <= 0 {
		return p
	}
	perHour := int64(c.CollectorCount(hq)) * c.RatePerHour(hq) * c.productionPct(resource) / 100
	// Bound the horizon (30 days) so the product cannot overflow.
	secs = min(secs, 30*24*3600)
	amt := min(p.Amount+perHour*secs/3600, c.PoolCap(hq))
	if amt < p.Amount { // pool already above the cap (e.g. cap shrank): never destroy it
		amt = p.Amount
	}
	return Pool{Amount: amt, At: p.At.Add(time.Duration(secs) * time.Second)}
}

// Collect moves pool contents into the wallet, limited by storage headroom. The remainder
// stays in the pool. It returns the credited amount and the new pool.
func (c *Config) Collect(hq int, resource string, p Pool, walletAmount int64, now time.Time) (int64, Pool) {
	p = c.Accrue(hq, resource, p, now)
	p.At = now
	headroom := max(c.StorageCap(hq)-walletAmount, 0)
	take := min(p.Amount, headroom)
	p.Amount -= take
	return take, p
}

// ClampToStorage returns how much of a credit fits under the storage ceiling.
func (c *Config) ClampToStorage(hq int, walletAmount, credit int64) int64 {
	return min(credit, max(c.StorageCap(hq)-walletAmount, 0))
}
