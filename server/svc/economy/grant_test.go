package economy_test

import (
	"context"
	"sync"
	"testing"

	"github.com/geone39/fireline/server/svc/economy"
)

func TestStartGrantFromContent(t *testing.T) {
	cfg, err := economy.Load("../../../content/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int64{}
	for _, d := range cfg.StartGrant() {
		got[d.Resource] = d.Amount
	}
	if got["cash"] != 1500 || got["fuel"] != 600 || len(got) != 2 {
		t.Fatalf("start grant = %v", got)
	}
	if _, err := economy.Parse([]byte("resources: {cash: {kind: soft}}\nprogression: {hq_max_release1: 1}\npassive: {collectors_count: {base: 1, hq_divisor: 1, max: 1}, rate_per_hour_each: {base: 1, growth: 1}, storage_per_resource: {base: 1, growth: 1}, offline_accrual_cap_pct_of_storage: 50}\nstart_grant: {ghost: 5}\n")); err == nil {
		t.Fatal("unknown start_grant resource accepted")
	}
}

func TestGrantStartOnce(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	r1, err := ledger.GrantStart(ctx, p)
	if err != nil || r1.Replayed || r1.Balances["cash"] != 1500 || r1.Balances["fuel"] != 600 {
		t.Fatalf("first grant: %+v %v", r1, err)
	}
	r2, err := ledger.GrantStart(ctx, p)
	if err != nil || !r2.Replayed {
		t.Fatalf("second grant: %+v %v", r2, err)
	}
	if bal(t, p, "cash") != 1500 || bal(t, p, "fuel") != 600 {
		t.Fatal("repeat changed the balance")
	}
	if n, sum := ledgerRows(t, p); n != 1 || sum != 1500 {
		t.Fatalf("cash ledger rows=%d sum=%d", n, sum)
	}
	var reason string
	if err := store.Pool.QueryRow(ctx, `SELECT reason FROM ledger WHERE player_id=$1 AND resource='fuel'`, p).Scan(&reason); err != nil || reason != "start_grant" {
		t.Fatalf("reason %q %v", reason, err)
	}
}

func TestGrantStartConcurrentAndPerPlayer(t *testing.T) {
	ctx := context.Background()
	a, b := newPlayer(t), newPlayer(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := ledger.GrantStart(ctx, a); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if bal(t, a, "cash") != 1500 || bal(t, a, "fuel") != 600 {
		t.Fatalf("concurrent grants: cash=%d fuel=%d", bal(t, a, "cash"), bal(t, a, "fuel"))
	}
	if r, err := ledger.GrantStart(ctx, b); err != nil || r.Replayed {
		t.Fatalf("other player must get their own grant: %+v %v", r, err)
	}
}

func TestGrantStartTxRollsBackWithCaller(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.GrantStartTx(ctx, tx, p); err != nil {
		t.Fatal(err)
	}
	_ = tx.Rollback(ctx)
	if bal(t, p, "cash") != 0 {
		t.Fatal("rolled-back grant leaked")
	}
	if r, err := ledger.GrantStart(ctx, p); err != nil || r.Replayed {
		t.Fatalf("grant after rollback: %+v %v", r, err)
	}
}
