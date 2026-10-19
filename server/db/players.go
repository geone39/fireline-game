package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNotFound is returned when a row does not exist.
var ErrNotFound = errors.New("db: not found")

// Player is the profile row of players.
type Player struct {
	ID          uuid.UUID
	CreatedAt   time.Time
	Provider    string
	Nickname    string
	HQLevel     int
	Trophies    int
	League      string
	ShieldUntil *time.Time
	LastSeen    time.Time
}

const playerCols = `id, created_at, auth_provider, nickname, hq_level, trophies, league, shield_until, last_seen`

func scanPlayer(row pgx.Row) (Player, error) {
	var p Player
	err := row.Scan(&p.ID, &p.CreatedAt, &p.Provider, &p.Nickname, &p.HQLevel, &p.Trophies, &p.League, &p.ShieldUntil, &p.LastSeen)
	return p, err
}

// UpsertPlayer returns the player for (provider, extID), creating it with the given
// nickname if it does not exist, and refreshes last_seen.
func (s *Store) UpsertPlayer(ctx context.Context, provider, extID, nickname string) (Player, error) {
	return scanPlayer(s.Pool.QueryRow(ctx, `
		INSERT INTO players (id, auth_provider, ext_id, nickname)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (auth_provider, ext_id) DO UPDATE SET last_seen = now()
		RETURNING `+playerCols, uuid.New(), provider, extID, nickname))
}

// GetPlayer loads a player by id.
func (s *Store) GetPlayer(ctx context.Context, id uuid.UUID) (Player, error) {
	p, err := scanPlayer(s.Pool.QueryRow(ctx, `SELECT `+playerCols+` FROM players WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return Player{}, ErrNotFound
	}
	return p, err
}

// Wallets returns resource -> amount for a player.
func (s *Store) Wallets(ctx context.Context, id uuid.UUID) (map[string]int64, error) {
	rows, err := s.Pool.Query(ctx, `SELECT resource, amount FROM wallets WHERE player_id = $1`, id)
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

// SaveRefreshToken records an issued refresh token.
func (s *Store) SaveRefreshToken(ctx context.Context, jti, player uuid.UUID, expires time.Time) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO refresh_tokens (jti, player_id, expires_at) VALUES ($1, $2, $3)`, jti, player, expires)
	return err
}

// ErrRefreshInvalid is returned when a refresh token is unknown, revoked or expired.
var ErrRefreshInvalid = errors.New("db: refresh token invalid")

// RotateRefreshToken atomically revokes the refresh token oldJTI (only if it is still
// active at now) and records newJTI for the same player. The conditional UPDATE makes
// concurrent use of the same token succeed at most once.
func (s *Store) RotateRefreshToken(ctx context.Context, oldJTI, newJTI uuid.UUID, player uuid.UUID, newExpires, now time.Time) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	tag, err := tx.Exec(ctx, `
		UPDATE refresh_tokens SET revoked_at = $3
		WHERE jti = $1 AND player_id = $2 AND revoked_at IS NULL AND expires_at > $3`, oldJTI, player, now)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrRefreshInvalid
	}
	if _, err := tx.Exec(ctx, `INSERT INTO refresh_tokens (jti, player_id, expires_at) VALUES ($1, $2, $3)`, newJTI, player, newExpires); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
