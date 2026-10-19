package auth

import (
	"context"
	"errors"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/geone39/fireline/server/db"
)

// ProviderGuest is the auth_provider value for guest accounts.
const ProviderGuest = "guest"

// ErrInvalidDeviceID is returned for a malformed device_id.
var ErrInvalidDeviceID = errors.New("auth: invalid device_id")

var deviceIDRe = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

// Tokens is the token pair returned by login.
type Tokens struct {
	AccessToken      string    `json:"access_token"`
	AccessExpiresAt  time.Time `json:"access_expires_at"`
	RefreshToken     string    `json:"refresh_token"`
	RefreshExpiresAt time.Time `json:"refresh_expires_at"`
}

// Service implements guest login on top of the store.
type Service struct {
	Store  *db.Store
	Signer *Signer
}

// Guest creates or returns a guest player. deviceID is a client-generated random secret;
// when empty a new one is generated and returned so the client can persist it.
func (s *Service) Guest(ctx context.Context, deviceID string) (db.Player, Tokens, string, error) {
	if deviceID == "" {
		deviceID = uuid.NewString()
	} else if !deviceIDRe.MatchString(deviceID) {
		return db.Player{}, Tokens{}, "", ErrInvalidDeviceID
	}
	nick := "Guest-" + uuid.NewString()[:6]
	p, err := s.Store.UpsertPlayer(ctx, ProviderGuest, deviceID, nick)
	if err != nil {
		return db.Player{}, Tokens{}, "", err
	}
	t, err := s.issuePair(ctx, p.ID)
	return p, t, deviceID, err
}

func (s *Service) issuePair(ctx context.Context, player uuid.UUID) (Tokens, error) {
	var t Tokens
	var err error
	t.AccessToken, t.AccessExpiresAt, err = s.Signer.Issue(KindAccess, player, uuid.New())
	if err != nil {
		return t, err
	}
	jti := uuid.New()
	t.RefreshToken, t.RefreshExpiresAt, err = s.Signer.Issue(KindRefresh, player, jti)
	if err != nil {
		return t, err
	}
	return t, s.Store.SaveRefreshToken(ctx, jti, player, t.RefreshExpiresAt)
}
