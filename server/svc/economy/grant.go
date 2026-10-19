package economy

import (
	"context"
	"sort"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

const startGrantReason = "start_grant"

// StartGrantKey is the idempotency key of a player's one-off start grant.
func StartGrantKey(playerID uuid.UUID) string { return "start_grant:" + playerID.String() }

// StartGrant lists the non-zero deltas of content/economy.yaml start_grant in stable order.
func (c *Config) StartGrant() []Delta {
	out := make([]Delta, 0, len(c.StartGrantAmounts))
	for r, a := range c.StartGrantAmounts {
		if a > 0 {
			out = append(out, Delta{r, a})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Resource < out[j].Resource })
	return out
}

// GrantStart credits the configured start grant exactly once per player (idemKey
// start_grant:<playerID>); a repeat returns Replayed=true and changes nothing. With an
// empty grant it is a no-op. Needs a Ledger built with a Config.
func (l *Ledger) GrantStart(ctx context.Context, playerID uuid.UUID) (Result, error) {
	ds, ok := l.grant()
	if !ok {
		return Result{Balances: map[string]int64{}}, nil
	}
	return l.Apply(ctx, playerID, ds, startGrantReason, StartGrantKey(playerID))
}

// GrantStartTx is GrantStart inside the caller's transaction (e.g. guest creation).
func (l *Ledger) GrantStartTx(ctx context.Context, tx pgx.Tx, playerID uuid.UUID) (Result, error) {
	ds, ok := l.grant()
	if !ok {
		return Result{Balances: map[string]int64{}}, nil
	}
	return l.ApplyTx(ctx, tx, playerID, ds, startGrantReason, StartGrantKey(playerID))
}

func (l *Ledger) grant() ([]Delta, bool) {
	if l.cfg == nil {
		return nil, false
	}
	ds := l.cfg.StartGrant()
	return ds, len(ds) > 0
}
