package army

import (
	"sort"
	"time"
)

// Count is a stock line: how many of a unit or spell the player has at home and the
// research-derived level of that type.
type Count struct {
	ID    string `json:"id"`
	Count int    `json:"count"`
	Level int    `json:"level"`
}

// PoolState is one housing pool (army-lab.md §8).
type PoolState struct {
	Capacity int `json:"capacity"`
	Used     int `json:"used"`
	Free     int `json:"free"`
	Base     int `json:"base"`
	PerkPct  int `json:"perk_pct"`
}

// OrderView is an order in a building's queue.
type OrderView struct {
	OrderID      string `json:"order_id"`
	UnitID       string `json:"unit_id"`
	Count        int    `json:"count"`
	Remaining    int    `json:"remaining"`
	UnitFinishAt int64  `json:"unit_finish_at"` // unix seconds, when the unit in progress is done
}

// QueueView is the queue of one producing building.
type QueueView struct {
	BuildingID string      `json:"building_id"`
	Orders     []OrderView `json:"orders"`
	FinishAt   int64       `json:"finish_at"` // unix seconds, when the whole queue is done
}

// DeployedView is the escrow of one active raid.
type DeployedView struct {
	RaidID string `json:"raid_id"`
	Units  []Slot `json:"units"`
	Spells []Slot `json:"spells"`
}

// State is ArmyState, the body of GET /v1/army.
type State struct {
	Stock    []Count              `json:"stock"`
	Spells   []Count              `json:"spells"`
	Deployed []DeployedView       `json:"deployed"`
	Pools    map[string]PoolState `json:"pools"`
	Queues   []QueueView          `json:"queues"`
	Perks    map[string]int       `json:"perks"`
}

func (s *Service) view(w *world, now time.Time) State {
	out := State{
		Stock: []Count{}, Spells: []Count{}, Deployed: []DeployedView{}, Queues: []QueueView{},
		Pools: map[string]PoolState{}, Perks: w.perks(),
	}
	ids := make([]string, 0, len(w.stock))
	for id, n := range w.stock {
		if n > 0 {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	for _, id := range ids {
		c := Count{ID: id, Count: w.stock[id], Level: w.level(id)}
		if w.c.units[id].Spell {
			out.Spells = append(out.Spells, c)
		} else {
			out.Stock = append(out.Stock, c)
		}
	}
	for _, d := range w.deployed {
		out.Deployed = append(out.Deployed, DeployedView{RaidID: d.RaidID, Units: nonNil(d.Units), Spells: nonNil(d.Spells)})
	}
	sort.Slice(out.Deployed, func(i, j int) bool { return out.Deployed[i].RaidID < out.Deployed[j].RaidID })
	for _, p := range w.c.Pools() {
		ps := PoolState{Capacity: w.cap(p), Used: w.used(p), Base: w.baseCap(p)}
		ps.Free = max(ps.Capacity-ps.Used, 0)
		if w.c.perkPools[p] {
			ps.PerkPct = w.perkPct()
		}
		out.Pools[p] = ps
	}
	for _, b := range w.buildings() {
		qv := QueueView{BuildingID: b, Orders: []OrderView{}, FinishAt: w.queueEnd(b).Unix()}
		for _, o := range w.queue(b) {
			qv.Orders = append(qv.Orders, OrderView{OrderID: o.ID, UnitID: o.Unit, Count: o.Count, Remaining: o.Remaining,
				UnitFinishAt: o.NextAt.Add(time.Duration(w.c.units[o.Unit].TrainS) * time.Second).Unix()})
		}
		out.Queues = append(out.Queues, qv)
	}
	return out
}

func nonNil(s []Slot) []Slot {
	if s == nil {
		return []Slot{}
	}
	return s
}
