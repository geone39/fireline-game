package economy_test

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/geone39/fireline/server/db"
	"github.com/geone39/fireline/server/svc/economy"
)

var (
	store  *db.Store
	ledger *economy.Ledger
)

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("fireline"), tcpg.WithUsername("fireline"), tcpg.WithPassword("fireline"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	if err != nil {
		if os.Getenv("REQUIRE_DOCKER") == "" {
			println("SKIP: docker unavailable:", err.Error())
			os.Exit(0)
		}
		println("FAIL: docker unavailable:", err.Error())
		os.Exit(1)
	}
	dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
	if err != nil {
		panic(err)
	}
	if store, err = db.Connect(ctx, dsn); err != nil {
		panic(err)
	}
	if err := store.Migrate(ctx); err != nil {
		panic(err)
	}
	cfg, err := economy.Load("../../../content/economy.yaml")
	if err != nil {
		panic(err)
	}
	ledger = economy.NewLedger(store.Pool, cfg)
	code := m.Run()
	store.Close()
	_ = testcontainers.TerminateContainer(ctr)
	os.Exit(code)
}

func newPlayer(t *testing.T) uuid.UUID {
	t.Helper()
	p, err := store.UpsertPlayer(context.Background(), "test", uuid.NewString(), "p")
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

func bal(t *testing.T, p uuid.UUID, res string) int64 {
	t.Helper()
	w, err := ledger.Balances(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	return w[res]
}

func ledgerRows(t *testing.T, p uuid.UUID) (n int, sum int64) {
	t.Helper()
	err := store.Pool.QueryRow(context.Background(),
		`SELECT count(*), COALESCE(sum(delta),0) FROM ledger WHERE player_id=$1 AND resource='cash'`, p).Scan(&n, &sum)
	if err != nil {
		t.Fatal(err)
	}
	return
}

func TestApplyIdempotentReplay(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	d := []economy.Delta{{"cash", 500}, {"fuel", 200}}
	r1, err := ledger.Apply(ctx, p, d, "test_grant", "k1")
	if err != nil || r1.Replayed || r1.Balances["cash"] != 500 {
		t.Fatalf("first apply: %+v %v", r1, err)
	}
	r2, err := ledger.Apply(ctx, p, d, "test_grant", "k1")
	if err != nil || !r2.Replayed || r2.Balances["cash"] != 500 || r2.Balances["fuel"] != 200 {
		t.Fatalf("replay: %+v %v", r2, err)
	}
	if bal(t, p, "cash") != 500 || bal(t, p, "fuel") != 200 {
		t.Fatal("replay changed the balance")
	}
	if n, _ := ledgerRows(t, p); n != 1 {
		t.Fatalf("ledger rows for cash = %d, want 1", n)
	}
}

func TestIdemKeyIsPerPlayer(t *testing.T) {
	ctx := context.Background()
	a, b := newPlayer(t), newPlayer(t)
	for _, p := range []uuid.UUID{a, b} {
		if r, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 10}}, "g", "same"); err != nil || r.Replayed {
			t.Fatalf("player %s: %+v %v", p, r, err)
		}
	}
	if bal(t, a, "cash") != 10 || bal(t, b, "cash") != 10 {
		t.Fatal("balances")
	}
}

func TestInsufficientFundsChangesNothing(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 100}, {"fuel", 100}}, "g", "seed"); err != nil {
		t.Fatal(err)
	}
	// fuel sorts after cash: the cash debit succeeds first, then fuel fails -> all rolled back.
	_, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", -50}, {"fuel", -101}}, "spend", "s1")
	if !errors.Is(err, economy.ErrInsufficientFunds) {
		t.Fatalf("err = %v", err)
	}
	if bal(t, p, "cash") != 100 || bal(t, p, "fuel") != 100 {
		t.Fatal("partial apply leaked")
	}
	if n, _ := ledgerRows(t, p); n != 1 {
		t.Fatalf("failed apply left ledger rows: %d", n)
	}
	// A failed key is not burned: retrying the same key with affordable amounts works.
	if r, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", -50}}, "spend", "s1"); err != nil || r.Replayed {
		t.Fatalf("retry: %+v %v", r, err)
	}
	// Debit from a resource the player never had.
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"alloy", -1}}, "spend", "s2"); !errors.Is(err, economy.ErrInsufficientFunds) {
		t.Fatalf("err = %v", err)
	}
}

