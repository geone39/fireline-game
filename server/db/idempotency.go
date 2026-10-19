package db

import (
	"bytes"
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// IdemRecord is a stored idempotent response.
type IdemRecord struct {
	RequestHash []byte
	Done        bool
	Status      int
	ContentType string
	Body        []byte
}

// SameRequest reports whether the stored request hash equals h.
func (r IdemRecord) SameRequest(h []byte) bool { return bytes.Equal(r.RequestHash, h) }

// IdemBegin tries to claim (player, key). It returns started=true if this call
// claimed the key; otherwise rec describes the existing record.
func (s *Store) IdemBegin(ctx context.Context, player uuid.UUID, key string, hash []byte) (started bool, rec IdemRecord, err error) {
	tag, err := s.Pool.Exec(ctx, `
		INSERT INTO idempotency_keys (player_id, key, request_hash) VALUES ($1, $2, $3)
		ON CONFLICT DO NOTHING`, player, key, hash)
	if err != nil {
		return false, rec, err
	}
	if tag.RowsAffected() == 1 {
		return true, rec, nil
	}
	var status *int
	var ct *string
	err = s.Pool.QueryRow(ctx, `
		SELECT request_hash, status, content_type, response FROM idempotency_keys
		WHERE player_id = $1 AND key = $2`, player, key).Scan(&rec.RequestHash, &status, &ct, &rec.Body)
	if errors.Is(err, pgx.ErrNoRows) { // released between the two statements
		return false, rec, ErrNotFound
	}
	if err != nil {
		return false, rec, err
	}
	if status != nil {
		rec.Done, rec.Status = true, *status
		if ct != nil {
			rec.ContentType = *ct
		}
	}
	return false, rec, nil
}

// IdemComplete stores the response for a claimed key.
func (s *Store) IdemComplete(ctx context.Context, player uuid.UUID, key string, status int, contentType string, body []byte) error {
	_, err := s.Pool.Exec(ctx, `
		UPDATE idempotency_keys SET status = $3, content_type = $4, response = $5
		WHERE player_id = $1 AND key = $2`, player, key, status, contentType, body)
	return err
}

// IdemRelease drops a claimed key so the client can retry (used for 5xx responses).
func (s *Store) IdemRelease(ctx context.Context, player uuid.UUID, key string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE player_id = $1 AND key = $2`, player, key)
	return err
}
