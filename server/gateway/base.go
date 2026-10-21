package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"go.yaml.in/yaml/v3"

	"github.com/geone39/fireline/server/db"
	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/economy"
)

// BaseModule wires svc/base and svc/economy into the gateway: the /v1/base routes, the
// defence-snapshot sink, the guest provisioning step and the builders of GET /v1/me.
type BaseModule struct {
	Service *base.Service
	Ledger  *economy.Ledger
	Cat     *base.Catalog
	Grid    base.Grid
	Store   *db.Store
	Now     func() time.Time
	// Army, when set, adds the start army to Provision (army.yaml start_army_auto_grant).
	Army *ArmyModule

	start *base.Layout // from content/start_layout.yaml; nil: HQ only
}

// BaseConfig is everything NewBaseModule needs; all of it comes from content/*.yaml.
type BaseConfig struct {
	Store  *db.Store
	Econ   *economy.Config
	Cat    *base.Catalog
	Grid   base.Grid
	Prices *base.PriceBook
	// ContentDir, when set, is where start_layout.yaml is read from; empty means the HQ-only layout.
	ContentDir string
	Now        func() time.Time // optional, for tests
}

// NewBaseModule builds the base service with the real ledger and snapshot sink.
func NewBaseModule(c BaseConfig) (*BaseModule, error) {
	m := &BaseModule{Ledger: economy.NewLedger(c.Store.Pool, c.Econ), Cat: c.Cat, Grid: c.Grid, Store: c.Store, Now: c.Now}
	if c.ContentDir != "" {
		l, err := loadStartLayout(c.ContentDir)
		if err != nil {
			return nil, err
		}
		m.start = &l
	}
	svc, err := base.NewService(base.Config{
		DB: c.Store.Pool, Ledger: m.Ledger, Econ: c.Econ,
		Engine:    base.Engine{Rules: base.Rules{Cat: c.Cat, Grid: c.Grid}, Prices: c.Prices},
		Snapshots: snapshotSink{c.Store}, Now: c.Now,
	})
	if err != nil {
		return nil, err
	}
	m.Service = svc
	return m, nil
}

// snapshotSink implements base.SnapshotSink on top of db.Store.
type snapshotSink struct{ store *db.Store }

func (k snapshotSink) SaveSnapshot(ctx context.Context, tx pgx.Tx, player uuid.UUID, baseVersion, hqLevel int, l base.Layout) error {
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	return k.store.InsertSnapshot(ctx, tx, player, baseVersion, hqLevel, b)
}

// startLayoutFile is content/start_layout.yaml (OQ-S6, army-lab.md §7): same item format as reference_bases.
type startLayoutFile struct {
	Layout struct {
		Items []struct {
			UID   string `yaml:"uid"`
			Type  string `yaml:"type"`
			Level int    `yaml:"level"`
			X     int    `yaml:"x"`
			Y     int    `yaml:"y"`
			Rot   int    `yaml:"rot"`
		} `yaml:"items"`
	} `yaml:"layout"`
}

func loadStartLayout(dir string) (base.Layout, error) {
	raw, err := os.ReadFile(filepath.Join(dir, "start_layout.yaml"))
	if err != nil {
		return base.Layout{}, fmt.Errorf("gateway: %w", err)
	}
	var f startLayoutFile
	if err := yaml.Unmarshal(raw, &f); err != nil {
		return base.Layout{}, fmt.Errorf("gateway: start_layout.yaml: %w", err)
	}
	var l base.Layout
	for _, it := range f.Layout.Items {
		l.Items = append(l.Items, base.Item{UID: it.UID, Type: it.Type, Level: it.Level, X: it.X, Y: it.Y, Rot: it.Rot})
	}
	return l, nil
}

// StartLayout is the layout of a new base: content/start_layout.yaml (HQ L1 and a ready
// camp), validated against the catalog and the grid. Without ContentDir (tests) it falls
// back to the HQ alone in the middle of the grid; no numbers live here.
func (m *BaseModule) StartLayout() (base.Layout, error) {
	rules := base.Rules{Cat: m.Cat, Grid: m.Grid}
	if m.start != nil {
		if err := rules.ValidateLayout(*m.start); err != nil {
			return base.Layout{}, fmt.Errorf("gateway: start layout: %w", err)
		}
		return m.start.Clone(), nil
	}
	d, ok := m.Cat.Def(m.Cat.HQID())
	if !ok {
		return base.Layout{}, errors.New("gateway: catalog has no HQ")
	}
	w, h := base.Footprint(d, 0)
	l := base.Layout{Items: []base.Item{{UID: "hq", Type: d.ID, Level: 1, X: (m.Grid.W - w) / 2, Y: (m.Grid.H - h) / 2}}}
	if err := rules.ValidateLayout(l); err != nil {
		return base.Layout{}, fmt.Errorf("gateway: start layout: %w", err)
	}
	return l, nil
}

// Provision creates the start base and credits the start grant in the caller's
// transaction (auth.Service.Provision). Both parts are idempotent: the base row is
// written ON CONFLICT DO NOTHING and the grant has the ledger key start_grant:<player>.
// ASSUMPTION (OQ-S6): svc/base has no InitBase variant taking a tx, so the same INSERT
// as base.Service.InitBase is done here to keep base and grant atomic.
func (m *BaseModule) Provision(ctx context.Context, tx pgx.Tx, player uuid.UUID) error {
	l, err := m.StartLayout()
	if err != nil {
		return err
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	now := time.Now()
	if m.Now != nil {
		now = m.Now()
	}
	if _, err := tx.Exec(ctx, `INSERT INTO base_state (player_id, layout, updated_at) VALUES ($1, $2::jsonb, $3)
		ON CONFLICT (player_id) DO NOTHING`, player, string(b), now); err != nil {
		return err
	}
	if _, err = m.Ledger.GrantStartTx(ctx, tx, player); err != nil {
		return err
	}
	if m.Army != nil {
		return m.Army.provision(ctx, tx, player)
	}
	return nil
}

// routes mounts the /v1/base routes inside the authenticated group.
func (m *BaseModule) routes(s *Server) func(r chi.Router) {
	return func(r chi.Router) {
		base.Mount(r, base.Deps{
			Service:        m.Service,
			PlayerID:       PlayerFrom,
			ContentVersion: s.content.Version,
			Now:            s.now,
			OnError: func(req *http.Request, err error) {
				s.log.Error().Err(err).Str("request_id", RequestIDFrom(req.Context())).Msg("base: internal error")
			},
		})
	}
}

// builders lists builders for GET /v1/me: one per slot, busy ones carry their timer.
// ASSUMPTION: ids are "b1".."bN"; busy builders are matched to timers by finish time.
func (m *BaseModule) builders(ctx context.Context, player uuid.UUID) ([]builder, error) {
	st, err := m.Service.Get(ctx, player)
	if errors.Is(err, base.ErrNoBase) {
		return []builder{}, nil
	}
	if err != nil {
		return nil, err
	}
	timers := append([]base.Timer(nil), st.Timers...)
	sort.Slice(timers, func(i, j int) bool { return timers[i].FinishAt.Before(timers[j].FinishAt) })
	out := make([]builder, 0, st.Builders.Total)
	for i := 0; i < st.Builders.Total; i++ {
		b := builder{ID: fmt.Sprintf("b%d", i+1)}
		if i < len(timers) {
			t := timers[i].FinishAt
			b.BusyUntil, b.TargetID = &t, timers[i].Target
		}
		out = append(out, b)
	}
	return out, nil
}
