package base

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
	"github.com/geone39/fireline/server/svc/economy"
)

// The database is started lazily so the pure tests in this package never need Docker.
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

type clock struct{ t time.Time }

func (c *clock) Now() time.Time { return c.t }

type env struct {
	t      *testing.T
	svc    *Service
	ledger *economy.Ledger
	player uuid.UUID
	clk    *clock
	snaps  *fakeSnaps
}

type fakeSnaps struct{ got []int }

func (f *fakeSnaps) SaveSnapshot(_ context.Context, _ pgx.Tx, _ uuid.UUID, v, _ int, _ Layout) error {
	f.got = append(f.got, v)
	return nil
}

func newEnv(t *testing.T, items ...Item) *env {
	t.Helper()
	st := testStore(t)
	econ, err := economy.Load("../../../content/economy.yaml")
	if err != nil {
		t.Fatal(err)
	}
	e := &env{t: t, clk: &clock{t0}, snaps: &fakeSnaps{}}
	e.ledger = economy.NewLedger(st.Pool, econ)
	p, err := st.UpsertPlayer(context.Background(), "test", uuid.NewString(), "p")
	if err != nil {
		t.Fatal(err)
	}
	e.player = p.ID
	eng := testEngine(t)
	eng.NewID = uuid.NewString
	e.svc, err = NewService(Config{DB: st.Pool, Ledger: e.ledger, Econ: econ, Engine: eng, Snapshots: e.snaps, Now: e.clk.Now})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) == 0 {
		items = []Item{hqItem(1, 20, 20)}
	}
	if err := e.svc.InitBase(context.Background(), e.player, Layout{Items: items}); err != nil {
		t.Fatal(err)
	}
	return e
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

func (e *env) count(q string) int {
	e.t.Helper()
	var n int
	if err := testStore(e.t).Pool.QueryRow(context.Background(), q, e.player).Scan(&n); err != nil {
		e.t.Fatal(err)
	}
	return n
}

func (e *env) apply(key string, cmds ...Command) (State, error) {
	return e.svc.Apply(context.Background(), e.player, key, cmds)
}

func (e *env) get() State {
	e.t.Helper()
	s, err := e.svc.Get(context.Background(), e.player)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}

func level(s State, uid string) int {
	if i := s.Layout.Find(uid); i >= 0 {
		return s.Layout.Items[i].Level
	}
	return -1
}

func wantErr(t *testing.T, err error, code Code, index int) {
	t.Helper()
	wantCode(t, err, code)
	var ce *CommandError
	if index >= 0 && (!errors.As(err, &ce) || ce.Index != index) {
		t.Fatalf("want command index %d, got %v", index, err)
	}
}

func TestLazyTimersThroughDB(t *testing.T) {
	e := newEnv(t, hqItem(1, 20, 20), Item{UID: "m", Type: "b_cash_mine", Level: 1, X: 2, Y: 2})
	e.grant("cash", 1000)
	st, err := e.apply("k1", Command{Op: OpUpgrade, UID: "m", To: 2}) // 200 cash, 120 s
	if err != nil {
		t.Fatal(err)
	}
	if e.bal("cash") != 800 || len(st.Timers) != 1 || level(st, "m") != 1 || st.Builders.Busy != 1 {
		t.Fatalf("after upgrade: bal %d, %+v", e.bal("cash"), st)
	}
	e.clk.t = t0.Add(119 * time.Second)
	if s := e.get(); level(s, "m") != 1 || len(s.Timers) != 1 {
		t.Fatalf("1s early: %+v", s)
	}
	e.clk.t = t0.Add(121 * time.Second)
	s := e.get()
	if level(s, "m") != 2 || len(s.Timers) != 0 {
		t.Fatalf("after finish: %+v", s)
	}
	v := s.Version
	// settled twice: nothing changes, no new version
	if s2 := e.get(); s2.Version != v || level(s2, "m") != 2 {
		t.Fatalf("second settle: %+v", s2)
	}
	// the clock jumps back: the finished upgrade stays finished
	e.clk.t = t0
	if s3 := e.get(); level(s3, "m") != 2 || s3.Version != v {
		t.Fatalf("clock back: %+v", s3)
	}
	if n := e.count(`SELECT count(*) FROM timers WHERE player_id = $1`); n != 0 {
		t.Fatalf("%d timers left in db", n)
	}
	if n := e.count(`SELECT count(*) FROM base_events WHERE player_id = $1 AND op = 'settle'`); n != 1 {
		t.Fatalf("settle events: %d", n)
	}
}

