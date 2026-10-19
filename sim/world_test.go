package sim

import (
	"errors"
	"testing"
)

func TestXXH64Vectors(t *testing.T) {
	for _, c := range []struct {
		in   string
		want uint64
	}{
		{"", 0xEF46DB3751D8E999},
		{"a", 0xD24EC4F1A98C6E5B},
		{"abc", 0x44BC2CF5AD770999},
		{"Nobody inspects the spammish repetition", 0xFBCEA83C8A378BF1},
	} {
		if got := xxh64([]byte(c.in)); got != c.want {
			t.Errorf("xxh64(%q)=%x want %x", c.in, got, c.want)
		}
	}
}

func TestCanonicalEntityIDs(t *testing.T) {
	b, err := NewBattle(f01Snapshot(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if b.ents[0].contentID != "d_mg_nest" || b.ents[1].contentID != "b_hq" { // spec 12: nest=1, hq=2
		t.Fatalf("ids: %s %s", b.ents[0].contentID, b.ents[1].contentID)
	}
	b.Apply(dep("u_rifleman", 1, 10))
	b.Apply(dep("u_brawler", 1, 12))
	if b.ent(3).contentID != "u_rifleman" || b.ent(4).contentID != "u_brawler" {
		t.Fatal("attacker ids follow Deploy order")
	}
}

func TestSnapshotOrderIrrelevant(t *testing.T) {
	a := f01Snapshot()
	c := f01Snapshot()
	c.Buildings[0], c.Buildings[1] = c.Buildings[1], c.Buildings[0]
	cmds := []Command{dep("u_rifleman", 1, 10), {Op: OpStart}, mv(1, 3, 2, 10)}
	ra, _ := Run(a, cmds, 7)
	rc, _ := Run(c, cmds, 7)
	if ra.StateHash != rc.StateHash {
		t.Fatal("hash depends on Snapshot.Buildings order")
	}
}

func TestBadSnapshots(t *testing.T) {
	cases := map[string]func(*Snapshot){
		"unknown building":   func(s *Snapshot) { s.Buildings = append(s.Buildings, pb("nope", 30, 30)) },
		"outside zone":       func(s *Snapshot) { s.Buildings = append(s.Buildings, pb("d_wall", 1, 30)) },
		"off zone footprint": func(s *Snapshot) { s.Buildings = append(s.Buildings, pb("b_hq", 40, 40)) },
		"overlap":            func(s *Snapshot) { s.Buildings = append(s.Buildings, pb("d_wall", 13, 11)) },
		"bad level":          func(s *Snapshot) { s.Buildings[0].Level = 0 },
		"unknown map":        func(s *Snapshot) { s.Map = "moon" },
		"unknown army":       func(s *Snapshot) { s.Army = append(s.Army, ArmySlot{ID: "zzz", Level: 1, Count: 1}) },
		"hero not hero":      func(s *Snapshot) { s.Hero, s.HeroLevel = "u_rifleman", 1 },
		"water building": func(s *Snapshot) {
			s.Map = "coastal"
			s.Buildings = append(s.Buildings, pb("d_wall", 41, 5))
		},
	}
	for name, mut := range cases {
		s := f01Snapshot()
		mut(&s)
		if _, err := NewBattle(s, 1); !errors.Is(err, ErrBadSnapshot) {
			t.Errorf("%s: want ErrBadSnapshot, got %v", name, err)
		}
	}
}

func TestScaledHP(t *testing.T) {
	if scaledHP(800, 1220, 1) != 800 || scaledHP(800, 1220, 2) != 976 || scaledHP(800, 1220, 3) != 800*1488/1000 {
		t.Fatalf("hp scaling %d %d %d", scaledHP(800, 1220, 1), scaledHP(800, 1220, 2), scaledHP(800, 1220, 3))
	}
	// M[3] = floor(1220*1220/1000) = 1488
}

func TestGarrisonIDsAssignedAtStart(t *testing.T) {
	s := f01Snapshot()
	s.Buildings = append(s.Buildings, pb("d_bunker", 30, 30))
	s.Garrison = []GarrisonUnit{{"u_guard", 1}, {"u_guard", 1}, {"u_guard", 1}}
	b, err := NewBattle(s, 1)
	if err != nil {
		t.Fatal(err)
	}
	// 3 buildings (ids 1..3) + 2 garrison units (bunker capacity 2, third dropped), then attackers
	if len(b.ents) != 5 || b.ents[3].inside == 0 || b.ents[3].onField {
		t.Fatalf("garrison entities: %+v", b.ents[3:])
	}
	b.Apply(dep("u_rifleman", 1, 10))
	if b.ents[5].id != 6 {
		t.Fatal("attacker id after garrison")
	}
}

func TestEndConditionsAndLifecycle(t *testing.T) {
	// retreat in phase 0
	r, _ := Run(f01Snapshot(), []Command{{Op: OpRetreat}}, 1)
	if r.EndReason != EndRetreat || r.TurnsUsed != 0 {
		t.Fatalf("%+v", r)
	}
	// log exhausted
	r, _ = Run(f01Snapshot(), []Command{dep("u_rifleman", 1, 10), {Op: OpStart}}, 1)
	if r.EndReason != EndLogEnd || r.TurnsUsed != 1 {
		t.Fatalf("%+v", r)
	}
	// max_turns fuse: 30 EndTurns
	cmds := []Command{dep("u_rifleman", 1, 10), {Op: OpStart}}
	for i := 1; i <= 30; i++ {
		cmds = append(cmds, end(i))
	}
	cmds = append(cmds, end(30)) // after the end
	r, _ = Run(f01Snapshot(), cmds, 1)
	if r.EndReason != EndTurnLimit || r.TurnsUsed != 30 || len(r.Rejected) != 1 || r.Rejected[0].Code != RejectBattleOver {
		t.Fatalf("%+v", r)
	}
}

func TestImplicitEndTurn(t *testing.T) {
	b, _ := NewBattle(f01Snapshot(), 1)
	b.Apply(dep("u_brawler", 1, 12))
	b.Apply(Command{Op: OpStart})
	b.Apply(mv(1, 3, 2, 12)) // AP 1 spent
	if _, err := b.Apply(mv(1, 3, 3, 12)); err != nil || lastReject(b) != RejectNoAP {
		t.Fatalf("want no_ap, got %d", lastReject(b))
	}
	// a Turn 2 command closes turn 1 implicitly and AP is refreshed
	b.Apply(mv(2, 3, 3, 12))
	if b.turn != 2 || b.ent(3).pos != cell(3, 12) || len(b.rejected) != 1 {
		t.Fatalf("turn=%d pos=%v rej=%v", b.turn, b.ent(3).pos, b.rejected)
	}
	// skipping a turn or going back is bad_turn
	b.Apply(mv(4, 3, 4, 12))
	b.Apply(mv(1, 3, 4, 12))
	if len(b.rejected) != 3 || b.rejected[1].Code != RejectBadTurn || b.rejected[2].Code != RejectBadTurn {
		t.Fatalf("%v", b.rejected)
	}
}
