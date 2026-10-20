package army

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/economy"
)

// ErrNoBase is returned for a player without a base (and therefore without buildings).
var ErrNoBase = errors.New("army: player has no base")

// gemsResource is the wallet resource id of the premium currency (economy.yaml).
const gemsResource = "gems"

// DB is the slice of *pgxpool.Pool the service needs.
type DB interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Ledger is the slice of *economy.Ledger the service needs: the only wallet writer.
type Ledger interface {
	ApplyTx(ctx context.Context, tx pgx.Tx, playerID uuid.UUID, deltas []economy.Delta, reason, idemKey string) (economy.Result, error)
}

// Config wires a Service.
type Config struct {
	DB      DB
	Ledger  Ledger
	Content *Content
	Catalog *base.Catalog   // HQ id for the HQ level
	Prices  *base.PriceBook // the shared gems speed-up function
	Now     func() time.Time
}

// Service is the army application layer. Every operation locks the player's base_state
// row (the same lock the base service takes), so army and base mutations of one player
// are serialised and housing checks see a consistent layout.
type Service struct{ cfg Config }

// NewService validates the config.
func NewService(cfg Config) (*Service, error) {
	if cfg.DB == nil || cfg.Ledger == nil || cfg.Content == nil || cfg.Catalog == nil || cfg.Prices == nil {
		return nil, errors.New("army: Config needs DB, Ledger, Content, Catalog and Prices")
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return &Service{cfg: cfg}, nil
}

// Get returns the army with finished training settled (and persisted).
func (s *Service) Get(ctx context.Context, player uuid.UUID) (State, error) {
	var out State
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.view(w, now)
		return nil
	})
	return out, err
}

// Train queues an order (POST /v1/army/train). key is the request's Idempotency-Key:
// repeating it never queues a second order or charges twice.
func (s *Service) Train(ctx context.Context, player uuid.UUID, key string, req Request) (State, error) {
	if key == "" {
		return State{}, fail("invalid", "no_idempotency_key", "an idempotency key is required")
	}
	var out State
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		id := orderID(player, key)
		if w.hasOrder(id) {
			out = s.view(w, now)
			return nil
		}
		o, perr := w.planTrain(req, now.Truncate(time.Second))
		if perr != nil {
			return perr
		}
		o.ID = id
		total := map[string]int64{}
		for r, c := range o.Cost {
			total[r] = c * int64(o.Count)
		}
		res, err := s.charge(ctx, tx, player, base.Spend(total), "army_train", "army:train:"+key)
		if err != nil {
			return err
		}
		if !res.Replayed {
			w.orders = append(w.orders, o)
			w.sortOrders()
		}
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.view(w, now)
		return nil
	})
	return out, err
}

// Cancel removes count units (default: all that remain) from the tail of an order and
// refunds them: units not started at army.yaml cancel_refund_unstarted_pct, the unit in
// progress at cancel_refund_started_pct. Finished units stay in the stock.
func (s *Service) Cancel(ctx context.Context, player uuid.UUID, key, orderID string, count int) (State, error) {
	if key == "" {
		return State{}, fail("invalid", "no_idempotency_key", "an idempotency key is required")
	}
	var out State
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		i := w.orderIndex(orderID)
		if i < 0 {
			// Finished, cancelled before, or never existed: an idempotent no-op would hide
			// typos, so only a replayed refund key is silent.
			if replayed, err := s.refundSeen(ctx, tx, player, key); err != nil {
				return err
			} else if replayed {
				out = s.view(w, now)
				return nil
			}
			return fail("not_found", "no_order", "order %q not found", orderID)
		}
		o := w.orders[i]
		if count == 0 {
			count = o.Remaining
		}
		if count < 1 || count > o.Remaining {
			return fail("invalid", "bad_count", "count must be 1..%d", o.Remaining)
		}
		plan := w.planCancel(o, count, now)
		if len(plan.refund) > 0 {
			res, err := s.charge(ctx, tx, player, base.Credit(plan.refund), "army_cancel", "army:cancel:"+key)
			if err != nil {
				return err
			}
			if res.Replayed {
				out = s.view(w, now)
				return nil
			}
		}
		headRemoved := false
		headAt := now.Truncate(time.Second)
		if q := w.queue(o.Building); q[0].ID == o.ID && count == o.Remaining {
			headRemoved = true
			if o.NextAt.After(headAt) {
				headAt = o.NextAt
			}
		}
		w.orders[i].Remaining -= count
		w.orders[i].Count -= count
		if w.orders[i].Remaining == 0 {
			w.orders = append(w.orders[:i], w.orders[i+1:]...)
		}
		w.rechain(o.Building, !headRemoved, headAt)
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.view(w, now)
		return nil
	})
	return out, err
}

