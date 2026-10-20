package sim

import (
	"reflect"
	"testing"
)

// checkState verifies State() against the live battle: it mirrors the entities (minus live hidden traps), does not
// move the StateHash, shares no memory with the battle and is repeatable.
func checkState(t *testing.T, b *Battle) {
	t.Helper()
	before := b.stateHash()
	s := b.State()
	if b.stateHash() != before {
		t.Fatal("State() changed the battle")
	}
	if s2 := b.State(); !reflect.DeepEqual(s, s2) {
		t.Fatal("State() is not repeatable")
	}
	if int(s.Turn) != b.turn || s.EndReason != b.endReason || int(s.Phase) != int(b.ph) {
		t.Fatalf("header %+v vs turn %d phase %d end %d", s, b.turn, b.ph, b.endReason)
	}
	k := 0
	var last EntityID
	for i := range b.ents {
		e := &b.ents[i]
		if e.bd != nil && e.bd.Hidden && !e.revealed && e.alive {
			continue // a live hidden trap is never part of the client view
		}
		if k >= len(s.Entities) {
			t.Fatalf("entity %d missing in State", e.id)
		}
		es := s.Entities[k]
		k++
		if es.ID != e.id || es.ID <= last {
			t.Fatalf("State entity %d, battle %d (last %d)", es.ID, e.id, last)
		}
		last = es.ID
		if es.HP != e.hp || es.MaxHP != e.maxHP || es.APLeft != e.apLeft || es.AP != e.ap || es.Alive != e.alive ||
			es.OnField != e.onField || es.Pos != e.pos || es.ContentID != e.contentID || es.Level != e.level ||
			es.Aboard != e.aboard || es.Inside != e.inside || !reflect.DeepEqual(append([]EntityID(nil), e.cargo...), append([]EntityID(nil), es.Cargo...)) {
			t.Fatalf("State entity %+v differs from %+v", es, *e)
		}
		if es.HP < 0 || es.HP > es.MaxHP || es.APLeft < 0 || es.APLeft > es.AP {
			t.Fatalf("State entity out of range: %+v", es)
		}
		if es.Alive != (es.HP > 0) && e.kind != KindDecoy {
			t.Fatalf("alive/hp mismatch: %+v", es)
		}
	}
	if k != len(s.Entities) {
		t.Fatalf("State has %d entities, expected %d", len(s.Entities), k)
	}
	if len(s.Zones) != len(b.zones) {
		t.Fatalf("State has %d zones, battle %d", len(s.Zones), len(b.zones))
	}
	// mutating the copy leaves the battle alone
	if len(s.Entities) > 0 {
		s.Entities[0].HP = -99
		if len(s.Entities[0].Cargo) > 0 {
			s.Entities[0].Cargo[0] = 12345
		}
		s.SpellsLeft = append(s.SpellsLeft, ArmySlot{ID: "x"})
		for i := range b.spells {
			if b.spells[i].ID == "x" {
				t.Fatal("State shares the spell stock")
			}
		}
		if b.stateHash() != before {
			t.Fatal("mutating State changed the battle")
		}
	}
}

func TestStateHidesLiveHiddenTraps(t *testing.T) {
	b, err := NewBattle(hiddenTrapsSnapshot(), 1)
	if err != nil {
		t.Fatal(err)
	}
	s := b.State()
	if s.Phase != PhaseDeploy || s.Turn != 0 {
		t.Fatalf("fresh battle: %+v", s)
	}
	for _, e := range s.Entities {
		if e.ContentID == "d_mine" || e.ContentID == "d_spring_trap" {
			t.Fatalf("hidden trap leaked into State: %+v", e)
		}
	}
	if len(s.Entities) != 2 || s.Entities[0].ID != 2 { // the mine took id 1; its id is skipped, not renumbered
		t.Fatalf("entities %+v", s.Entities)
	}
	// a trap that fired is visible (the client has seen the event)
	for _, c := range trapWalk(5) {
		b.Apply(c)
	}
	var mine *EntityState
	s = b.State()
	for i := range s.Entities {
		if s.Entities[i].ContentID == "d_mine" {
			mine = &s.Entities[i]
		}
	}
	if mine == nil || mine.Alive {
		t.Fatalf("the consumed mine must show up dead: %+v", mine)
	}
	// a revealed trap is visible from the start
	snap := hiddenTrapsSnapshot()
	snap.Buildings[len(snap.Buildings)-2].Revealed = true // the mine
	b2, _ := NewBattle(snap, 1)
	seen := false
	for _, e := range b2.State().Entities {
		seen = seen || e.ContentID == "d_mine"
	}
	if !seen {
		t.Fatal("a revealed mine must be in State")
	}
	checkState(t, b)
	checkState(t, b2)
}

func TestStateFollowsTheBattle(t *testing.T) {
	b, _ := NewBattle(f01Snapshot(), 1)
	b.Apply(dep("u_rifleman", 1, 10))
	s := b.State()
	r := s.Entities[len(s.Entities)-1]
	if r.ContentID != "u_rifleman" || r.Owner != 1 || r.Pos != cell(1, 10) || r.HP != 50 || r.APLeft != 0 || r.AP != 2 {
		t.Fatalf("deployed rifleman: %+v", r)
	}
	b.Apply(Command{Op: OpStart})
	b.Apply(mv(1, 3, 2, 10, 3, 10))
	s = b.State()
	r = s.Entities[len(s.Entities)-1]
	if s.Phase != PhasePlayer || s.Turn != 1 || r.Pos != cell(3, 10) || r.APLeft != 1 {
		t.Fatalf("after Move: phase %d turn %d %+v", s.Phase, s.Turn, r)
	}
	b.Apply(end(1))
	s = b.State()
	if s.Turn != 2 || s.Entities[len(s.Entities)-1].APLeft != 2 {
		t.Fatalf("after EndTurn: %+v", s)
	}
	b.Apply(Command{Turn: 2, Op: OpRetreat})
	if s = b.State(); s.Phase != PhaseOver || s.EndReason != EndRetreat {
		t.Fatalf("after Retreat: %+v", s)
	}
	checkState(t, b)
}

func TestStateShowsZonesAndEffects(t *testing.T) {
	s := f01Snapshot()
	s.Buildings = []PlacedBuilding{pb("d_mg_nest", 6, 10)}
	s.Buildings = append(s.Buildings, pb("b_hq", 30, 30))
	s.Army = []ArmySlot{{"u_rifleman", 1, 1}, {"u_saboteur", 1, 1}}
	s.Spells = []ArmySlot{{"c_smoke", 1, 1}, {"c_adrenaline", 1, 1}}
	b, _ := NewBattle(s, 1)
	b.Apply(dep("u_rifleman", 1, 10))
	b.Apply(Command{Op: OpStart})
	b.Apply(cast(1, "c_smoke", 3, 10))
	b.Apply(cast(1, "c_adrenaline", 3, 10))
	st := b.State()
	if len(st.Zones) != 2 || st.Zones[0].Kind != ZoneSmoke || st.Zones[0].Last != 2 || st.Zones[1].Kind != ZoneAdrenaline ||
		st.Zones[1].MovePermille != 500 || st.SpellsThisTurn != 2 || len(st.SpellsLeft) != 2 || st.SpellsLeft[0].Count != 0 {
		t.Fatalf("zones %+v left %+v turn casts %d", st.Zones, st.SpellsLeft, st.SpellsThisTurn)
	}
	checkState(t, b)
}