func TestClockBackwardsBeforeFinish(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	e.clk.t = t0.Add(time.Hour)
	if _, err := e.apply("k1", build("m", "b_cash_mine", 2, 2)); err != nil { // finishes t0+1h+60s
		t.Fatal(err)
	}
	e.clk.t = t0 // an hour earlier than the order
	if s := e.get(); len(s.Timers) != 1 {
		t.Fatalf("timer must wait: %+v", s)
	}
	e.clk.t = t0.Add(2 * time.Hour)
	if s := e.get(); len(s.Timers) != 0 {
		t.Fatalf("timer must fire: %+v", s)
	}
}

func TestBatchIsIdempotent(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	cmds := []Command{build("m", "b_cash_mine", 2, 2), build("v", "b_vault", 8, 2)}
	s1, err := e.apply("same-key", cmds...)
	if err != nil || s1.Replayed {
		t.Fatalf("first: %+v %v", s1, err)
	}
	spent := e.bal("cash")
	if spent != 1000-100-50 {
		t.Fatalf("balance %d", spent)
	}
	s2, err := e.apply("same-key", cmds...)
	if err != nil || !s2.Replayed || e.bal("cash") != spent || len(s2.Timers) != 2 || s2.Version != s1.Version {
		t.Fatalf("replay: %+v %v bal %d", s2, err, e.bal("cash"))
	}
	if n := e.count(`SELECT count(*) FROM timers WHERE player_id = $1`); n != 2 {
		t.Fatalf("timers: %d", n)
	}
	// a new key with the same build is a per-command replay: no charge
	s3, err := e.apply("other-key", cmds[0])
	if err != nil || s3.Replayed || !s3.Results[0].Replayed || e.bal("cash") != spent {
		t.Fatalf("same uid, new key: %+v %v", s3, err)
	}
}

func TestInsufficientResourcesRollsBackEverything(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 120) // the mine (100) fits, the vault (50) does not
	before := e.get()
	_, err := e.apply("k", build("m", "b_cash_mine", 2, 2), build("v", "b_vault", 8, 2))
	wantErr(t, err, CodeInsufficient, 1)
	if APICode(CodeInsufficient) != "insufficient_resources" {
		t.Fatal("api code")
	}
	after := e.get()
	if e.bal("cash") != 120 || len(after.Timers) != 0 || len(after.Layout.Items) != 1 || after.Version != before.Version {
		t.Fatalf("partial changes: bal %d, %+v", e.bal("cash"), after)
	}
	if n := e.count(`SELECT count(*) FROM base_events WHERE player_id = $1 AND args ? 'batch'`); n != 0 {
		t.Fatalf("%d command events written by a failed batch", n)
	}
	// the key is not burned: a smaller batch under the same key works
	if _, err := e.apply("k", build("m", "b_cash_mine", 2, 2)); err != nil || e.bal("cash") != 20 {
		t.Fatalf("retry: %v bal %d", err, e.bal("cash"))
	}
}

func TestRuleErrorRollsBackWallet(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	_, err := e.apply("k", build("m", "b_cash_mine", 2, 2), build("bad", "b_cash_mine", 2, 2)) // overlap
	wantErr(t, err, CodeOverlap, 1)
	if e.bal("cash") != 1000 || len(e.get().Timers) != 0 {
		t.Fatal("first command must be rolled back")
	}
}

func TestEventsAndLedgerRows(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	if _, err := e.apply("k", build("m", "b_cash_mine", 2, 2), Command{Op: OpMove, UID: "m", X: 8, Y: 8}); err != nil {
		t.Fatal(err)
	}
	if n := e.count(`SELECT count(*) FROM base_events WHERE player_id = $1 AND args->>'batch' = 'k'`); n != 2 {
		t.Fatalf("events: %d", n)
	}
	if n := e.count(`SELECT count(*) FROM ledger WHERE player_id = $1 AND reason = 'build' AND resource = 'cash' AND delta = -100`); n != 1 {
		t.Fatalf("build ledger rows: %d", n)
	}
}

func TestBuildersLimit(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	if _, err := e.apply("k1", build("m1", "b_cash_mine", 2, 2), build("m2", "b_cash_mine", 6, 2)); err != nil {
		t.Fatal(err)
	}
	_, err := e.apply("k2", build("v", "b_vault", 10, 2))
	wantErr(t, err, CodeNoBuilder, 0)
	e.clk.t = t0.Add(time.Hour)
	if _, err := e.apply("k3", build("v", "b_vault", 10, 2)); err != nil {
		t.Fatalf("builder must be free after the timers fired: %v", err)
	}
}

