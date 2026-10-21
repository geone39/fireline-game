package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.yaml.in/yaml/v3"

	"github.com/geone39/fireline/server/db"
	"github.com/geone39/fireline/server/svc/army"
	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/economy"
)

// Tutorial marks in players.tutorial.
const (
	markFreeFinish   = "free_finish_used"
	markStartArmy    = "start_army_granted"
	tutorialFreeKey  = "tutorial-free-finish"
	maxTrainBodySize = 1 << 16
)

// ArmyModule wires svc/army into the gateway: /v1/army*, /v1/lab*, the tutorial free
// finish of the first army order and the army/lab part of GET /v1/me.
type ArmyModule struct {
	Service *army.Service
	Store   *db.Store

	tut tutorialConfig
}

type startUnit struct {
	ID    string
	Count int
}

// tutorialConfig is the part of content/army.yaml that is about new players.
type tutorialConfig struct {
	FreeFinishFirstOrder bool
	StartArmyAutoGrant   bool
	StartArmy            []startUnit
}

// ArmyConfig is everything NewArmyModule needs; all numbers come from content/*.yaml.
type ArmyConfig struct {
	Store      *db.Store
	Ledger     *economy.Ledger
	ContentDir string
	Cat        *base.Catalog
	Prices     *base.PriceBook
	Now        func() time.Time // optional, for tests
}

// NewArmyModule loads army.yaml, troops.yaml, buildings.yaml and research.yaml and builds the service.
func NewArmyModule(c ArmyConfig) (*ArmyModule, error) {
	content, err := army.LoadContent(c.ContentDir)
	if err != nil {
		return nil, err
	}
	tut, err := loadTutorial(c.ContentDir)
	if err != nil {
		return nil, err
	}
	svc, err := army.NewService(army.Config{
		DB: c.Store.Pool, Ledger: tutorialLedger{c.Ledger}, Content: content, Catalog: c.Cat, Prices: c.Prices, Now: c.Now,
	})
	if err != nil {
		return nil, err
	}
	return &ArmyModule{Service: svc, Store: c.Store, tut: tut}, nil
}

func loadTutorial(dir string) (tutorialConfig, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "army.yaml"))
	if err != nil {
		return tutorialConfig{}, fmt.Errorf("gateway: %w", err)
	}
	var f struct {
		Tutorial struct {
			FreeFinishFirstOrder bool `yaml:"free_finish_first_order"`
		} `yaml:"tutorial"`
		StartArmyAutoGrant bool `yaml:"start_army_auto_grant"`
		StartArmy          []struct {
			ID    string `yaml:"id"`
			Count int    `yaml:"count"`
		} `yaml:"start_army"`
	}
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return tutorialConfig{}, fmt.Errorf("gateway: army.yaml: %w", err)
	}
	t := tutorialConfig{FreeFinishFirstOrder: f.Tutorial.FreeFinishFirstOrder, StartArmyAutoGrant: f.StartArmyAutoGrant}
	for _, u := range f.StartArmy {
		t.StartArmy = append(t.StartArmy, startUnit{u.ID, u.Count})
	}
	return t, nil
}

type freeCtxKey struct{}

// tutorialLedger is the ledger army uses. It behaves exactly like economy.Ledger except
// for the gems speed-up of the tutorial free finish, which the gateway marks in the
// context: svc/army has no notion of a player flag (OQ-ARMY-11 (5)).
type tutorialLedger struct{ *economy.Ledger }

func (l tutorialLedger) ApplyTx(ctx context.Context, tx pgx.Tx, p uuid.UUID, ds []economy.Delta, reason, idemKey string) (economy.Result, error) {
	if free, _ := ctx.Value(freeCtxKey{}).(bool); free && reason == "army_speedup" {
		return economy.Result{}, nil
	}
	return l.Ledger.ApplyTx(ctx, tx, p, ds, reason, idemKey)
}

// provision grants the reference start army when army.yaml asks for it
// (start_army_auto_grant, false by default: OQ-ARMY-8). It runs inside the guest
// transaction and is idempotent through the players.tutorial mark.
func (m *ArmyModule) provision(ctx context.Context, tx pgx.Tx, player uuid.UUID) error {
	if !m.tut.StartArmyAutoGrant || len(m.tut.StartArmy) == 0 {
		return nil
	}
	ok, err := m.Store.ClaimTutorialMarkTx(ctx, tx, player, markStartArmy)
	if err != nil || !ok {
		return err
	}
	for _, u := range m.tut.StartArmy {
		if _, err := tx.Exec(ctx, `INSERT INTO army (player_id, unit_id, level, count) VALUES ($1, $2, 1, $3)
			ON CONFLICT (player_id, unit_id, level) DO UPDATE SET count = army.count + EXCLUDED.count`,
			player, u.ID, u.Count); err != nil {
			return err
		}
	}
	return nil
}

