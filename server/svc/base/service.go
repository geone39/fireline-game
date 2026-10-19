package base

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/geone39/fireline/server/svc/economy"
)

// ErrNoBase is returned for a player that has no base_state row yet.
var ErrNoBase = errors.New("base: player has no base")

// MaxBatch is the largest accepted POST /v1/base/commands batch (a transport limit,
// not a game number).
const MaxBatch = 100

// CommandError ties a rule violation to the command that caused it.
type CommandError struct {
	Index int
	Err   error
}

func (e *CommandError) Error() string { return fmt.Sprintf("command %d: %v", e.Index, e.Err) }
func (e *CommandError) Unwrap() error { return e.Err }

// DB is the slice of *pgxpool.Pool the service needs.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Ledger is the slice of *economy.Ledger the service needs: the only wallet writer.
type Ledger interface {
	ApplyTx(ctx context.Context, tx pgx.Tx, playerID uuid.UUID, deltas []economy.Delta, reason, idemKey string) (economy.Result, error)
}

// SnapshotSink stores an immutable defence snapshot in the caller's transaction.
// There is no defence_snapshots table yet (docs/open-questions.md, OQ-B7), so the sink
// is optional; with nil the layout is saved and base_version is bumped without a snapshot.
type SnapshotSink interface {
	SaveSnapshot(ctx context.Context, tx pgx.Tx, player uuid.UUID, baseVersion, hqLevel int, l Layout) error
}

// Config wires a Service.
type Config struct {
	DB        DB
	Ledger    Ledger
	Econ      *economy.Config
	Engine    Engine
	Snapshots SnapshotSink
	Now       func() time.Time
}

// Service is the base application layer: lazy timers, commands, layout save.
type Service struct {
	cfg Config
}

// NewService validates and creates a Service.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil || cfg.Ledger == nil || cfg.Econ == nil || cfg.Engine.Prices == nil || cfg.Engine.Rules.Cat == nil {
		return nil, errors.New("base: Config needs DB, Ledger, Econ, Engine.Rules and Engine.Prices")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	if cfg.Engine.NewID == nil {
		cfg.Engine.NewID = uuid.NewString
	}
	return &Service{cfg: cfg}, nil
}

// Builders reports builder capacity.
type Builders struct {
	Total int `json:"total"`
	Busy  int `json:"busy"`
}

// CommandResult is the outcome of one command of a batch.
type CommandResult struct {
	Op       Op     `json:"op"`
	UID      string `json:"uid,omitempty"`
	Replayed bool   `json:"replayed"`
}

// State is the BaseState returned by GET /v1/base and POST /v1/base/commands.
type State struct {
	Version    int              `json:"version"`
	Layout     Layout           `json:"layout"`
	Timers     []Timer          `json:"timers"`
	Builders   Builders         `json:"builders"`
	Collectors map[string]int64 `json:"collectors"` // accrued, uncollected output per resource
	Replayed   bool             `json:"replayed,omitempty"`
	Results    []CommandResult  `json:"results,omitempty"`
}

type txState struct {
	version int
	world   World
	loaded  Layout // layout as stored, to detect changes
	dirty   bool   // layout or version must be written
	events  []event
	done    []Timer
}

type event struct {
	op   string
	args map[string]any
}

// InitBase creates the player's base_state if it does not exist (no-op otherwise).
func (s *Service) InitBase(ctx context.Context, player uuid.UUID, l Layout) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	tx, err := s.cfg.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `INSERT INTO base_state (player_id, layout, updated_at) VALUES ($1, $2::jsonb, $3)
		ON CONFLICT (player_id) DO NOTHING`, player, string(b), s.cfg.Now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Get returns the base with all due timers settled (and persisted).
func (s *Service) Get(ctx context.Context, player uuid.UUID) (State, error) {
	var out State
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		st, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if err := s.save(ctx, tx, player, st, now); err != nil {
			return err
		}
		out, err = s.view(ctx, tx, player, st, now)
		return err
	})
	return out, err
}