// Speedup finishes the whole queue of a building for gems; the price comes from the same
// function the base uses for construction.
func (s *Service) Speedup(ctx context.Context, player uuid.UUID, key, building string) (State, error) {
	if key == "" {
		return State{}, fail("invalid", "no_idempotency_key", "an idempotency key is required")
	}
	var out State
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		q := w.queue(building)
		if len(q) == 0 {
			if replayed, err := s.gemsSeen(ctx, tx, player, key); err != nil {
				return err
			} else if replayed {
				out = s.view(w, now)
				return nil
			}
			return fail("invalid", "empty_queue", "building %q has nothing in its queue", building)
		}
		left := int64((w.queueEnd(building).Sub(now) + time.Second - 1) / time.Second)
		gems := s.cfg.Prices.SpeedupGems(left)
		res, err := s.charge(ctx, tx, player, base.Spend(map[string]int64{gemsResource: gems}), "army_speedup", "army:speedup:"+key)
		if err != nil {
			return err
		}
		if !res.Replayed {
			keep := w.orders[:0:0]
			for _, o := range w.orders {
				if o.Building == building {
					w.stock[o.Unit] += o.Remaining
					continue
				}
				keep = append(keep, o)
			}
			w.orders = keep
		}
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.view(w, now)
		return nil
	})
	return out, err
}

// charge applies ledger deltas; a shortage becomes an insufficient_resources error that
// lists what is missing. The caller's transaction stays usable and is rolled back by it.
func (s *Service) charge(ctx context.Context, tx pgx.Tx, player uuid.UUID, ds []economy.Delta, reason, idemKey string) (economy.Result, error) {
	res, err := s.cfg.Ledger.ApplyTx(ctx, tx, player, ds, reason, idemKey)
	if !errors.Is(err, economy.ErrInsufficientFunds) {
		return res, err
	}
	missing := map[string]int64{}
	for _, d := range ds {
		if d.Amount >= 0 {
			continue
		}
		var have int64
		if err := tx.QueryRow(ctx, `SELECT COALESCE((SELECT amount FROM wallets WHERE player_id = $1 AND resource = $2), 0)`,
			player, d.Resource).Scan(&have); err != nil {
			return economy.Result{}, err
		}
		if have < -d.Amount {
			missing[d.Resource] = -d.Amount - have
		}
	}
	return economy.Result{}, fail("insufficient_resources", "insufficient_resources", "not enough resources").with("missing", missing)
}

// refundSeen / gemsSeen tell whether a ledger entry of this request key already exists,
// so a replayed request after the order is gone stays a no-op instead of an error.
func (s *Service) refundSeen(ctx context.Context, tx pgx.Tx, player uuid.UUID, key string) (bool, error) {
	return ledgerSeen(ctx, tx, player, "army:cancel:"+key)
}

func (s *Service) gemsSeen(ctx context.Context, tx pgx.Tx, player uuid.UUID, key string) (bool, error) {
	return ledgerSeen(ctx, tx, player, "army:speedup:"+key)
}

