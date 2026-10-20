package db_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/testcontainers/testcontainers-go"
	tcpg "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"

	"github.com/geone39/fireline/server/db"
)

var testStore *db.Store

func TestMain(m *testing.M) {
	ctx := context.Background()
	ctr, err := tcpg.Run(ctx, "postgres:16-alpine",
		tcpg.WithDatabase("fireline"), tcpg.WithUsername("fireline"), tcpg.WithPassword("fireline"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(90*time.Second)))
	if err != nil {
		// CI must have Docker; locally allow skipping when it is unavailable.
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
	testStore, err = db.Connect(ctx, dsn)
	if err != nil {
		panic(err)
	}
	if err := testStore.Migrate(ctx); err != nil {
		panic(err)
	}
	code := m.Run()
	testStore.Close()
	_ = testcontainers.TerminateContainer(ctr)
	os.Exit(code)
}

func newPlayer(t *testing.T) uuid.UUID {
	t.Helper()
	p, err := testStore.UpsertPlayer(context.Background(), "guest", uuid.NewString(), "tester")
	if err != nil {
		t.Fatal(err)
	}
	return p.ID
}

// sqlState returns the SQLSTATE of err ("" if it is not a Postgres error).
func sqlState(err error) string {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		return pe.Code
	}
	return ""
}

func exec(t *testing.T, sql string, args ...any) error {
	t.Helper()
	_, err := testStore.Pool.Exec(context.Background(), sql, args...)
	return err
}

func wantState(t *testing.T, err error, code string) {
	t.Helper()
	if got := sqlState(err); got != code {
		t.Fatalf("want SQLSTATE %q, got %q (err=%v)", code, got, err)
	}
}

const (
	checkViolation = "23514"
	uniqueViolat   = "23505"
	fkViolation    = "23503"
)

func TestArmyConstraints(t *testing.T) {
	p := newPlayer(t)
	ins := `INSERT INTO army (player_id, unit_id, level, count) VALUES ($1, $2, $3, $4)`

	if err := exec(t, ins, p, "rifleman", 1, 10); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, ins, p, "rifleman", 2, 0); err != nil { // count 0 is allowed; other level = other row
		t.Fatal(err)
	}
	wantState(t, exec(t, ins, p, "rifleman", 1, 5), uniqueViolat)
	wantState(t, exec(t, ins, p, "grenadier", 0, 1), checkViolation)
	wantState(t, exec(t, ins, p, "grenadier", 1, -1), checkViolation)
	wantState(t, exec(t, ins, uuid.New(), "rifleman", 1, 1), fkViolation)
	wantState(t, exec(t, `UPDATE army SET count = -1 WHERE player_id = $1 AND unit_id = 'rifleman' AND level = 1`, p), checkViolation)

	// upsert pattern used by the future army service
	if err := exec(t, ins+` ON CONFLICT (player_id, unit_id, level) DO UPDATE SET count = army.count + EXCLUDED.count`, p, "rifleman", 1, 5); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := testStore.Pool.QueryRow(context.Background(), `SELECT count FROM army WHERE player_id=$1 AND unit_id='rifleman' AND level=1`, p).Scan(&n); err != nil || n != 15 {
		t.Fatalf("count=%d err=%v, want 15", n, err)
	}
}

func TestResearchConstraints(t *testing.T) {
	p := newPlayer(t)
	ins := `INSERT INTO research (player_id, node_id, state, finish_at) VALUES ($1, $2, $3, $4)`
	fin := time.Now().Add(time.Hour)

	if err := exec(t, ins, p, "dmg_1", "researching", fin); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, ins, p, "hp_1", "done", nil); err != nil {
		t.Fatal(err)
	}
	wantState(t, exec(t, ins, p, "dmg_1", "done", nil), uniqueViolat)
	wantState(t, exec(t, ins, p, "x", "queued", fin), checkViolation)
	wantState(t, exec(t, ins, p, "y", "researching", nil), checkViolation)
	wantState(t, exec(t, ins, uuid.New(), "dmg_1", "done", nil), fkViolation)
	if err := exec(t, `UPDATE research SET state='done', finish_at=NULL WHERE player_id=$1 AND node_id='dmg_1'`, p); err != nil {
		t.Fatal(err)
	}
}

func TestArmyResearchCascadeOnPlayerDelete(t *testing.T) {
	p := newPlayer(t)
	if err := exec(t, `INSERT INTO army (player_id, unit_id, level, count) VALUES ($1,'rifleman',1,1)`, p); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, `INSERT INTO research (player_id, node_id, state) VALUES ($1,'hp_1','done')`, p); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, `DELETE FROM players WHERE id=$1`, p); err != nil {
		t.Fatal(err)
	}
	for _, tbl := range []string{"army", "research"} {
		var n int
		if err := testStore.Pool.QueryRow(context.Background(), `SELECT count(*) FROM `+tbl+` WHERE player_id=$1`, p).Scan(&n); err != nil || n != 0 {
			t.Fatalf("%s rows after player delete: %d err=%v", tbl, n, err)
		}
	}
}

// Training and research are queued as timers with kind 'train' / 'research' (no schema change
// needed: kind is free text, payload is jsonb), and are looked up per player+kind.
func TestTimersTrainResearchKinds(t *testing.T) {
	p := newPlayer(t)
	ins := `INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload) VALUES ($1,$2,$3,$4,$5,$6::jsonb)`
	fin := time.Now().Add(time.Minute)
	if err := exec(t, ins, uuid.New(), p, "train", "rifleman", fin, `{"building_id":"b1","count":5,"level":1}`); err != nil {
		t.Fatal(err)
	}
	if err := exec(t, ins, uuid.New(), p, "research", "dmg_1", fin, `{}`); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := testStore.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM timers WHERE player_id=$1 AND kind='train' AND payload->>'building_id'='b1'`, p).Scan(&n); err != nil || n != 1 {
		t.Fatalf("train timers=%d err=%v", n, err)
	}

	var idx int
	if err := testStore.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_indexes WHERE tablename='timers' AND indexname IN ('timers_finish_at_idx','timers_player_kind_idx')`).Scan(&idx); err != nil || idx != 2 {
		t.Fatalf("timers indexes found=%d err=%v, want 2", idx, err)
	}
}