func TestSpeedupThroughDB(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 20000)
	e.grant("gems", 10)
	if _, err := e.apply("k1", Command{Op: OpUpgrade, UID: "hq"}); err != nil { // 7200 s
		t.Fatal(err)
	}
	e.clk.t = t0.Add(time.Hour)
	s, err := e.apply("k2", Command{Op: OpSpeedup, UID: "hq"})
	if err != nil || level(s, "hq") != 2 || len(s.Timers) != 0 || e.bal("gems") != 6 {
		t.Fatalf("speedup: %+v %v gems %d", s, err, e.bal("gems"))
	}
	if _, err := e.apply("k2", Command{Op: OpSpeedup, UID: "hq"}); err != nil || e.bal("gems") != 6 {
		t.Fatalf("replay charged: %v gems %d", err, e.bal("gems"))
	}
	// not enough gems -> insufficient, timer untouched
	if _, err := e.apply("k3", Command{Op: OpUpgrade, UID: "hq"}); err != nil {
		t.Fatal(err)
	}
	_, err = e.apply("k4", Command{Op: OpSpeedup, UID: "hq"}) // 4h+ left -> more than 6 gems
	wantErr(t, err, CodeInsufficient, 0)
	if len(e.get().Timers) != 1 {
		t.Fatal("timer must survive a failed speedup")
	}
}

func TestCancelRefundThroughDB(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	if _, err := e.apply("k1", build("m", "b_cash_mine", 2, 2)); err != nil {
		t.Fatal(err)
	}
	s, err := e.apply("k2", Command{Op: OpRemove, UID: "m"})
	if err != nil || e.bal("cash") != 950 || len(s.Timers) != 0 || s.Layout.Find("m") >= 0 {
		t.Fatalf("cancel: %+v %v bal %d", s, err, e.bal("cash"))
	}
	if n := e.count(`SELECT count(*) FROM timers WHERE player_id = $1`); n != 0 {
		t.Fatalf("timers: %d", n)
	}
}

func TestClearObstacleThroughDB(t *testing.T) {
	e := newEnv(t, hqItem(1, 20, 20), Item{UID: "r", Type: "o_rock", Level: 1, X: 2, Y: 2})
	_, err := e.apply("k1", Command{Op: OpClearObstacle, UID: "r"})
	wantErr(t, err, CodeInsufficient, 0)
	if e.get().Layout.Find("r") < 0 {
		t.Fatal("obstacle must stay when the player cannot pay")
	}
	e.grant("cash", 1000)
	s, err := e.apply("k2", Command{Op: OpClearObstacle, UID: "r"})
	rock, _ := e.svc.cfg.Engine.Rules.Cat.Def("o_rock")
	if err != nil || s.Layout.Find("r") >= 0 || e.bal("cash") != 1000-rock.ClearCost["cash"] || e.bal("gems") != rock.Reward["gems"] {
		t.Fatalf("clear: %+v %v cash %d gems %d", s, err, e.bal("cash"), e.bal("gems"))
	}
}

func TestCollectThroughDB(t *testing.T) {
	e := newEnv(t)
	// first visit starts the pool at "now"
	if s := e.get(); s.Collectors["cash"] != 0 {
		t.Fatalf("fresh pool: %+v", s.Collectors)
	}
	if _, err := e.apply("c0", Command{Op: OpCollect}); err != nil { // creates the pool rows
		t.Fatal(err)
	}
	e.clk.t = t0.Add(time.Hour)
	s := e.get()
	econ := e.svc.cfg.Econ
	want := econ.Accrue(1, "cash", economy.Pool{At: t0}, e.clk.t).Amount
	if want == 0 || s.Collectors["cash"] != want {
		t.Fatalf("GET shows %d, want %d", s.Collectors["cash"], want)
	}
	if e.bal("cash") != 0 {
		t.Fatal("GET must not pay out")
	}
	s, err := e.apply("c1", Command{Op: OpCollect})
	if err != nil || e.bal("cash") != want || s.Collectors["cash"] != 0 {
		t.Fatalf("collect: %v bal %d want %d, pool %v", err, e.bal("cash"), want, s.Collectors)
	}
	// same key again: nothing more
	if _, err := e.apply("c1", Command{Op: OpCollect}); err != nil || e.bal("cash") != want {
		t.Fatalf("replay paid twice: %v %d", err, e.bal("cash"))
	}
	// the clock jumps back and collect is repeated under a new key: no double pay
	e.clk.t = t0
	if _, err := e.apply("c2", Command{Op: OpCollect}); err != nil || e.bal("cash") != want {
		t.Fatalf("collect after clock rewind: %v %d", err, e.bal("cash"))
	}
	// storage headroom limits the pickup and the rest stays in the pool
	e.clk.t = t0.Add(30 * 24 * time.Hour)
	if _, err := e.apply("c3", Command{Op: OpCollect}); err != nil {
		t.Fatal(err)
	}
	if got, cap := e.bal("cash"), econ.StorageCap(1); got > cap {
		t.Fatalf("wallet %d above storage cap %d", got, cap)
	}
}

