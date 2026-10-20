package army

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/geone39/fireline/server/db"
	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/economy"
)

const contentDir = "../../../content"

// The database starts lazily so the pure tests never need Docker.
var (
	storeOnce sync.Once
	store     *db.Store
	storeErr  error
)

func testStore(t *testing.T) *db.Store {
	t.Helper()
	storeOnce.Do(func() {
		ctx := context.Background()
		ctr, err := tcpg.Run(ctx, "postgres:16-alpine",
			tcpg.WithDatabase("fireline"), tcpg.WithUsername("fireline"), tcpg.WithPassword("fireline"),
			testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
		if err != nil {
			storeErr = err
			return
		}
		dsn, err := ctr.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			storeErr = err
			return
		}
		if store, storeErr = db.Connect(ctx, dsn); storeErr == nil {
			storeErr = store.Migrate(ctx)
		}
	})
	if storeErr != nil {
		if os.Getenv("REQUIRE_DOCKER") == "" {
			t.Skipf("docker unavailable: %v", storeErr)
		}
		t.Fatalf("docker unavailable: %v", storeErr)
	}
	return store
}

var t0 = time.Date(2026, 10, 20, 12, 0, 0, 0, time.UTC)

type env struct {
	t      *testing.T
	svc    *Service
	ledger *economy.Ledger
	player uuid.UUID
	now    time.Time
}

