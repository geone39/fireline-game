package gateway

import (
	"encoding/json"
	"net/http"
	"time"
)

// Error codes from docs/03-api.md, plus the ones recorded in docs/open-questions.md
// (OQ-S1): unauthorized, not_found.
const (
	CodeInvalid       = "invalid"
	CodeConflict      = "conflict"
	CodeContentOld    = "content_outdated"
	CodeRateLimited   = "rate_limited"
	CodeInternal      = "internal"
	CodeUnauthorized  = "unauthorized"
	CodeNotFound      = "not_found"
	headerRequestID   = "X-Request-ID"
	headerIdempotency = "Idempotency-Key"
	headerContentVer  = "X-Content-Version"
)

// ErrorBody is the error envelope: {error:{code,message,details}}.
type ErrorBody struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details"`
}

func (s *Server) meta() map[string]json.RawMessage {
	t, _ := json.Marshal(s.now().UTC().Format(time.RFC3339Nano))
	v, _ := json.Marshal(s.content.Version())
	return map[string]json.RawMessage{"server_time": t, "content_version": v}
}

// writeJSON writes v (a struct or map serialising to a JSON object) with server_time
// and content_version merged in.
func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	raw, err := json.Marshal(v)
	if err != nil {
		status, raw = http.StatusInternalServerError, []byte(`{}`)
	}
	out := map[string]json.RawMessage{}
	if err := json.Unmarshal(raw, &out); err != nil {
		out = map[string]json.RawMessage{}
	}
	for k, m := range s.meta() {
		out[k] = m
	}
	body, _ := json.Marshal(out)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set(headerContentVer, s.content.Version())
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

func (s *Server) writeError(w http.ResponseWriter, status int, code, msg string, details any) {
	if details == nil {
		details = map[string]any{}
	}
	s.writeJSON(w, status, map[string]ErrorBody{"error": {Code: code, Message: msg, Details: details}})
}