// routes mounts /v1/army* and /v1/lab* inside the authenticated group.
func (m *ArmyModule) routes(s *Server) func(r chi.Router) {
	deps := army.Deps{
		Service:        m.Service,
		PlayerID:       PlayerFrom,
		ContentVersion: s.content.Version,
		Now:            s.now,
		OnError: func(req *http.Request, err error) {
			s.log.Error().Err(err).Str("request_id", RequestIDFrom(req.Context())).Msg("army: internal error")
		},
	}
	return func(r chi.Router) {
		r.Group(func(r chi.Router) {
			if m.tut.FreeFinishFirstOrder {
				r.Use(m.freeFinish(deps))
			}
			army.Mount(r, deps)
		})
	}
}

// bufWriter buffers a response so it can be replaced.
type bufWriter struct {
	h      http.Header
	status int
	buf    bytes.Buffer
}

func (b *bufWriter) Header() http.Header { return b.h }
func (b *bufWriter) WriteHeader(code int) {
	if b.status == 0 {
		b.status = code
	}
}
func (b *bufWriter) Write(p []byte) (int, error) {
	if b.status == 0 {
		b.status = http.StatusOK
	}
	return b.buf.Write(p)
}

func (b *bufWriter) flushTo(w http.ResponseWriter) {
	for k, v := range b.h {
		w.Header()[k] = v
	}
	if b.status == 0 {
		b.status = http.StatusOK
	}
	w.WriteHeader(b.status)
	_, _ = w.Write(b.buf.Bytes())
}

// freeFinish is the tutorial rule of OQ-ARMY-8: the first army order a player successfully
// queues is finished for free, once. It wraps POST /army/train; the response of that one
// request is replaced by the army state after the finish. The mark is claimed first so
// concurrent first orders cannot both be free, and released when the finish failed.
func (m *ArmyModule) freeFinish(deps army.Deps) func(http.Handler) http.Handler {
	plain := chi.NewRouter()
	army.Mount(plain, deps)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/army/train") {
				next.ServeHTTP(w, r)
				return
			}
			body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxTrainBodySize))
			if err != nil {
				next.ServeHTTP(w, r)
				return
			}
			r.Body = io.NopCloser(bytes.NewReader(body))
			orig := &bufWriter{h: http.Header{}}
			next.ServeHTTP(orig, r)
			if orig.status == http.StatusOK {
				if out := m.finishFirst(r, body, plain); out != nil {
					out.flushTo(w)
					return
				}
			}
			orig.flushTo(w)
		})
	}
}

func (m *ArmyModule) finishFirst(r *http.Request, body []byte, plain http.Handler) *bufWriter {
	ctx := context.WithoutCancel(r.Context())
	player, ok := PlayerFrom(ctx)
	var req struct {
		BuildingID string `json:"building_id"`
	}
	if !ok || json.Unmarshal(body, &req) != nil || req.BuildingID == "" {
		return nil
	}
	claimed, err := m.Store.ClaimTutorialMark(ctx, player, markFreeFinish)
	if err != nil || !claimed {
		return nil
	}
	if _, err := m.Service.Speedup(context.WithValue(ctx, freeCtxKey{}, true), player, tutorialFreeKey, req.BuildingID); err != nil {
		_ = m.Store.ReleaseTutorialMark(ctx, player, markFreeFinish)
		return nil
	}
	// A fresh route context: the inherited one still routes the original POST /army/train.
	get, err := http.NewRequestWithContext(context.WithValue(ctx, chi.RouteCtxKey, chi.NewRouteContext()), http.MethodGet, "/army", nil)
	if err != nil {
		return nil
	}
	out := &bufWriter{h: http.Header{}}
	plain.ServeHTTP(out, get)
	if out.status != http.StatusOK {
		return nil
	}
	return out
}

// meArmy and meLab are the army/lab parts of GET /v1/me (additive fields).
type meArmy struct {
	Pools map[string]mePool `json:"pools"`
}

type mePool struct {
	Used     int `json:"used"`
	Capacity int `json:"capacity"`
}

type meLab struct {
	Level  int         `json:"level"`
	Active *meResearch `json:"active"`
}

type meResearch struct {
	NodeID   string    `json:"node_id"`
	FinishAt time.Time `json:"finish_at"`
}

// me builds the army and lab summaries. A player without a base has neither.
func (m *ArmyModule) me(ctx context.Context, player uuid.UUID) (*meArmy, *meLab, error) {
	st, err := m.Service.Get(ctx, player)
	if errors.Is(err, army.ErrNoBase) {
		return &meArmy{Pools: map[string]mePool{}}, &meLab{}, nil
	}
	if err != nil {
		return nil, nil, err
	}
	a := &meArmy{Pools: make(map[string]mePool, len(st.Pools))}
	for k, p := range st.Pools {
		a.Pools[k] = mePool{Used: p.Used, Capacity: p.Capacity}
	}
	ls, err := m.Service.Lab(ctx, player)
	if err != nil {
		return nil, nil, err
	}
	l := &meLab{Level: ls.LabLevel}
	if ls.Active != nil {
		l.Active = &meResearch{NodeID: ls.Active.NodeID, FinishAt: time.Unix(ls.Active.FinishAt, 0).UTC()}
	}
	return a, l, nil
}