// Apply runs a batch of commands atomically under batchKey (the request's
// Idempotency-Key). Any failing command rolls the whole batch back, wallets included. A
// batchKey that was already applied returns the current state with Replayed=true.
func (s *Service) Apply(ctx context.Context, player uuid.UUID, batchKey string, cmds []Command) (State, error) {
	if batchKey == "" {
		return State{}, fail(CodeBadCommand, "", "batch needs an idempotency key")
	}
	if len(cmds) > MaxBatch {
		return State{}, fail(CodeBadCommand, "", "at most %d commands per batch", MaxBatch)
	}
	var out State
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		st, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		var seen bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM base_events WHERE player_id = $1 AND args->>'batch' = $2)`,
			player, batchKey).Scan(&seen); err != nil {
			return err
		}
		var results []CommandResult
		if !seen {
			for i, c := range cmds {
				w, eff, res, err := s.cfg.Engine.Plan(st.world, c, now)
				if err != nil {
					return &CommandError{Index: i, Err: err}
				}
				if err := s.effect(ctx, tx, player, st, eff, fmt.Sprintf("base:%s:%d", batchKey, i), now); err != nil {
					return &CommandError{Index: i, Err: err}
				}
				if !res.Replayed {
					st.dirty = true
				}
				st.world = w
				results = append(results, CommandResult{Op: c.Op, UID: c.UID, Replayed: res.Replayed})
				st.events = append(st.events, event{op: string(c.Op), args: map[string]any{"batch": batchKey, "i": i, "cmd": c, "replayed": res.Replayed}})
			}
		}
		if err := s.save(ctx, tx, player, st, now); err != nil {
			return err
		}
		out, err = s.view(ctx, tx, player, st, now)
		out.Replayed, out.Results = seen, results
		return err
	})
	return out, err
}

// SaveResult is the outcome of SaveLayout.
type SaveResult struct {
	State
	Unchanged bool `json:"unchanged"`
}

// SaveLayout stores a rearranged layout. The server is authoritative: the new layout
// must contain exactly the player's existing items (same uid, type and level) and may
// only change positions and rotations; obstacles stay where they are. It is checked with
// ValidateLayout and, when it differs from the stored one, bumps the version and writes
// a defence snapshot. Saving an identical layout is a no-op.
func (s *Service) SaveLayout(ctx context.Context, player uuid.UUID, nl Layout) (SaveResult, error) {
	var out SaveResult
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		st, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if err := s.sameItems(st.world.Layout, nl); err != nil {
			return err
		}
		if err := s.cfg.Engine.Rules.ValidateLayout(nl); err != nil {
			return err
		}
		changed := !layoutEqual(st.world.Layout, nl)
		if changed {
			st.world.Layout = nl.Clone()
			st.dirty = true
			st.events = append(st.events, event{op: "save_layout", args: map[string]any{"items": len(nl.Items)}})
		}
		if err := s.save(ctx, tx, player, st, now); err != nil {
			return err
		}
		if changed && s.cfg.Snapshots != nil {
			hq := st.world.Layout.HQLevel(s.cfg.Engine.Rules.Cat)
			if err := s.cfg.Snapshots.SaveSnapshot(ctx, tx, player, st.version, hq, st.world.Layout.SortedByUID()); err != nil {
				return err
			}
		}
		v, err := s.view(ctx, tx, player, st, now)
		out = SaveResult{State: v, Unchanged: !changed}
		return err
	})
	return out, err
}

func (s *Service) sameItems(cur, nl Layout) error {
	if len(cur.Items) != len(nl.Items) {
		return fail(CodeLayoutChanged, "", "layout has %d items, the base has %d", len(nl.Items), len(cur.Items))
	}
	for _, it := range nl.Items {
		i := cur.Find(it.UID)
		if i < 0 || cur.Items[i].Type != it.Type || cur.Items[i].Level != it.Level {
			return fail(CodeLayoutChanged, it.UID, "item differs from the server state (only position and rotation may change)")
		}
		if d, _ := s.cfg.Engine.Rules.Cat.Def(it.Type); d.Kind == KindObstacle {
			if o := cur.Items[i]; o.X != it.X || o.Y != it.Y || o.Rot != it.Rot {
				return fail(CodeNotPlaceable, it.UID, "obstacles cannot be moved")
			}
		}
	}
	return nil
}

func layoutEqual(a, b Layout) bool {
	x, _ := json.Marshal(a.SortedByUID())
	y, _ := json.Marshal(b.SortedByUID())
	return string(x) == string(y)
}

func (s *Service) inTx(ctx context.Context, f func(tx pgx.Tx, now time.Time) error) error {
	tx, err := s.cfg.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := f(tx, s.cfg.Now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// load locks the player's base row, reads layout and timers and settles them at now.
func (s *Service) load(ctx context.Context, tx pgx.Tx, player uuid.UUID, now time.Time) (*txState, error) {
	var raw []byte
	st := &txState{}
	err := tx.QueryRow(ctx, `SELECT version, layout FROM base_state WHERE player_id = $1 FOR UPDATE`, player).Scan(&st.version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoBase
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(raw, &st.loaded); err != nil {
		return nil, fmt.Errorf("base: stored layout: %w", err)
	}
	rows, err := tx.Query(ctx, `SELECT id, kind, target_id, finish_at, payload FROM timers
		WHERE player_id = $1 AND kind IN ('build', 'upgrade') ORDER BY finish_at, id`, player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var timers []Timer
	for rows.Next() {
		var t Timer
		var id uuid.UUID
		var kind string
		var payload []byte
		if err := rows.Scan(&id, &kind, &t.Target, &t.FinishAt, &payload); err != nil {
			return nil, err
		}
		var p struct {
			ToLevel int              `json:"to_level"`
			Paid    map[string]int64 `json:"paid"`
		}
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, fmt.Errorf("base: timer %s payload: %w", id, err)
		}
		t.ID, t.Kind, t.ToLevel, t.Paid = id.String(), TimerKind(kind), p.ToLevel, p.Paid
		timers = append(timers, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	l, pending, done := Settle(st.loaded, timers, now)
	st.world = World{Layout: l, Timers: pending}
	st.done = done
	if len(done) > 0 {
		st.dirty = true
		ids := make([]string, len(done))
		for i, t := range done {
			ids[i] = t.ID
		}
		st.events = append(st.events, event{op: "settle", args: map[string]any{"timers": ids}})
	}
	return st, nil
}

// save writes the layout (version+1 when it changed), the timer diff and the events.
func (s *Service) save(ctx context.Context, tx pgx.Tx, player uuid.UUID, st *txState, now time.Time) error {
	keep := make([]uuid.UUID, 0, len(st.world.Timers))
	for _, t := range st.world.Timers {
		id, err := uuid.Parse(t.ID)
		if err != nil {
			return fmt.Errorf("base: timer id %q: %w", t.ID, err)
		}
		keep = append(keep, id)
		p, _ := json.Marshal(map[string]any{"to_level": t.ToLevel, "paid": t.Paid})
		if _, err := tx.Exec(ctx, `INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload)
			VALUES ($1, $2, $3, $4, $5, $6::jsonb)
			ON CONFLICT (id) DO UPDATE SET finish_at = EXCLUDED.finish_at`,
			id, player, string(t.Kind), t.Target, t.FinishAt, string(p)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM timers WHERE player_id = $1 AND kind IN ('build', 'upgrade') AND id <> ALL($2)`, player, keep); err != nil {
		return err
	}
	if st.dirty && !layoutEqual(st.loaded, st.world.Layout) {
		b, err := json.Marshal(st.world.Layout)
		if err != nil {
			return err
		}
		st.version++
		if _, err := tx.Exec(ctx, `UPDATE base_state SET layout = $2::jsonb, version = $3, updated_at = $4 WHERE player_id = $1`,
			player, string(b), st.version, now); err != nil {
			return err
		}
		st.loaded = st.world.Layout
	}
	for _, e := range st.events {
		b, _ := json.Marshal(e.args)
		if _, err := tx.Exec(ctx, `INSERT INTO base_events (player_id, op, args, applied_at) VALUES ($1, $2, $3::jsonb, $4)`,
			player, e.op, string(b), now); err != nil {
			return err
		}
	}
	st.events = nil
	return nil
}