func TestSaveLayout(t *testing.T) {
	e := newEnv(t, hqItem(1, 20, 20), Item{UID: "m", Type: "b_cash_mine", Level: 1, X: 2, Y: 2}, Item{UID: "r", Type: "o_rock", Level: 1, X: 30, Y: 30})
	ctx := context.Background()
	cur := e.get()

	// identical layout: no-op
	res, err := e.svc.SaveLayout(ctx, e.player, cur.Layout)
	if err != nil || !res.Unchanged || res.Version != cur.Version || len(e.snaps.got) != 0 {
		t.Fatalf("unchanged: %+v %v", res, err)
	}
	// rearranged: new version and snapshot
	moved := cur.Layout.Clone()
	moved.Items[1].X, moved.Items[1].Y = 8, 8
	res, err = e.svc.SaveLayout(ctx, e.player, moved)
	if err != nil || res.Unchanged || res.Version != cur.Version+1 || len(e.snaps.got) != 1 || e.snaps.got[0] != res.Version {
		t.Fatalf("moved: %+v %v snaps %v", res, err, e.snaps.got)
	}
	if got := e.get().Layout.Items[1]; got.X != 8 {
		t.Fatalf("not stored: %+v", got)
	}

	bad := func(name string, mutate func(l *Layout), code Code) {
		t.Helper()
		l := e.get().Layout.Clone()
		mutate(&l)
		_, err := e.svc.SaveLayout(ctx, e.player, l)
		var be *Error
		if !errors.As(err, &be) || be.Code != code {
			t.Errorf("%s: want %s, got %v", name, code, err)
		}
	}
	bad("extra item", func(l *Layout) { l.Items = append(l.Items, Item{UID: "x", Type: "b_vault", Level: 1, X: 12, Y: 2}) }, CodeLayoutChanged)
	bad("missing item", func(l *Layout) { l.Items = l.Items[:1] }, CodeLayoutChanged)
	bad("level cheat", func(l *Layout) { l.Items[1].Level = 9 }, CodeLayoutChanged)
	bad("type swap", func(l *Layout) { l.Items[1].Type = "b_vault" }, CodeLayoutChanged)
	bad("move obstacle", func(l *Layout) { l.Items[2].X = 31 }, CodeNotPlaceable)
	bad("overlap", func(l *Layout) { l.Items[1].X, l.Items[1].Y = 20, 20 }, CodeOverlap)
	bad("out of bounds", func(l *Layout) { l.Items[1].X = 43 }, CodeOutOfBounds)
	if len(e.snaps.got) != 1 {
		t.Fatalf("rejected saves must not snapshot: %v", e.snaps.got)
	}
}

// ---- HTTP ----

func httpEnv(t *testing.T, items ...Item) (*env, http.Handler) {
	e := newEnv(t, items...)
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		Mount(r, Deps{
			Service:        e.svc,
			PlayerID:       func(context.Context) (uuid.UUID, bool) { return e.player, true },
			ContentVersion: func() string { return "cv-test" },
			Now:            e.clk.Now,
		})
	})
	return e, r
}

