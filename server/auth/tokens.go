// Package auth issues and verifies JWT access/refresh tokens and implements guest login.
package auth

import (
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

// Token kinds stored in the "typ" claim.
const (
	KindAccess  = "access"
	KindRefresh = "refresh"
)

// MinSecretLen is the minimum accepted JWT secret size in bytes.
const MinSecretLen = 32

// ErrInvalidToken is returned for any token verification failure.
var ErrInvalidToken = errors.New("auth: invalid token")

// Claims are the JWT claims used by Fireline.
type Claims struct {
	Kind string `json:"typ"`
	jwt.RegisteredClaims
}

// PlayerID returns the subject as uuid.
func (c Claims) PlayerID() (uuid.UUID, error) { return uuid.Parse(c.Subject) }

// Signer signs and verifies HS256 tokens.
type Signer struct {
	Secret     []byte
	AccessTTL  time.Duration
	RefreshTTL time.Duration
	Now        func() time.Time // optional, for tests
}

func (s *Signer) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Issue signs a token of the given kind for a player; jti is the token id.
func (s *Signer) Issue(kind string, player, jti uuid.UUID) (string, time.Time, error) {
	ttl := s.AccessTTL
	if kind == KindRefresh {
		ttl = s.RefreshTTL
	}
	now := s.now()
	exp := now.Add(ttl)
	c := Claims{Kind: kind, RegisteredClaims: jwt.RegisteredClaims{
		Subject:   player.String(),
		ID:        jti.String(),
		IssuedAt:  jwt.NewNumericDate(now),
		ExpiresAt: jwt.NewNumericDate(exp),
	}}
	tok, err := jwt.NewWithClaims(jwt.SigningMethodHS256, c).SignedString(s.Secret)
	return tok, exp, err
}

// Verify parses a token and checks its kind.
func (s *Signer) Verify(token, kind string) (Claims, error) {
	var c Claims
	_, err := jwt.ParseWithClaims(token, &c, func(*jwt.Token) (any, error) { return s.Secret, nil },
		jwt.WithValidMethods([]string{"HS256"}), jwt.WithTimeFunc(s.now), jwt.WithExpirationRequired())
	if err != nil {
		return Claims{}, fmt.Errorf("%w: %v", ErrInvalidToken, err)
	}
	if c.Kind != kind {
		return Claims{}, ErrInvalidToken
	}
	return c, nil
}