// effect performs a command's wallet side: ledger deltas or a collector pickup.
func (s *Service) effect(ctx context.Context, tx pgx.Tx, player uuid.UUID, st *txState, eff Effect, idemKey string, now time.Time) error {
	if eff.Collect {
		return s.collect(ctx, tx, player, st, idemKey, now)
	}
	if len(eff.Deltas) == 0 {
		return nil
	}
	return s.ledger(ctx, tx, player, eff.Deltas, eff.Reason, idemKey)
}

func (s *Service) ledger(ctx context.Context, tx pgx.Tx, player uuid.UUID, ds []economy.Delta, reason, idemKey string) error {
	_, err := s.cfg.Ledger.ApplyTx(ctx, tx, player, ds, reason, idemKey)
	if errors.Is(err, economy.ErrInsufficientFunds) {
		return fail(CodeInsufficient, "", "not enough resources")
	}
	return err
}

// resources lists the resources collectors produce, in a stable order.
func (s *Service) resources() []string {
	var rs []string
	for r, pct := range s.cfg.Econ.Passive.ShareByResource {
		if pct > 0 {
			rs = append(rs, r)
		}
	}
	sort.Strings(rs)
	return rs
}

func (s *Service) pool(ctx context.Context, tx pgx.Tx, player uuid.UUID, res string, now time.Time, lock bool) (economy.Pool, error) {
	q := `SELECT amount, updated_at FROM collector_pools WHERE player_id = $1 AND resource = $2`
	if lock {
		q += ` FOR UPDATE`
	}
	p := economy.Pool{At: now}
	err := tx.QueryRow(ctx, q, player, res).Scan(&p.Amount, &p.At)
	if errors.Is(err, pgx.ErrNoRows) {
		// ASSUMPTION: a pool that was never touched starts accruing at the first visit.
		return economy.Pool{At: now}, nil
	}
	return p, err
}

