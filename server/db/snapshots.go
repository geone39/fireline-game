package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// InsertSnapshot stores an immutable defence snapshot inside the caller's transaction.
// layoutJSON is the layout document as stored in base_state. A repeat of the same
// (player, baseVersion) is a no-op.
func (s *Store) InsertSnapshot(ctx context.Context, tx pgx.Tx, player uuid.UUID, baseVersion, hqLevel int, layoutJSON []byte) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO defence_snapshots (id, player_id, base_version, hq_level, layout)
		VALUES ($1, $2, $3, $4, $5::jsonb)
		ON CONFLICT (player_id, base_version) DO NOTHING`,
		uuid.New(), player, baseVersion, hqLevel, string(layoutJSON))
	return err
}