func loadContent(t *testing.T) *Content {
	t.Helper()
	c, err := LoadContent(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func it(uid, typ string, level int) base.Item { return base.Item{UID: uid, Type: typ, Level: level} }

// standard is the F-ARMY-1 fixture (army-lab.md §9): HQ3, two camps L1, barracks A and B
// L2, a factory, a garage and a support bay.
func standard() []base.Item {
	return []base.Item{
		it("hq", "b_hq", 3), it("camp1", "b_camp", 1), it("camp2", "b_camp", 1),
		it("A", "b_barracks", 2), it("B", "b_barracks", 2),
		it("fac", "b_factory", 1), it("gar", "b_garage", 1), it("sb", "b_support_bay", 1),
	}
}

func newEnv(t *testing.T, items []base.Item) *env {
	t.Helper()
	st := testStore(t)
	econ, err := economy.Load(contentDir + "/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cat, err := base.LoadCatalog(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	pb, err := base.LoadPriceBook(contentDir)
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, now: t0, ledger: economy.NewLedger(st.Pool, econ)}
	p, err := st.UpsertPlayer(context.Background(), "test", uuid.NewString(), "p")
	if err != nil {
		t.Fatal(err)
	}
	e.player = p.ID
	e.svc, err = NewService(Config{DB: st.Pool, Ledger: e.ledger, Content: loadContent(t), Catalog: cat, Prices: pb, Now: func() time.Time { return e.now }})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(base.Layout{Items: items})
	e.exec(`INSERT INTO base_state (player_id, layout) VALUES ($1, $2::jsonb)`, e.player, string(raw))
	return e
}

func (e *env) exec(q string, args ...any) {
	e.t.Helper()
	if _, err := testStore(e.t).Pool.Exec(context.Background(), q, args...); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) grant(res string, n int64) {
	e.t.Helper()
	if _, err := e.ledger.Apply(context.Background(), e.player, []economy.Delta{{Resource: res, Amount: n}}, "test_grant", uuid.NewString()); err != nil {
		e.t.Fatal(err)
	}
}

func (e *env) bal(res string) int64 {
	e.t.Helper()
	b, err := e.ledger.Balances(context.Background(), e.player)
	if err != nil {
		e.t.Fatal(err)
	}
	return b[res]
}

func (e *env) setStock(id string, n int) {
	e.exec(`INSERT INTO army (player_id, unit_id, level, count) VALUES ($1, $2, 1, $3)
		ON CONFLICT (player_id, unit_id, level) DO UPDATE SET count = EXCLUDED.count`, e.player, id, n)
}

func (e *env) research(node string) {
	e.exec(`INSERT INTO research (player_id, node_id, state) VALUES ($1, $2, 'done')`, e.player, node)
}

func (e *env) timers(kind string) int {
	var n int
	if err := testStore(e.t).Pool.QueryRow(context.Background(), `SELECT count(*) FROM timers WHERE player_id = $1 AND kind = $2`, e.player, kind).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) train(key, building, unit string, n int) (State, error) {
	return e.svc.Train(context.Background(), e.player, key, Request{BuildingID: building, UnitID: unit, Count: n})
}

func (e *env) mustTrain(key, building, unit string, n int) State {
	e.t.Helper()
	st, err := e.train(key, building, unit, n)
	if err != nil {
		e.t.Fatalf("train %s x%d in %s: %v", unit, n, building, err)
	}
	return st
}

func (e *env) get() State {
	e.t.Helper()
	st, err := e.svc.Get(context.Background(), e.player)
	if err != nil {
		e.t.Fatal(err)
	}
	return st
}

func (e *env) at(sec int) { e.now = t0.Add(time.Duration(sec) * time.Second) }

func stockOf(st State, id string) int {
	for _, c := range append(append([]Count(nil), st.Stock...), st.Spells...) {
		if c.ID == id {
			return c.Count
		}
	}
	return 0
}

func wantErr(t *testing.T, err error, kind, reason string) *Error {
	t.Helper()
	var ae *Error
	if !errors.As(err, &ae) || ae.Kind != kind || ae.Reason != reason {
		t.Fatalf("want %s/%s, got %v", kind, reason, err)
	}
	return ae
}

func queueOf(st State, b string) QueueView {
	for _, q := range st.Queues {
		if q.BuildingID == b {
			return q
		}
	}
	return QueueView{}
}

// army-lab.md §9.1: overflow rejects the whole order, parallel queues, perk, speed-up, cancel.
func TestHousingScenario(t *testing.T) {
	e := newEnv(t, standard())
	e.grant("fuel", 2000)
	e.setStock("u_rifleman", 12)
	e.setStock("u_sniper", 2)

	st := e.mustTrain("k1", "A", "u_brawler", 3) // 12 housing
	if p := st.Pools["unit"]; p.Capacity != 40 || p.Used != 28 || p.Free != 12 {
		t.Fatalf("pool: %+v", p)
	}
	// B asks for 4 brawlers (16 > 12): rejected whole, nothing charged.
	fuel := e.bal("fuel")
	_, err := e.train("k2", "B", "u_brawler", 4)
	ae := wantErr(t, err, "conflict", "housing_full")
	if ae.Details["max_count"] != 3 || ae.Details["free"] != 12 || ae.Details["need"] != 16 || ae.Details["capacity"] != 40 || ae.Details["used"] != 28 {
		t.Fatalf("details: %v", ae.Details)
	}
	if e.bal("fuel") != fuel || e.timers("train") != 1 {
		t.Fatal("a rejected order must change nothing")
	}
	st = e.mustTrain("k3", "B", "u_brawler", 3)
	if e.bal("fuel") != fuel-330 {
		t.Fatalf("fuel: %d", e.bal("fuel"))
	}
	if p := st.Pools["unit"]; p.Used != 40 || p.Free != 0 {
		t.Fatalf("pool: %+v", p)
	}
	// Both queues run in parallel and end together: 3 * 45 s.
	if a, b := queueOf(st, "A"), queueOf(st, "B"); a.FinishAt != t0.Unix()+135 || b.FinishAt != a.FinishAt {
		t.Fatalf("queues: %+v %+v", a, b)
	}
	_, err = e.train("k4", "A", "u_rifleman", 1)
	if ae := wantErr(t, err, "conflict", "housing_full"); ae.Details["max_count"] != 0 {
		t.Fatalf("details: %v", ae.Details)
	}

	// The lab perk (+5%) frees two places: floor(40*1050/1000) = 42.
	e.research("r_camp_cap")
	st = e.get()
	if p := st.Pools["unit"]; p.Capacity != 42 || p.Free != 2 || p.PerkPct != 5 || p.Base != 40 {
		t.Fatalf("pool with perk: %+v", p)
	}
	st = e.mustTrain("k5", "A", "u_rifleman", 2) // starts after A's 135 s: ends at 135+24
	if got := queueOf(st, "A").FinishAt; got != t0.Unix()+159 {
		t.Fatalf("A finishes at +%d", got-t0.Unix())
	}

	// Speed-up of A: 159 s left -> 1 gem; everything lands in the stock, housing is unchanged.
	if _, err := e.svc.Speedup(context.Background(), e.player, "s0", "A"); err == nil {
		t.Fatal("no gems: want an error")
	} else {
		wantErr(t, err, "insufficient_resources", "insufficient_resources")
	}
	e.grant("gems", 5)
	st, err = e.svc.Speedup(context.Background(), e.player, "s1", "A")
	if err != nil {
		t.Fatal(err)
	}
	if e.bal("gems") != 4 || stockOf(st, "u_brawler") != 3 || stockOf(st, "u_rifleman") != 14 || len(queueOf(st, "A").Orders) != 0 {
		t.Fatalf("after speedup: gems %d, %+v", e.bal("gems"), st.Stock)
	}
	if st.Pools["unit"].Used != 42 {
		t.Fatalf("used: %+v", st.Pools["unit"])
	}
	// A replayed speed-up charges nothing; an empty queue is invalid.
	if _, err := e.svc.Speedup(context.Background(), e.player, "s1", "A"); err != nil || e.bal("gems") != 4 {
		t.Fatalf("replay: %v gems %d", err, e.bal("gems"))
	}
	_, err = e.svc.Speedup(context.Background(), e.player, "s2", "A")
	wantErr(t, err, "invalid", "empty_queue")

	// B at +50 s: one brawler is done, one in progress, one not started.
	e.at(50)
	st = e.get()
	if stockOf(st, "u_brawler") != 4 {
		t.Fatalf("brawlers at +50: %d", stockOf(st, "u_brawler"))
	}
	ord := queueOf(st, "B").Orders[0]
	fuel = e.bal("fuel")
	st, err = e.svc.Cancel(context.Background(), e.player, "c1", ord.OrderID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := e.bal("fuel") - fuel; got != 55+110 { // 50% of the running unit + 100% of the unstarted
		t.Fatalf("refund %d", got)
	}
	if len(st.Queues) != 1 && len(queueOf(st, "B").Orders) != 0 {
		t.Fatalf("queues: %+v", st.Queues)
	}
	if st.Pools["unit"].Used != 42-8 {
		t.Fatalf("used: %+v", st.Pools["unit"])
	}
	// The same cancel key again refunds nothing and does not fail.
	if _, err := e.svc.Cancel(context.Background(), e.player, "c1", ord.OrderID, 0); err != nil || e.bal("fuel") != fuel+165 {
		t.Fatalf("replayed cancel: %v fuel %d", err, e.bal("fuel"))
	}
	if _, err := e.svc.Cancel(context.Background(), e.player, "c2", ord.OrderID, 0); err == nil {
		t.Fatal("a vanished order must be not_found")
	} else {
		wantErr(t, err, "not_found", "no_order")
	}
}

func TestTimeForwardBackAndOffline(t *testing.T) {
	e := newEnv(t, standard())
	e.grant("fuel", 2000)
	e.mustTrain("k1", "A", "u_brawler", 3)
	e.mustTrain("k2", "A", "u_rifleman", 2) // queued behind the brawlers

	e.at(44)
	if n := stockOf(e.get(), "u_brawler"); n != 0 {
		t.Fatalf("+44: %d", n)
	}
	e.at(45)
	if n := stockOf(e.get(), "u_brawler"); n != 1 {
		t.Fatalf("+45: %d", n)
	}
	e.at(30) // the clock jumps back: nothing is taken back, nothing new arrives
	st := e.get()
	if stockOf(st, "u_brawler") != 1 || stockOf(st, "u_rifleman") != 0 {
		t.Fatalf("back in time: %+v", st.Stock)
	}
	e.at(135 + 12) // all brawlers and the first rifleman
	st = e.get()
	if stockOf(st, "u_brawler") != 3 || stockOf(st, "u_rifleman") != 1 {
		t.Fatalf("+147: %+v", st.Stock)
	}
	again := e.get() // settle twice at the same time: idempotent
	if stockOf(again, "u_brawler") != 3 || stockOf(again, "u_rifleman") != 1 {
		t.Fatalf("second get: %+v", again.Stock)
	}
	e.at(200)
	st = e.get()
	if stockOf(st, "u_rifleman") != 2 || e.timers("train") != 0 || len(st.Queues) != 0 {
		t.Fatalf("done: %+v queues %d", st.Stock, e.timers("train"))
	}
	e.now = t0.Add(30 * 24 * time.Hour) // US-A12: the army does not rot
	if st = e.get(); stockOf(st, "u_brawler") != 3 || stockOf(st, "u_rifleman") != 2 {
		t.Fatalf("after 30 days: %+v", st.Stock)
	}
}

func TestIdempotentTrain(t *testing.T) {
	e := newEnv(t, standard())
	e.grant("fuel", 2000)
	st1 := e.mustTrain("same", "A", "u_brawler", 3)
	fuel := e.bal("fuel")
	// Fill the camps so that validation alone would now reject the same request.
	e.setStock("u_rifleman", 28)
	st2 := e.mustTrain("same", "A", "u_brawler", 3)
	if e.bal("fuel") != fuel || e.timers("train") != 1 || len(queueOf(st2, "A").Orders) != 1 {
		t.Fatalf("replay changed things: fuel %d timers %d", e.bal("fuel"), e.timers("train"))
	}
	if queueOf(st1, "A").Orders[0].OrderID != queueOf(st2, "A").Orders[0].OrderID {
		t.Fatal("replay must return the same order")
	}
	if _, err := e.svc.Train(context.Background(), e.player, "", Request{BuildingID: "A", UnitID: "u_rifleman", Count: 1}); err == nil {
		t.Fatal("an empty idempotency key must be refused")
	}
}

func TestInsufficientResourcesIsAtomic(t *testing.T) {
	e := newEnv(t, standard())
	e.grant("fuel", 20)
	_, err := e.train("k1", "A", "u_rifleman", 1) // 25 fuel
	ae := wantErr(t, err, "insufficient_resources", "insufficient_resources")
	if m, _ := ae.Details["missing"].(map[string]int64); m["fuel"] != 5 {
		t.Fatalf("details: %v", ae.Details)
	}
	if e.bal("fuel") != 20 || e.timers("train") != 0 || stockOf(e.get(), "u_rifleman") != 0 {
		t.Fatal("a failed purchase must change nothing")
	}
	e.grant("fuel", 5)
	if _, err := e.train("k1", "A", "u_rifleman", 1); err != nil { // same key, now affordable
		t.Fatalf("retry with the same key: %v", err)
	}
}

func TestValidationOrder(t *testing.T) {
	items := append(standard(), it("C", "b_barracks", 1))
	e := newEnv(t, items)
	e.grant("fuel", 100000)
	e.grant("cash", 100000)
	// A build timer that has not run out: barracks D is still under construction.
	raw, _ := json.Marshal(base.Layout{Items: append(items, it("D", "b_barracks", 1))})
	e.exec(`UPDATE base_state SET layout = $2::jsonb WHERE player_id = $1`, e.player, string(raw))
	e.exec(`INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload) VALUES ($1, $2, 'build', 'D', $3, '{"to_level":1}')`,
		uuid.New(), e.player, t0.Add(time.Hour))

	_, err := e.train("a", "nope", "u_rifleman", 1)
	wantErr(t, err, "not_found", "no_building")
	_, err = e.train("b", "fac", "u_rifleman", 1)
	wantErr(t, err, "invalid", "wrong_building")
	_, err = e.train("c", "D", "u_rifleman", 1)
	wantErr(t, err, "invalid", "not_built")
	_, err = e.train("d", "A", "u_sniper", 1) // barracks 3 + hq 3 needed; A is L2
	ae := wantErr(t, err, "conflict", "locked")
	if need, _ := ae.Details["need"].(map[string]int); need["b_barracks"] != 3 || need["hq"] != 3 {
		t.Fatalf("need: %v", ae.Details)
	}
	_, err = e.train("e", "A", "u_rifleman", 0)
	wantErr(t, err, "invalid", "bad_request")
	_, err = e.train("f", "A", "u_nobody", 1)
	wantErr(t, err, "invalid", "bad_request")

	// The build finished: the same barracks now works (settle by time).
	e.at(3601)
	e.mustTrain("g", "D", "u_rifleman", 1)

	// Vehicles use cash and the garage pool; spells the support bay pool.
	st := e.mustTrain("h", "fac", "v_humvee", 2)
	if g := st.Pools["garage"]; g.Capacity != 20 || g.Used != 10 {
		t.Fatalf("garage: %+v", g)
	}
	_, err = e.train("i", "fac", "v_humvee", 3)
	ae = wantErr(t, err, "conflict", "housing_full")
	if ae.Details["pool"] != "garage" || ae.Details["free"] != 10 {
		t.Fatalf("details: %v", ae.Details)
	}
	st = e.mustTrain("j", "sb", "c_smoke", 10)
	if s := st.Pools["spell"]; s.Capacity != 10 || s.Used != 10 {
		t.Fatalf("spell pool: %+v", s)
	}
	_, err = e.train("k", "sb", "c_smoke", 1)
	wantErr(t, err, "conflict", "housing_full")
}

func TestQueueFull(t *testing.T) {
	e := newEnv(t, standard())
	e.grant("fuel", 100000)
	max := e.svc.cfg.Content.MaxOrders
	for i := 0; i < max; i++ {
		e.mustTrain(string(rune('a'+i)), "A", "u_rifleman", 1)
	}
	_, err := e.train("z", "A", "u_rifleman", 1)
	wantErr(t, err, "conflict", "queue_full")
	e.mustTrain("y", "B", "u_rifleman", 1) // another building has its own queue
}

func TestBuildingCapacityByState(t *testing.T) {
	e := newEnv(t, append(standard(), it("camp3", "b_camp", 1)))
	// camp3 is a first construction in progress: it adds nothing until it finishes.
	e.exec(`INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload) VALUES ($1, $2, 'build', 'camp3', $3, '{"to_level":1}')`,
		uuid.New(), e.player, t0.Add(time.Hour))
	// camp1 is being upgraded to L2: it keeps working at L1 (20) until the timer runs out.
	e.exec(`INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload) VALUES ($1, $2, 'upgrade', 'camp1', $3, '{"to_level":2}')`,
		uuid.New(), e.player, t0.Add(2*time.Hour))
	if c := e.get().Pools["unit"].Capacity; c != 40 {
		t.Fatalf("capacity: %d", c)
	}
	e.at(3601)
	if c := e.get().Pools["unit"].Capacity; c != 60 {
		t.Fatalf("after the build: %d", c)
	}
	e.at(7201) // camp1 L2 = 26 per the content table
	if c := e.get().Pools["unit"].Capacity; c != 26+20+20 {
		t.Fatalf("after the upgrade: %d", c)
	}
}

func TestLevelsComeFromResearch(t *testing.T) {
	e := newEnv(t, standard())
	e.setStock("u_rifleman", 3)
	e.setStock("u_brawler", 1)
	e.research("r_rifle_1")
	st := e.get()
	for _, c := range st.Stock {
		want := map[string]int{"u_rifleman": 2, "u_brawler": 1}[c.ID]
		if c.Level != want {
			t.Fatalf("%s level %d, want %d", c.ID, c.Level, want)
		}
	}
	e.research("r_rifle_2") // to_level 4
	if l := e.get().Stock[1].Level; l != 4 {
		t.Fatalf("level %d", l)
	}
}

func TestFormArmyAndResolve(t *testing.T) {
	e := newEnv(t, standard())
	e.setStock("u_rifleman", 10)
	e.setStock("u_brawler", 2)
	e.setStock("s_gunboat", 1)
	e.setStock("c_smoke", 2)
	e.research("r_rifle_1")
	ctx := context.Background()
	inTx := func(f func(tx pgx.Tx) error) error {
		tx, err := testStore(t).Pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = tx.Rollback(ctx) }()
		if err := f(tx); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	used := e.get().Pools["unit"].Used

	var f Formation
	err := inTx(func(tx pgx.Tx) error {
		var err error
		f, err = e.svc.FormArmy(ctx, tx, e.player, "raid-1", []Pick{{"u_rifleman", 6}, {"u_brawler", 2}}, []Pick{{"c_smoke", 2}}, false, t0.Add(time.Hour))
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(f.Units) != 2 || f.Units[0] != (Slot{"u_rifleman", 2, 6}) || f.Units[1].Level != 1 || f.Spells[0] != (Slot{"c_smoke", 1, 2}) {
		t.Fatalf("formation: %+v", f)
	}
	st := e.get()
	if stockOf(st, "u_rifleman") != 4 || stockOf(st, "u_brawler") != 0 || stockOf(st, "c_smoke") != 0 || st.Pools["unit"].Used != used {
		t.Fatalf("escrow: %+v used %d", st.Stock, st.Pools["unit"].Used)
	}
	if len(st.Deployed) != 1 || st.Deployed[0].RaidID != "raid-1" {
		t.Fatalf("deployed: %+v", st.Deployed)
	}
	// A research finished mid-battle does not change the formed levels; the same raid id returns the escrow.
	e.research("r_rifle_2")
	err = inTx(func(tx pgx.Tx) error {
		g, err := e.svc.FormArmy(ctx, tx, e.player, "raid-1", nil, nil, false, t0)
		if err == nil && g.Units[0].Level != 2 {
			t.Errorf("replayed formation: %+v", g)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Rejections change nothing.
	for name, c := range map[string]struct {
		units  []Pick
		water  bool
		reason string
	}{
		"no stock":      {[]Pick{{"u_rifleman", 5}}, false, "no_stock"},
		"no water":      {[]Pick{{"s_gunboat", 1}}, false, "ship_no_water"},
		"unknown":       {[]Pick{{"u_ghost", 1}}, false, "bad_selection"},
		"spell as unit": {[]Pick{{"c_smoke", 1}}, true, "bad_selection"},
	} {
		err := inTx(func(tx pgx.Tx) error {
			_, err := e.svc.FormArmy(ctx, tx, e.player, "raid-x", c.units, nil, c.water, t0)
			return err
		})
		if ae := new(Error); !errors.As(err, &ae) || ae.Reason != c.reason {
			t.Fatalf("%s: %v", name, err)
		}
	}
	err = inTx(func(tx pgx.Tx) error {
		_, err := e.svc.FormArmy(ctx, tx, e.player, "raid-w", []Pick{{"s_gunboat", 1}}, nil, true, t0)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}

	// Result: 2 riflemen and 1 brawler lost, 1 smoke used. Survivors and the unused spell return.
	var applied bool
	for i := 0; i < 2; i++ { // the second resolve is a no-op
		err = inTx(func(tx pgx.Tx) error {
			var err error
			applied, err = e.svc.Resolve(ctx, tx, e.player, "raid-1", Outcome{UnitsLost: []Pick{{"u_rifleman", 2}, {"u_brawler", 1}}, SpellsUsed: []Pick{{"c_smoke", 1}}})
			return err
		})
		if err != nil || applied != (i == 0) {
			t.Fatalf("resolve #%d: %v applied=%v", i, err, applied)
		}
	}
	st = e.get()
	if stockOf(st, "u_rifleman") != 8 || stockOf(st, "u_brawler") != 1 || stockOf(st, "c_smoke") != 1 || len(st.Deployed) != 1 {
		t.Fatalf("after the raid: %+v %+v deployed %d", st.Stock, st.Spells, len(st.Deployed))
	}
	// A forged outcome is refused and leaves the escrow in place.
	err = inTx(func(tx pgx.Tx) error {
		_, err := e.svc.Resolve(ctx, tx, e.player, "raid-w", Outcome{UnitsLost: []Pick{{"s_gunboat", 2}}})
		return err
	})
	if ae := new(Error); !errors.As(err, &ae) || ae.Reason != "bad_outcome" {
		t.Fatalf("forged outcome: %v", err)
	}
	// Release returns everything.
	err = inTx(func(tx pgx.Tx) error { _, err := e.svc.Release(ctx, tx, e.player, "raid-w"); return err })
	if err != nil || stockOf(e.get(), "s_gunboat") != 1 || e.timers("deploy") != 0 {
		t.Fatalf("release: %v", err)
	}
}

func TestHTTP(t *testing.T) {
	e := newEnv(t, standard())
	e.grant("fuel", 2000)
	r := chi.NewRouter()
	Mount(r, Deps{Service: e.svc, PlayerID: func(context.Context) (uuid.UUID, bool) { return e.player, true },
		ContentVersion: func() string { return "cv" }, Now: func() time.Time { return e.now }})
	do := func(method, path, key, body string) (int, map[string]any) {
		req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		var m map[string]any
		_ = json.Unmarshal(rec.Body.Bytes(), &m)
		return rec.Code, m
	}
	if c, m := do("POST", "/army/train", "", `{"building_id":"A","unit_id":"u_rifleman","count":1}`); c != http.StatusBadRequest {
		t.Fatalf("no key: %d %v", c, m)
	}
	c, m := do("POST", "/army/train", "k1", `{"building_id":"A","unit_id":"u_brawler","count":3}`)
	if c != 200 || m["content_version"] != "cv" || m["server_time"] == nil || len(m["queues"].([]any)) != 1 {
		t.Fatalf("train: %d %v", c, m)
	}
	c, m = do("POST", "/army/train", "k2", `{"building_id":"A","unit_id":"u_brawler","count":10}`)
	d := m["error"].(map[string]any)["details"].(map[string]any)
	if c != http.StatusConflict || d["reason"] != "housing_full" || d["max_count"] != float64(7) {
		t.Fatalf("housing_full: %d %v", c, m)
	}
	c, m = do("POST", "/army/train", "k3", `{"building_id":"fac","unit_id":"v_humvee","count":1}`) // no cash
	if c != http.StatusUnprocessableEntity || m["error"].(map[string]any)["code"] != "insufficient_resources" {
		t.Fatalf("no cash: %d %v", c, m)
	}
	if c, m = do("GET", "/army", "", ""); c != 200 || m["pools"] == nil {
		t.Fatalf("get: %d %v", c, m)
	}
}