// collect moves collector pools into wallets. The clock never rewinds a pool: if now is
// before the pool's last update, the pool's own time is used.
func (s *Service) collect(ctx context.Context, tx pgx.Tx, player uuid.UUID, st *txState, idemKey string, now time.Time) error {
	hq := st.world.Layout.HQLevel(s.cfg.Engine.Rules.Cat)
	var credits []economy.Delta
	for _, res := range s.resources() {
		p, err := s.pool(ctx, tx, player, res, now, true)
		if err != nil {
			return err
		}
		at := now
		if at.Before(p.At) {
			at = p.At
		}
		var wallet int64
		err = tx.QueryRow(ctx, `SELECT amount FROM wallets WHERE player_id = $1 AND resource = $2`, player, res).Scan(&wallet)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return err
		}
		take, np := s.cfg.Econ.Collect(hq, res, p, wallet, at)
		if _, err := tx.Exec(ctx, `INSERT INTO collector_pools (player_id, resource, amount, updated_at) VALUES ($1, $2, $3, $4)
			ON CONFLICT (player_id, resource) DO UPDATE SET amount = EXCLUDED.amount, updated_at = EXCLUDED.updated_at`,
			player, res, np.Amount, np.At); err != nil {
			return err
		}
		if take > 0 {
			credits = append(credits, economy.Delta{Resource: res, Amount: take})
		}
	}
	if len(credits) == 0 {
		return nil
	}
	return s.ledger(ctx, tx, player, credits, "collect", idemKey)
}

// view builds the response: settled layout, running timers, builders, accrued pools.
func (s *Service) view(ctx context.Context, tx pgx.Tx, player uuid.UUID, st *txState, now time.Time) (State, error) {
	hq := st.world.Layout.HQLevel(s.cfg.Engine.Rules.Cat)
	out := State{
		Version: st.version, Layout: st.world.Layout, Timers: st.world.Timers,
		Builders:   Builders{Total: s.cfg.Engine.Prices.Builders(hq), Busy: len(st.world.Timers)},
		Collectors: map[string]int64{},
	}
	if out.Layout.Items == nil {
		out.Layout.Items = []Item{}
	}
	if out.Timers == nil {
		out.Timers = []Timer{}
	}
	for _, res := range s.resources() {
		p, err := s.pool(ctx, tx, player, res, now, false)
		if err != nil {
			return State{}, err
		}
		out.Collectors[res] = s.cfg.Econ.Accrue(hq, res, p, now).Amount
	}
	return out, nil
}
