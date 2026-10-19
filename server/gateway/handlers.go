package gateway

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"time"

	"github.com/geone39/fireline/server/auth"
	"github.com/geone39/fireline/server/db"
)

type guestRequest struct {
	DeviceID string `json:"device_id"`
}

type guestResponse struct {
	PlayerID string      `json:"player_id"`
	DeviceID string      `json:"device_id"`
	Tokens   auth.Tokens `json:"tokens"`
}

func (s *Server) handleGuest(w http.ResponseWriter, r *http.Request) {
	var req guestRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil {
		s.writeError(w, http.StatusBadRequest, CodeInvalid, "unreadable body", nil)
		return
	}
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			s.writeError(w, http.StatusBadRequest, CodeInvalid, "body must be JSON {\"device_id\": string}", nil)
			return
		}
	}
	p, tokens, deviceID, err := s.auth.Guest(r.Context(), req.DeviceID)
	if errors.Is(err, auth.ErrInvalidDeviceID) {
		s.writeError(w, http.StatusBadRequest, CodeInvalid, "device_id must match [A-Za-z0-9_-]{16,64}", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	s.writeJSON(w, http.StatusOK, guestResponse{PlayerID: p.ID.String(), DeviceID: deviceID, Tokens: tokens})
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

type refreshResponse struct {
	PlayerID string      `json:"player_id"`
	Tokens   auth.Tokens `json:"tokens"`
}

// handleRefresh rotates a refresh token: the old jti is revoked, a new pair is returned.
// Unknown, expired, revoked or malformed tokens all give 401 unauthorized.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || json.Unmarshal(body, &req) != nil || req.RefreshToken == "" {
		s.writeError(w, http.StatusBadRequest, CodeInvalid, "body must be JSON {\"refresh_token\": string}", nil)
		return
	}
	player, tokens, err := s.auth.Refresh(r.Context(), req.RefreshToken)
	if errors.Is(err, auth.ErrInvalidRefresh) {
		s.metrics.refresh.WithLabelValues("invalid").Inc()
		s.writeError(w, http.StatusUnauthorized, CodeUnauthorized, "invalid, expired or already used refresh token", nil)
		return
	}
	if err != nil {
		s.metrics.refresh.WithLabelValues("error").Inc()
		s.internal(w, r, err)
		return
	}
	s.metrics.refresh.WithLabelValues("ok").Inc()
	s.writeJSON(w, http.StatusOK, refreshResponse{PlayerID: player.String(), Tokens: tokens})
}

// builder is a placeholder shape for the builders list of GET /v1/me.
// Data comes from svc/base (BaseModule.builders); without a base module the list is empty. Fields are additive-only.
type builder struct {
	ID        string     `json:"id"`
	BusyUntil *time.Time `json:"busy_until"`
	TargetID  string     `json:"target_id,omitempty"`
}

type profile struct {
	ID          string     `json:"id"`
	Nickname    string     `json:"nickname"`
	HQLevel     int        `json:"hq_level"`
	Trophies    int        `json:"trophies"`
	League      string     `json:"league"`
	ShieldUntil *time.Time `json:"shield_until"`
	CreatedAt   time.Time  `json:"created_at"`
}

type meResponse struct {
	Profile  profile          `json:"profile"`
	Wallets  map[string]int64 `json:"wallets"`
	Builders []builder        `json:"builders"`
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	id, _ := PlayerFrom(r.Context())
	p, err := s.store.GetPlayer(r.Context(), id)
	if errors.Is(err, db.ErrNotFound) {
		s.writeError(w, http.StatusUnauthorized, CodeUnauthorized, "player no longer exists", nil)
		return
	}
	if err != nil {
		s.internal(w, r, err)
		return
	}
	wallets, err := s.store.Wallets(r.Context(), id)
	if err != nil {
		s.internal(w, r, err)
		return
	}
	builders := []builder{}
	if s.base != nil {
		if builders, err = s.base.builders(r.Context(), id); err != nil {
			s.internal(w, r, err)
			return
		}
	}
	s.writeJSON(w, http.StatusOK, meResponse{
		Profile: profile{ID: p.ID.String(), Nickname: p.Nickname, HQLevel: p.HQLevel, Trophies: p.Trophies,
			League: p.League, ShieldUntil: p.ShieldUntil, CreatedAt: p.CreatedAt},
		Wallets:  wallets,
		Builders: builders,
	})
}

func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	s.writeJSON(w, http.StatusOK, map[string]any{"files": s.content.files, "bundle_url": s.content.bundleURL})
}
