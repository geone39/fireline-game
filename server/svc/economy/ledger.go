package economy

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Delta is a signed change of one resource.
type Delta struct {
	Resource string
	Amount   int64
}

var (
	ErrInsufficientFunds = errors.New("economy: insufficient funds")
	ErrPlayerNotFound    = errors.New("economy: player not found")
	ErrUnknownResource   = errors.New("economy: unknown resource")
	ErrInvalid           = errors.New("economy: invalid request")
)

// Result of an Apply.
type Result struct {
	Replayed bool             // true: idemKey was seen before, nothing changed
	Balances map[string]int64 // current balance of the touched resources
}

// Ledger is the only writer of wallets. Every change is an append-only ledger row.
type Ledger struct {
	pool *pgxpool.Pool
	cfg  *Config
}

// NewLedger creates a ledger; cfg validates resource ids and may be nil in tests.
func NewLedger(pool *pgxpool.Pool, cfg *Config) *Ledger { return &Ledger{pool: pool, cfg: cfg} }

// Apply atomically applies deltas to the player's wallets under idemKey. A repeated
// (player, idemKey) is a no-op returning Replayed=true. Spending below zero fails with
// ErrInsufficientFunds and changes nothing.
func (l *Ledger) Apply(ctx context.Context, playerID uuid.UUID, deltas []Delta, reason, idemKey string) (Result, error) {
	tx, err := l.pool.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	res, err := l.ApplyTx(ctx, tx, playerID, deltas, reason, idemKey)
	if err != nil {
		return Result{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

// ApplyTx is Apply inside the caller's transaction, so a game command can combine the
// wallet change with its own writes atomically. The caller owns commit/rollback.
func (l *Ledger) ApplyTx(ctx context.Context, tx pgx.Tx, playerID uuid.UUID, deltas []Delta, reason, idemKey string) (Result, error) {
	merged, err := l.normalize(deltas, reason, idemKey)
	if err != nil {
		return Result{}, err
	}
	// Savepoint so a failure here leaves the caller's transaction usable.
	sp, err := tx.Begin(ctx)
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = sp.Rollback(ctx) }()

	res := Result{Balances: map[string]int64{}}
	for i, d := range merged {
		key := fmt.Sprintf("%s:%s#%d", playerID, idemKey, i)
		tag, err := sp.Exec(ctx, `
			INSERT INTO ledger (player_id, resource, delta, reason, idem_key)
			VALUES ($1, $2, $3, $4, $5) ON CONFLICT (idem_key) DO NOTHING`,
			playerID, d.Resource, d.Amount, reason, key)
		if err != nil {
			return Result{}, mapErr(err)
		}
		if tag.RowsAffected() == 0 {
			if i != 0 {
				return Result{}, fmt.Errorf("%w: idemKey reused with a different request", ErrInvalid)
			}
			return l.replay(ctx, sp, playerID, merged)
		}
		bal, err := l.credit(ctx, sp, playerID, d)
		if err != nil {
			return Result{}, err
		}
		res.Balances[d.Resource] = bal
	}
	if err := sp.Commit(ctx); err != nil {
		return Result{}, err
	}
	return res, nil
}

func (l *Ledger) normalize(deltas []Delta, reason, idemKey string) ([]Delta, error) {
	if idemKey == "" || reason == "" {
		return nil, fmt.Errorf("%w: idemKey and reason are required", ErrInvalid)
	}
	sum := map[string]int64{}
	for _, d := range deltas {
		if d.Resource == "" {
			return nil, fmt.Errorf("%w: empty resource", ErrInvalid)
		}
		if l.cfg != nil && !l.cfg.HasResource(d.Resource) {
			return nil, fmt.Errorf("%w: %q", ErrUnknownResource, d.Resource)
		}
		sum[d.Resource] += d.Amount
	}
	out := make([]Delta, 0, len(sum))
	for r, a := range sum {
		if a != 0 {
			out = append(out, Delta{r, a})
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: no non-zero deltas", ErrInvalid)
	}
	// Stable order: deterministic row keys and a consistent lock order (no deadlocks).
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out, nil
}

// credit changes the wallet and returns the new balance. UPDATE first: an INSERT with a
// negative amount would trip the CHECK even when a row exists.
func (l *Ledger) credit(ctx context.Context, tx pgx.Tx, p uuid.UUID, d Delta) (int64, error) {
	var bal int64
	err := tx.QueryRow(ctx, `UPDATE wallets SET amount = amount + $3 WHERE player_id = $1 AND resource = $2 RETURNING amount`,
		p, d.Resource, d.Amount).Scan(&bal)
	if err == nil {
		return bal, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return 0, mapErr(err)
	}
	if d.Amount < 0 {
		return 0, ErrInsufficientFunds
	}
	err = tx.QueryRow(ctx, `
		INSERT INTO wallets (player_id, resource, amount) VALUES ($1, $2, $3)
		ON CONFLICT (player_id, resource) DO UPDATE SET amount = wallets.amount + EXCLUDED.amount
		RETURNING amount`, p, d.Resource, d.Amount).Scan(&bal)
	return bal, mapErr(err)
}

func (l *Ledger) replay(ctx context.Context, tx pgx.Tx, p uuid.UUID, ds []Delta) (Result, error) {
	res := Result{Replayed: true, Balances: map[string]int64{}}
	for _, d := range ds {
		var bal int64
		err := tx.QueryRow(ctx, `SELECT amount FROM wallets WHERE player_id = $1 AND resource = $2`, p, d.Resource).Scan(&bal)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Result{}, err
		}
		res.Balances[d.Resource] = bal
	}
	return res, nil
}

// Balances returns the player's wallets (read-only helper).
func (l *Ledger) Balances(ctx context.Context, playerID uuid.UUID) (map[string]int64, error) {
	rows, err := l.pool.Query(ctx, `SELECT resource, amount FROM wallets WHERE player_id = $1`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]int64{}
	for rows.Next() {
		var r string
		var a int64
		if err := rows.Scan(&r, &a); err != nil {
			return nil, err
		}
		out[r] = a
	}
	return out, rows.Err()
}

func mapErr(err error) error {
	var pe *pgconn.PgError
	if errors.As(err, &pe) {
		switch pe.Code {
		case "23514":
			return ErrInsufficientFunds
		case "23503":
			return ErrPlayerNotFound
		}
	}
	return err
}
