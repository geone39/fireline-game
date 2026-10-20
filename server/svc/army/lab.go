package army

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/geone39/fireline/server/svc/base"
)

// fuelResource is the wallet resource research is paid in (research.yaml tiers.fuel).
const fuelResource = "fuel"

// activeResearch is the one research in flight (army-lab.md §5.1). Paid is what the player
// was charged, the base of the cancel refund.
type activeResearch struct {
	Node      string
	StartedAt time.Time
	FinishAt  time.Time
	Paid      map[string]int64
}

type researchPayload struct {
	StartedAt int64            `json:"started_at"`
	Paid      map[string]int64 `json:"paid"`
}

// NodeView is one research node for the lab screen.
type NodeView struct {
	ID        string           `json:"id"`
	Available bool             `json:"available"`
	Reason    string           `json:"reason,omitempty"` // why not: coming_soon, already_done, lab_level, lab_busy, prerequisite, locked
	Cost      map[string]int64 `json:"cost"`
	Hours     int64            `json:"hours"`
	ToLevel   int              `json:"to_level,omitempty"`
}

// ActiveView is the research in flight (unix seconds).
type ActiveView struct {
	NodeID    string           `json:"node_id"`
	StartedAt int64            `json:"started_at"`
	FinishAt  int64            `json:"finish_at"`
	Paid      map[string]int64 `json:"paid"`
}

// LabState is the body of GET /v1/lab and of the lab mutations.
type LabState struct {
	LabLevel int            `json:"lab_level"` // 0: no laboratory built
	Active   *ActiveView    `json:"active"`
	Done     []string       `json:"done"`
	Nodes    []NodeView     `json:"nodes"`
	Perks    map[string]int `json:"perks"`
}

// Lab returns the laboratory with finished research settled (and persisted).
func (s *Service) Lab(ctx context.Context, player uuid.UUID) (LabState, error) {
	var out LabState
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.labView(w)
		return nil
	})
	return out, err
}

// Perks returns the summed perks of the finished research as of now, for consumers in
// other modules (raid: raid_loot_pct, scan_reveals_traps). Bool perks are 0/1. It runs in
// the caller's transaction and writes nothing.
func (s *Service) Perks(ctx context.Context, tx pgx.Tx, player uuid.UUID) (map[string]int, error) {
	w, err := s.load(ctx, tx, player, s.cfg.Now())
	if err != nil {
		return nil, err
	}
	return w.perks(), nil
}

// StartResearch begins a node (POST /v1/lab/research). The price is charged in the same
// transaction as the timer; key is the request's Idempotency-Key.
func (s *Service) StartResearch(ctx context.Context, player uuid.UUID, key, node string) (LabState, error) {
	if key == "" {
		return LabState{}, fail("invalid", "no_idempotency_key", "an idempotency key is required")
	}
	var out LabState
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if seen, err := ledgerSeen(ctx, tx, player, "lab:research:"+key); err != nil {
			return err
		} else if seen {
			out = s.labView(w)
			return nil
		}
		if perr := w.checkStart(node); perr != nil {
			return perr
		}
		nd := w.c.nodes[node]
		cost := map[string]int64{fuelResource: nd.Fuel}
		if _, err := s.charge(ctx, tx, player, base.Spend(cost), "lab_research", "lab:research:"+key); err != nil {
			return err
		}
		start := now.Truncate(time.Second)
		w.lab = &activeResearch{Node: node, StartedAt: start, FinishAt: start.Add(time.Duration(nd.Hours) * time.Hour), Paid: cost}
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.labView(w)
		return nil
	})
	return out, err
}