// ledgerSeen reads the ledger's own key format (economy.Ledger.ApplyTx: "<player>:<key>#0").
func ledgerSeen(ctx context.Context, tx pgx.Tx, player uuid.UUID, idemKey string) (bool, error) {
	var ok bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM ledger WHERE idem_key = $1)`, fmt.Sprintf("%s:%s#0", player, idemKey)).Scan(&ok)
	return ok, err
}

func orderID(player uuid.UUID, key string) string {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("fireline/army/train/"+player.String()+"/"+key)).String()
}

func (w *world) hasOrder(id string) bool { return w.orderIndex(id) >= 0 }

func (w *world) orderIndex(id string) int {
	for i, o := range w.orders {
		if o.ID == id {
			return i
		}
	}
	return -1
}

func (s *Service) inTx(ctx context.Context, f func(tx pgx.Tx, now time.Time) error) error {
	tx, err := s.cfg.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := f(tx, s.cfg.Now()); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

type orderPayload struct {
	Unit      string           `json:"unit_id"`
	Count     int              `json:"count"`
	Remaining int              `json:"remaining"`
	NextAt    int64            `json:"next_at"`
	Seq       int64            `json:"seq"`
	Cost      map[string]int64 `json:"cost"`
}

type deployPayload struct {
	Units  []Slot `json:"units"`
	Spells []Slot `json:"spells"`
}

// load locks the player's base row, reads everything the army depends on and settles
// finished training at now. The caller persists the result with save.
func (s *Service) load(ctx context.Context, tx pgx.Tx, player uuid.UUID, now time.Time) (*world, error) {
	var raw []byte
	err := tx.QueryRow(ctx, `SELECT layout FROM base_state WHERE player_id = $1 FOR UPDATE`, player).Scan(&raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNoBase
	}
	if err != nil {
		return nil, err
	}
	var layout base.Layout
	if err := json.Unmarshal(raw, &layout); err != nil {
		return nil, fmt.Errorf("army: stored layout: %w", err)
	}

	w := &world{c: s.cfg.Content, built: map[string]base.Item{}, pending: map[string]base.Item{},
		stock: map[string]int{}, done: map[string]bool{}}

	rows, err := tx.Query(ctx, `SELECT id, kind, target_id, finish_at, payload FROM timers
		WHERE player_id = $1 AND kind IN ('build', 'upgrade', 'train', 'deploy', 'research') ORDER BY finish_at, id`, player)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var bt []base.Timer
	for rows.Next() {
		var id uuid.UUID
		var kind, target string
		var finish time.Time
		var payload []byte
		if err := rows.Scan(&id, &kind, &target, &finish, &payload); err != nil {
			return nil, err
		}
		switch kind {
		case "build", "upgrade":
			var p struct {
				ToLevel int `json:"to_level"`
			}
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, fmt.Errorf("army: timer %s payload: %w", id, err)
			}
			bt = append(bt, base.Timer{ID: id.String(), Kind: base.TimerKind(kind), Target: target, FinishAt: finish, ToLevel: p.ToLevel})
		case "train":
			var p orderPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, fmt.Errorf("army: timer %s payload: %w", id, err)
			}
			w.orders = append(w.orders, order{ID: id.String(), Building: target, Unit: p.Unit, Count: p.Count, Remaining: p.Remaining,
				NextAt: time.Unix(p.NextAt, 0), Seq: p.Seq, Cost: p.Cost})
		case "research":
			var p researchPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, fmt.Errorf("army: timer %s payload: %w", id, err)
			}
			w.lab = &activeResearch{Node: target, StartedAt: time.Unix(p.StartedAt, 0), FinishAt: finish, Paid: p.Paid}
		case "deploy":
			var p deployPayload
			if err := json.Unmarshal(payload, &p); err != nil {
				return nil, fmt.Errorf("army: timer %s payload: %w", id, err)
			}
			w.deployed = append(w.deployed, deployment{RaidID: target, Expires: finish, Units: p.Units, Spells: p.Spells})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	rows.Close()

	settled, pendingTimers, _ := base.Settle(layout, bt, now)
	w.hq = settled.HQLevel(s.cfg.Catalog)
	for _, it := range settled.Items {
		w.built[it.UID] = it
	}
	for _, t := range pendingTimers {
		if t.Kind == base.TimerBuild {
			if it, ok := w.built[t.Target]; ok {
				w.pending[t.Target] = it
				delete(w.built, t.Target)
			}
		}
	}

	ar, err := tx.Query(ctx, `SELECT unit_id, count FROM army WHERE player_id = $1`, player)
	if err != nil {
		return nil, err
	}
	defer ar.Close()
	for ar.Next() {
		var id string
		var n int
		if err := ar.Scan(&id, &n); err != nil {
			return nil, err
		}
		w.stock[id] += n
	}
	if err := ar.Err(); err != nil {
		return nil, err
	}
	ar.Close()

	rr, err := tx.Query(ctx, `SELECT node_id, state, finish_at FROM research WHERE player_id = $1`, player)
	if err != nil {
		return nil, err
	}
	defer rr.Close()
	for rr.Next() {
		var id, state string
		var fin *time.Time
		if err := rr.Scan(&id, &state, &fin); err != nil {
			return nil, err
		}
		// A research whose timer ran out counts as done even before the lab persists it.
		if state == "done" || (fin != nil && !fin.After(now)) {
			w.done[id] = true
			if state != "done" {
				w.newlyDone = append(w.newlyDone, id)
			}
		}
	}
	if err := rr.Err(); err != nil {
		return nil, err
	}

	w.sortOrders()
	w.settle(now)
	w.settleLab(now)
	return w, nil
}

// save writes the stock, the training timers and the raid escrows. The caller holds the
// player's lock, so replacing the whole state is safe.
func (s *Service) save(ctx context.Context, tx pgx.Tx, player uuid.UUID, w *world) error {
	ids := make([]string, 0, len(w.stock))
	for id, n := range w.stock {
		if n > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		// ASSUMPTION (army-lab.md §1): the level is a property of the type, not of the
		// stack; the table's level column (CHECK >= 1) is always 1 and never read.
		if _, err := tx.Exec(ctx, `INSERT INTO army (player_id, unit_id, level, count) VALUES ($1, $2, 1, $3)
			ON CONFLICT (player_id, unit_id, level) DO UPDATE SET count = EXCLUDED.count`, player, id, w.stock[id]); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM army WHERE player_id = $1 AND unit_id <> ALL($2)`, player, ids); err != nil {
		return err
	}

	keep := make([]uuid.UUID, 0, len(w.orders)+len(w.deployed))
	for _, o := range w.orders {
		id, err := uuid.Parse(o.ID)
		if err != nil {
			return fmt.Errorf("army: order id %q: %w", o.ID, err)
		}
		keep = append(keep, id)
		p, _ := json.Marshal(orderPayload{Unit: o.Unit, Count: o.Count, Remaining: o.Remaining, NextAt: o.NextAt.Unix(), Seq: o.Seq, Cost: o.Cost})
		if _, err := tx.Exec(ctx, `INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload)
			VALUES ($1, $2, 'train', $3, $4, $5::jsonb)
			ON CONFLICT (id) DO UPDATE SET finish_at = EXCLUDED.finish_at, payload = EXCLUDED.payload`,
			id, player, o.Building, o.finish(w.c.units[o.Unit].TrainS), string(p)); err != nil {
			return err
		}
	}
	for _, d := range w.deployed {
		id := deployID(player, d.RaidID)
		keep = append(keep, id)
		p, _ := json.Marshal(deployPayload{Units: d.Units, Spells: d.Spells})
		if _, err := tx.Exec(ctx, `INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload)
			VALUES ($1, $2, 'deploy', $3, $4, $5::jsonb)
			ON CONFLICT (id) DO UPDATE SET finish_at = EXCLUDED.finish_at, payload = EXCLUDED.payload`,
			id, player, d.RaidID, d.Expires, string(p)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM timers WHERE player_id = $1 AND kind IN ('train', 'deploy') AND id <> ALL($2)`, player, keep); err != nil {
		return err
	}
	return s.saveLab(ctx, tx, player, w)
}

func deployID(player uuid.UUID, raid string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("fireline/army/deploy/"+player.String()+"/"+raid))
}