func TestValidation(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 1}}, "g", ""); !errors.Is(err, economy.ErrInvalid) {
		t.Errorf("empty idemKey: %v", err)
	}
	if _, err := ledger.Apply(ctx, p, nil, "g", "v1"); !errors.Is(err, economy.ErrInvalid) {
		t.Errorf("no deltas: %v", err)
	}
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 0}}, "g", "v2"); !errors.Is(err, economy.ErrInvalid) {
		t.Errorf("zero delta: %v", err)
	}
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"mana", 1}}, "g", "v3"); !errors.Is(err, economy.ErrUnknownResource) {
		t.Errorf("unknown resource: %v", err)
	}
	if _, err := ledger.Apply(ctx, uuid.New(), []economy.Delta{{"cash", 1}}, "g", "v4"); !errors.Is(err, economy.ErrPlayerNotFound) {
		t.Errorf("missing player: %v", err)
	}
}

func TestLedgerIsAppendOnly(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 7}}, "g", "ao"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Pool.Exec(ctx, `UPDATE ledger SET delta = 1000 WHERE player_id = $1`, p); err == nil {
		t.Error("UPDATE on ledger must fail")
	}
	if _, err := store.Pool.Exec(ctx, `DELETE FROM ledger WHERE player_id = $1`, p); err == nil {
		t.Error("DELETE on ledger must fail")
	}
}

// Many goroutines race on one idemKey: exactly one applies, the rest replay.
func TestConcurrentSameIdemKey(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	const n = 32
	var wg sync.WaitGroup
	var mu sync.Mutex
	applied, replayed := 0, 0
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 100}}, "race", "one-key")
			if err != nil {
				errs <- err
				return
			}
			mu.Lock()
			defer mu.Unlock()
			if r.Replayed {
				replayed++
			} else {
				applied++
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Fatal(err)
	}
	if applied != 1 || replayed != n-1 {
		t.Fatalf("applied=%d replayed=%d", applied, replayed)
	}
	if bal(t, p, "cash") != 100 {
		t.Fatalf("balance = %d, want 100", bal(t, p, "cash"))
	}
}

// Distinct keys in parallel: no lost updates, and wallets == SUM(ledger).
func TestConcurrentDistinctKeysNoLostUpdates(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	const n = 50
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 10}}, "race", fmt.Sprintf("k%d", i))
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	rows, sum := ledgerRows(t, p)
	if bal(t, p, "cash") != n*10 || rows != n || sum != n*10 {
		t.Fatalf("balance=%d rows=%d sum=%d", bal(t, p, "cash"), rows, sum)
	}
}

// Concurrent spends of a limited balance: balance never goes negative, exactly floor(B/cost) succeed.
func TestConcurrentSpendNeverOverdraws(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	if _, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", 1000}}, "g", "seed"); err != nil {
		t.Fatal(err)
	}
	const n = 40
	var wg sync.WaitGroup
	res := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ledger.Apply(ctx, p, []economy.Delta{{"cash", -100}}, "spend", fmt.Sprintf("s%d", i))
			res <- err
		}()
	}
	wg.Wait()
	close(res)
	ok := 0
	for err := range res {
		switch {
		case err == nil:
			ok++
		case !errors.Is(err, economy.ErrInsufficientFunds):
			t.Fatal(err)
		}
	}
	if ok != 10 || bal(t, p, "cash") != 0 {
		t.Fatalf("successful spends=%d balance=%d", ok, bal(t, p, "cash"))
	}
	if rows, sum := ledgerRows(t, p); rows != 11 || sum != 0 {
		t.Fatalf("rows=%d sum=%d", rows, sum)
	}
}

func TestApplyTxRollsBackWithCaller(t *testing.T) {
	ctx := context.Background()
	p := newPlayer(t)
	tx, err := store.Pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ledger.ApplyTx(ctx, tx, p, []economy.Delta{{"cash", 5}}, "g", "tx1"); err != nil {
		t.Fatal(err)
	}
	// A failing ApplyTx must leave the caller's tx usable.
	if _, err := ledger.ApplyTx(ctx, tx, p, []economy.Delta{{"cash", -99}}, "g", "tx2"); !errors.Is(err, economy.ErrInsufficientFunds) {
		t.Fatalf("err = %v", err)
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	if bal(t, p, "cash") != 0 {
		t.Fatal("rollback did not undo ledger apply")
	}
}