// CancelResearch drops the research in flight and refunds lab_cancel_refund_pct of what
// was paid (POST /v1/lab/cancel). The node stays undone.
func (s *Service) CancelResearch(ctx context.Context, player uuid.UUID, key string) (LabState, error) {
	if key == "" {
		return LabState{}, fail("invalid", "no_idempotency_key", "an idempotency key is required")
	}
	var out LabState
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if w.lab == nil {
			if err := s.noActive(ctx, tx, player, "lab:cancel:"+key); err != nil {
				return err
			}
			out = s.labView(w)
			return nil
		}
		refund := map[string]int64{}
		for r, v := range w.lab.Paid {
			if x := v * w.c.LabRefund / 100; x > 0 {
				refund[r] = x
			}
		}
		if len(refund) > 0 {
			if _, err := s.charge(ctx, tx, player, base.Credit(refund), "lab_cancel", "lab:cancel:"+key); err != nil {
				return err
			}
		}
		w.lab = nil
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.labView(w)
		return nil
	})
	return out, err
}

// SpeedupResearch finishes the research in flight for gems (POST /v1/lab/speedup); the
// price is the shared speed-up function of the remaining time.
func (s *Service) SpeedupResearch(ctx context.Context, player uuid.UUID, key string) (LabState, error) {
	if key == "" {
		return LabState{}, fail("invalid", "no_idempotency_key", "an idempotency key is required")
	}
	var out LabState
	err := s.inTx(ctx, func(tx pgx.Tx, now time.Time) error {
		w, err := s.load(ctx, tx, player, now)
		if err != nil {
			return err
		}
		if w.lab == nil {
			if err := s.noActive(ctx, tx, player, "lab:speedup:"+key); err != nil {
				return err
			}
			out = s.labView(w)
			return nil
		}
		left := int64((w.lab.FinishAt.Sub(now) + time.Second - 1) / time.Second)
		gems := s.cfg.Prices.SpeedupGems(left)
		if _, err := s.charge(ctx, tx, player, base.Spend(map[string]int64{gemsResource: gems}), "lab_speedup", "lab:speedup:"+key); err != nil {
			return err
		}
		w.finishLab()
		if err := s.save(ctx, tx, player, w); err != nil {
			return err
		}
		out = s.labView(w)
		return nil
	})
	return out, err
}

// noActive answers cancel/speedup with nothing to act on: a replayed key is a silent
// no-op, anything else is an error (a no-op would hide a stale client).
func (s *Service) noActive(ctx context.Context, tx pgx.Tx, player uuid.UUID, idemKey string) error {
	if seen, err := ledgerSeen(ctx, tx, player, idemKey); err != nil {
		return err
	} else if seen {
		return nil
	}
	return fail("conflict", "no_research", "no research in progress")
}

// settleLab turns a research whose time ran out into a done node. Monotone like settle:
// an earlier now finishes nothing, and nothing already done is taken back.
func (w *world) settleLab(now time.Time) {
	if w.lab != nil && !w.lab.FinishAt.After(now) {
		w.finishLab()
	}
}

func (w *world) finishLab() {
	w.done[w.lab.Node] = true
	w.newlyDone = append(w.newlyDone, w.lab.Node)
	w.lab = nil
}

// labLevel is the level of the built laboratory, 0 if there is none.
func (w *world) labLevel() int {
	lv := 0
	for _, it := range w.built {
		if it.Type == w.c.LabBuilding {
			lv = max(lv, it.Level)
		}
	}
	return lv
}

// prerequisites are the nodes that must be done first: the explicit requires, else every
// lower node of the same item (army-lab.md §2.1).
func (w *world) prerequisites(nd Node) []string {
	if len(nd.Requires) > 0 {
		return nd.Requires
	}
	var out []string
	for _, id := range w.c.nodeOrder {
		if o := w.c.nodes[id]; nd.Item != "" && o.Item == nd.Item && o.ToLevel < nd.ToLevel {
			out = append(out, id)
		}
	}
	return out
}

