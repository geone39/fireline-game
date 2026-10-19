package economy

import (
	"testing"
	"time"
)

func loadCfg(t *testing.T) *Config {
	t.Helper()
	c, err := Load("../../../content/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

// Expected values mirror the calibration table in docs/design/economy.md.
func TestTablesMatchDesign(t *testing.T) {
	c := loadCfg(t)
	cases := []struct {
		hq      int
		count   int
		rate    int64
		storage int64
	}{
		{1, 2, 40, 1500}, {2, 3, 60, 3000}, {5, 4, 280, 24000}, {8, 6, 1170, 192000}, {10, 6, 3070, 768000},
	}
	for _, tc := range cases {
		if got := c.CollectorCount(tc.hq); got != tc.count {
			t.Errorf("hq%d count=%d want %d", tc.hq, got, tc.count)
		}
		if got := c.RatePerHour(tc.hq); got != tc.rate {
			t.Errorf("hq%d rate=%d want %d", tc.hq, got, tc.rate)
		}
		if got := c.StorageCap(tc.hq); got != tc.storage {
			t.Errorf("hq%d storage=%d want %d", tc.hq, got, tc.storage)
		}
	}
	if c.StorageCap(99) != c.StorageCap(10) || c.StorageCap(0) != c.StorageCap(1) {
		t.Error("hq must be clamped to 1..max")
	}
}

func TestAccrueAndPoolCap(t *testing.T) {
	c := loadCfg(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	// HQ1: 2 collectors * 40/h = 80 cash/h; fuel at 55% = 44/h.
	p := c.Accrue(1, "cash", Pool{At: t0}, t0.Add(3*time.Hour))
	if p.Amount != 240 {
		t.Errorf("cash after 3h = %d, want 240", p.Amount)
	}
	if f := c.Accrue(1, "fuel", Pool{At: t0}, t0.Add(3*time.Hour)); f.Amount != 132 {
		t.Errorf("fuel after 3h = %d, want 132", f.Amount)
	}
	if g := c.Accrue(1, "gems", Pool{At: t0}, t0.Add(100*time.Hour)); g.Amount != 0 {
		t.Errorf("gems must not be produced, got %d", g.Amount)
	}
	// Offline cap = 50% of storage = 750 at HQ1; reached after ~9.4h.
	p = c.Accrue(1, "cash", Pool{At: t0}, t0.Add(48*time.Hour))
	if p.Amount != c.PoolCap(1) || c.PoolCap(1) != 750 {
		t.Errorf("pool = %d, cap = %d, want 750", p.Amount, c.PoolCap(1))
	}
	// Backwards clock: unchanged.
	if q := c.Accrue(1, "cash", Pool{Amount: 5, At: t0}, t0.Add(-time.Hour)); q.Amount != 5 {
		t.Errorf("backwards clock changed pool: %d", q.Amount)
	}
	// Accrual is additive over split intervals (no loss from integer seconds at the boundary).
	a := c.Accrue(2, "cash", c.Accrue(2, "cash", Pool{At: t0}, t0.Add(1000*time.Second)), t0.Add(2000*time.Second))
	b := c.Accrue(2, "cash", Pool{At: t0}, t0.Add(2000*time.Second))
	if a.Amount < b.Amount-1 || a.Amount > b.Amount {
		t.Errorf("split accrual %d vs single %d", a.Amount, b.Amount)
	}
}

func TestCollectRespectsStorage(t *testing.T) {
	c := loadCfg(t)
	t0 := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	now := t0.Add(48 * time.Hour) // pool full: 750
	got, pool := c.Collect(1, "cash", Pool{At: t0}, 0, now)
	if got != 750 || pool.Amount != 0 || !pool.At.Equal(now) {
		t.Errorf("collect to empty wallet: got=%d pool=%+v", got, pool)
	}
	// Wallet 1400 of 1500 -> only 100 fit; the rest stays in the pool.
	got, pool = c.Collect(1, "cash", Pool{At: t0}, 1400, now)
	if got != 100 || pool.Amount != 650 {
		t.Errorf("collect near cap: got=%d pool=%d", got, pool.Amount)
	}
	// Wallet already over the cap (HQ downgrade/loot refund): nothing taken, nothing lost.
	got, pool = c.Collect(1, "cash", Pool{At: t0}, 2000, now)
	if got != 0 || pool.Amount != 750 {
		t.Errorf("collect over cap: got=%d pool=%d", got, pool.Amount)
	}
	if c.ClampToStorage(1, 1400, 500) != 100 || c.ClampToStorage(1, 1600, 5) != 0 {
		t.Error("ClampToStorage")
	}
}

func TestParseRejectsBadConfig(t *testing.T) {
	if _, err := Parse([]byte("resources: {}\n")); err == nil {
		t.Error("expected error")
	}
	if _, err := Parse([]byte(":::")); err == nil {
		t.Error("expected error")
	}
}