func do(h http.Handler, method, path, key, body string) (int, map[string]json.RawMessage) {
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	out := map[string]json.RawMessage{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func errCode(t *testing.T, m map[string]json.RawMessage) (code, reason string) {
	t.Helper()
	var e struct {
		Code    string `json:"code"`
		Details struct {
			Reason string `json:"reason"`
		} `json:"details"`
	}
	if err := json.Unmarshal(m["error"], &e); err != nil {
		t.Fatalf("no error body: %v", m)
	}
	return e.Code, e.Details.Reason
}

func TestHTTPFlow(t *testing.T) {
	e, h := httpEnv(t)
	e.grant("cash", 1000)

	code, body := do(h, "GET", "/v1/base", "", "")
	if code != 200 || body["server_time"] == nil || string(body["content_version"]) != `"cv-test"` || body["layout"] == nil {
		t.Fatalf("GET: %d %v", code, body)
	}
	// {op,args} array body, as client-core sends it
	code, body = do(h, "POST", "/v1/base/commands", "k1", `[{"op":"build","args":{"uid":"m","type":"b_cash_mine","x":2,"y":2,"rot":0}}]`)
	if code != 200 {
		t.Fatalf("POST: %d %s", code, body["error"])
	}
	var timers []Timer
	if err := json.Unmarshal(body["timers"], &timers); err != nil || len(timers) != 1 || timers[0].Target != "m" {
		t.Fatalf("timers: %s", body["timers"])
	}
	// wrapped form, flat fields, same key: replay
	code, body = do(h, "POST", "/v1/base/commands", "k1", `{"commands":[{"op":"build","uid":"m","type":"b_cash_mine","x":2,"y":2}]}`)
	if code != 200 || string(body["replayed"]) != "true" || e.bal("cash") != 900 {
		t.Fatalf("replay: %d %v bal %d", code, body, e.bal("cash"))
	}
}

func TestHTTPErrors(t *testing.T) {
	e, h := httpEnv(t)
	e.grant("cash", 50)
	cases := []struct {
		name, method, path, key, body string
		status                        int
		code, reason                  string
	}{
		{"no key", "POST", "/v1/base/commands", "", `[]`, 400, "invalid", ""},
		{"garbage", "POST", "/v1/base/commands", "a", `{"commands": 5}`, 400, "invalid", ""},
		{"insufficient", "POST", "/v1/base/commands", "b", `[{"op":"build","uid":"m","type":"b_cash_mine","x":2,"y":2}]`, 422, "insufficient_resources", "insufficient_resources"},
		{"overlap", "POST", "/v1/base/commands", "c", `[{"op":"build","uid":"m","type":"b_vault","x":20,"y":20}]`, 400, "invalid", "overlap"},
		{"unknown uid", "POST", "/v1/base/commands", "d", `[{"op":"upgrade","uid":"zzz"}]`, 404, "not_found", "not_found"},
		{"uid conflict", "POST", "/v1/base/commands", "e", `[{"op":"build","uid":"hq","type":"b_vault","x":2,"y":2}]`, 409, "conflict", "conflict"},
		{"layout body", "PUT", "/v1/base/layout", "f", `{}`, 400, "invalid", ""},
		{"layout cheat", "PUT", "/v1/base/layout", "g", `{"layout":{"items":[]}}`, 400, "invalid", "layout_mismatch"},
	}
	for _, c := range cases {
		status, body := do(h, c.method, c.path, c.key, c.body)
		code, reason := errCode(t, body)
		if status != c.status || code != c.code || reason != c.reason {
			t.Errorf("%s: got %d %s/%s, want %d %s/%s", c.name, status, code, reason, c.status, c.code, c.reason)
		}
	}
}

func TestHTTPNoBaseAndUnauthorized(t *testing.T) {
	st := testStore(t)
	e := newEnv(t)
	p, _ := st.UpsertPlayer(context.Background(), "test", uuid.NewString(), "nobase")
	auth := true
	r := chi.NewRouter()
	Mount(r, Deps{
		Service:  e.svc,
		PlayerID: func(context.Context) (uuid.UUID, bool) { return p.ID, auth },
	})
	if code, body := do(r, "GET", "/base", "", ""); code != 404 {
		t.Fatalf("no base: %d %v", code, body)
	}
	auth = false
	if code, _ := do(r, "GET", "/base", "", ""); code != 401 {
		t.Fatalf("unauth: %d", code)
	}
}

func TestHTTPSaveLayout(t *testing.T) {
	e, h := httpEnv(t, hqItem(1, 20, 20), Item{UID: "m", Type: "b_cash_mine", Level: 1, X: 2, Y: 2})
	code, body := do(h, "PUT", "/v1/base/layout", "p1", `{"layout":{"items":[{"uid":"hq","type":"b_hq","level":1,"x":20,"y":20,"rot":0},{"uid":"m","type":"b_cash_mine","level":1,"x":10,"y":10,"rot":0}]}}`)
	if code != 200 || string(body["unchanged"]) != "false" || string(body["version"]) != "2" {
		t.Fatalf("PUT: %d %v", code, body)
	}
	if got := e.get().Layout.Items[1]; got.X != 10 {
		t.Fatalf("not stored: %+v", got)
	}
}

func TestConcurrentSameKeyAppliesOnce(t *testing.T) {
	e := newEnv(t)
	e.grant("cash", 1000)
	var wg sync.WaitGroup
	errs := make([]error, 6)
	for i := range errs {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = e.apply("race", build("m", "b_cash_mine", 2, 2))
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if e.bal("cash") != 900 || len(e.get().Timers) != 1 {
		t.Fatalf("charged %d, timers %d", 1000-e.bal("cash"), len(e.get().Timers))
	}
}