// checkStart runs the start conditions of army-lab.md §5.1 in order; the price is checked
// by the caller when it charges.
func (w *world) checkStart(node string) *Error {
	nd, ok := w.c.nodes[node]
	switch {
	case !ok:
		return fail("invalid", "no_node", "unknown research node %q", node)
	case w.done[node]:
		return fail("invalid", "already_done", "%s is already researched", node)
	case nd.Soon:
		return fail("invalid", "coming_soon", "%s cannot be researched yet", node)
	}
	if have := w.labLevel(); have < nd.Lab {
		return fail("conflict", "lab_level", "laboratory level %d is needed", nd.Lab).with("need", nd.Lab).with("have", have)
	}
	if w.lab != nil {
		return fail("conflict", "lab_busy", "another research is in progress").with("node_id", w.lab.Node)
	}
	var missing []string
	for _, p := range w.prerequisites(nd) {
		if !w.done[p] {
			missing = append(missing, p)
		}
	}
	if len(missing) > 0 {
		return fail("conflict", "prerequisite", "earlier research is missing").with("missing", missing)
	}
	if u, ok := w.c.units[nd.Item]; ok && w.hq < u.NeedHQ {
		return fail("conflict", "locked", "%s is locked", nd.Item).with("need", map[string]int{"hq": u.NeedHQ})
	}
	return nil
}

func (s *Service) labView(w *world) LabState {
	out := LabState{LabLevel: w.labLevel(), Done: []string{}, Nodes: []NodeView{}, Perks: w.perks()}
	if w.lab != nil {
		out.Active = &ActiveView{NodeID: w.lab.Node, StartedAt: w.lab.StartedAt.Unix(), FinishAt: w.lab.FinishAt.Unix(), Paid: w.lab.Paid}
	}
	for id := range w.done {
		out.Done = append(out.Done, id)
	}
	sort.Strings(out.Done)
	for _, id := range w.c.nodeOrder {
		nd := w.c.nodes[id]
		v := NodeView{ID: id, Cost: map[string]int64{fuelResource: nd.Fuel}, Hours: nd.Hours, ToLevel: nd.ToLevel}
		if err := w.checkStart(id); err != nil {
			v.Reason = err.Reason
		} else {
			v.Available = true
		}
		out.Nodes = append(out.Nodes, v)
	}
	return out
}

func researchID(player uuid.UUID, node string) uuid.UUID {
	return uuid.NewSHA1(uuid.NameSpaceURL, []byte("fireline/army/research/"+player.String()+"/"+node))
}

// saveLab persists finished nodes and the research in flight; the caller holds the
// player's lock.
func (s *Service) saveLab(ctx context.Context, tx pgx.Tx, player uuid.UUID, w *world) error {
	for _, id := range w.newlyDone {
		if _, err := tx.Exec(ctx, `INSERT INTO research (player_id, node_id, state, finish_at) VALUES ($1, $2, 'done', NULL)
			ON CONFLICT (player_id, node_id) DO UPDATE SET state = 'done'`, player, id); err != nil {
			return err
		}
	}
	w.newlyDone = nil
	keep, active := make([]uuid.UUID, 0, 1), make([]string, 0, 1)
	if l := w.lab; l != nil {
		id := researchID(player, l.Node)
		keep, active = append(keep, id), append(active, l.Node)
		if _, err := tx.Exec(ctx, `INSERT INTO research (player_id, node_id, state, finish_at) VALUES ($1, $2, 'researching', $3)
			ON CONFLICT (player_id, node_id) DO UPDATE SET state = 'researching', finish_at = EXCLUDED.finish_at`, player, l.Node, l.FinishAt); err != nil {
			return err
		}
		p, _ := json.Marshal(researchPayload{StartedAt: l.StartedAt.Unix(), Paid: l.Paid})
		if _, err := tx.Exec(ctx, `INSERT INTO timers (id, player_id, kind, target_id, finish_at, payload)
			VALUES ($1, $2, 'research', $3, $4, $5::jsonb)
			ON CONFLICT (id) DO UPDATE SET finish_at = EXCLUDED.finish_at, payload = EXCLUDED.payload`,
			id, player, l.Node, l.FinishAt, string(p)); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `DELETE FROM research WHERE player_id = $1 AND state = 'researching' AND node_id <> ALL($2)`, player, active); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `DELETE FROM timers WHERE player_id = $1 AND kind = 'research' AND id <> ALL($2)`, player, keep)
	return err
}
