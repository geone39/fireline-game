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

// ErrInvalidRefresh is returned for a malformed, expired, unknown or already used refresh token.
var ErrInvalidRefresh = errors.New("auth: invalid refresh token")

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
	t, jti, err := s.signPair(player)
	if err != nil {
		return t, err
	}
	return t, s.Store.SaveRefreshToken(ctx, jti, player, t.RefreshExpiresAt)
}

func (s *Service) signPair(player uuid.UUID) (Tokens, uuid.UUID, error) {
	var t Tokens
	var err error
	t.AccessToken, t.AccessExpiresAt, err = s.Signer.Issue(KindAccess, player, uuid.New())
	if err != nil {
		return t, uuid.Nil, err
	}
	jti := uuid.New()
	t.RefreshToken, t.RefreshExpiresAt, err = s.Signer.Issue(KindRefresh, player, jti)
	return t, jti, err
}

// Refresh exchanges a valid, unused refresh token for a new pair and revokes the old
// token (rotation). Any failure to validate yields ErrInvalidRefresh; presenting an
// already revoked token is indistinguishable from an unknown one.
func (s *Service) Refresh(ctx context.Context, refreshToken string) (uuid.UUID, Tokens, error) {
	claims, err := s.Signer.Verify(refreshToken, KindRefresh)
	if err != nil {
		return uuid.Nil, Tokens{}, ErrInvalidRefresh
	}
	player, err := claims.PlayerID()
	if err != nil {
		return uuid.Nil, Tokens{}, ErrInvalidRefresh
	}
	oldJTI, err := uuid.Parse(claims.ID)
	if err != nil {
		return uuid.Nil, Tokens{}, ErrInvalidRefresh
	}
	t, newJTI, err := s.signPair(player)
	if err != nil {
		return uuid.Nil, Tokens{}, err
	}
	err = s.Store.RotateRefreshToken(ctx, oldJTI, newJTI, player, t.RefreshExpiresAt, s.Signer.now())
	if errors.Is(err, db.ErrRefreshInvalid) {
		return uuid.Nil, Tokens{}, ErrInvalidRefresh
	}
	if err != nil {
		return uuid.Nil, Tokens{}, err
	}
	return player, t, nil
}
