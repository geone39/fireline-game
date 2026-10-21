package db

import (
	"context"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ClaimTutorialMarkTx sets the one-shot mark in players.tutorial. It reports true only for
// the call that set it, so a mark can be claimed once per player even under concurrency.
func (s *Store) ClaimTutorialMarkTx(ctx context.Context, tx pgx.Tx, player uuid.UUID, mark string) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE players SET tutorial = tutorial || jsonb_build_object($2::text, true)
		WHERE id = $1 AND NOT (tutorial ? $2::text)`, player, mark)
	return tag.RowsAffected() == 1, err
}

// ClaimTutorialMark is ClaimTutorialMarkTx on its own transaction.
func (s *Store) ClaimTutorialMark(ctx context.Context, player uuid.UUID, mark string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE players SET tutorial = tutorial || jsonb_build_object($2::text, true)
		WHERE id = $1 AND NOT (tutorial ? $2::text)`, player, mark)
	return tag.RowsAffected() == 1, err
}

// ReleaseTutorialMark clears a mark whose action did not happen after all.
func (s *Store) ReleaseTutorialMark(ctx context.Context, player uuid.UUID, mark string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE players SET tutorial = tutorial - $2::text WHERE id = $1`, player, mark)
	return err
}
