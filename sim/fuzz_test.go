package sim

import (
	"fmt"
	"reflect"
	"testing"
)

// Garbage-in tests (DoD: "invalid commands -> RejectCode, no panics"). The server feeds sim with whatever a client
// sent, so Apply must survive any Command: unknown ops, wild ids, huge or negative numbers, absurd paths.

var garbageIDs = []string{"", "heal", "jam", "unload", "hero", "c_smoke", "c_airstrike", "c_emp", "c_breach", "u_rifleman", "h_commander", "v_apc", "nope", "d_mine", "\x00", "ё"}

// garbage draws a command with arbitrary field values; about half of them are near-legal (live ids, small cells)
// so that the deeper checks of each command are reached too.
func garbage(g gen, b *Battle) Command {
	wild := func(lim int) int {
		switch g.n(8) {
		case 0:
			return -g.n(lim + 1)
		case 1:
			return lim * (1 + g.n(1000))
		case 2:
			return []int{0, 1, -1, 32767, -32768, 1 << 30, -(1 << 30)}[g.n(7)]
		}
		return g.n(lim)
	}
	c := Command{Op: Op(g.n(14)), ID: garbageIDs[g.n(len(garbageIDs))], Level: wild(5)}
	switch {
	case g.chance(60):
		c.Turn = b.turn + g.n(3)
	default:
		c.Turn = wild(40)
	}
	n := len(b.ents) + 3
	c.Unit, c.Target = EntityID(g.n(n)), EntityID(g.n(n))
	if g.chance(15) {
		c.Unit, c.Target = EntityID(uint32(wild(1<<20))), EntityID(uint32(wild(1<<20)))
	}
	cc := func() Cell {
		if g.chance(70) {
			return cell(g.n(44), g.n(44))
		}
		return Cell{X: int16(wild(60)), Y: int16(wild(60))}
	}
	c.To = cc()
	if g.chance(50) {
		for k, l := 0, g.n(12); k < l; k++ {
			c.Path = append(c.Path, cc())
		}
	}
	if g.chance(5) { // a very long path
		for k := 0; k < 5000; k++ {
			c.Path = append(c.Path, cell(k%44, (k/44)%44))
		}
	}
	return c
}

// applyNoPanic applies c and fails the test on a panic or an engine error.
func applyNoPanic(t *testing.T, b *Battle, c Command, what string) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s: panic %v on %+v", what, r, c)
		}
	}()
	if _, err := b.Apply(c); err != nil {
		t.Fatalf("%s: Apply returned %v on %+v", what, err, c)
	}
}

// 1000 random battles, every step a mix of legal-ish and garbage commands: no panic, no error, every refusal is a
// known RejectCode that did not move the StateHash, and two runs of the same log agree.
func TestFuzzGarbageCommands1000(t *testing.T) {
	codes := map[RejectCode]int{}
	for seed := uint64(1); seed <= 1000; seed++ {
		g := gen{r: NewRNG(seed * 104729), focus: seed%3 == 0}
		snap := g.snapshot()
		b, err := NewBattle(snap, seed)
		if err != nil {
			t.Fatalf("seed %d: %v", seed, err)
		}
		var log []Command
		for i := 0; i < 150 && b.ph != phaseOver; i++ {
			var c Command
			if g.chance(45) {
				c = garbage(g, b)
			} else {
				c = g.next(b)
			}
			nRej := len(b.rejected)
			stBefore := b.State()
			applyNoPanic(t, b, c, fmt.Sprintf("seed %d step %d", seed, i))
			log = append(log, c)
			if len(b.rejected) > nRej {
				r := b.rejected[len(b.rejected)-1]
				if r.Code < RejectBadPhase || r.Code > RejectBattleOver || r.Index != len(log)-1 {
					t.Fatalf("seed %d: bad rejection %+v for command %d", seed, r, len(log)-1)
				}
				codes[r.Code]++
				// a refused command changes nothing (the only exception is the implicit EndTurn of a later Turn)
				if c.Turn <= stBefore.Turn && !reflect.DeepEqual(stBefore, b.State()) {
					t.Fatalf("seed %d: rejected command %+v (code %d) changed the world", seed, c, r.Code)
				}
			}
			checkInvariants(t, b)
		}
		r1, err1 := Run(snap, log, seed)
		r2, err2 := Run(snap, log, seed)
		if err1 != nil || err2 != nil || !reflect.DeepEqual(r1, r2) {
			t.Fatalf("seed %d: runs differ or fail: %v %v", seed, err1, err2)
		}
	}
	if len(codes) < 14 {
		t.Fatalf("garbage reached only %d reject codes: %v", len(codes), codes)
	}
	t.Logf("reject codes reached: %v", codes)
}

// An empty and a one-sided army/base never panic either (degenerate snapshots that still pass validation).
func TestFuzzDegenerateBattles(t *testing.T) {
	snaps := map[string]func(*Snapshot){
		"no buildings": func(s *Snapshot) { s.Buildings = nil },
		"no army":      func(s *Snapshot) { s.Army = nil },
		"only hero":    func(s *Snapshot) { s.Army, s.Hero, s.HeroLevel = nil, "h_commander", 1 },
		"spells only":  func(s *Snapshot) { s.Army, s.Spells = nil, allSpells() },
	}
	for name, mut := range snaps {
		s := f01Snapshot()
		mut(&s)
		b, err := NewBattle(s, 1)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		g := gen{r: NewRNG(7)}
		for i := 0; i < 400 && b.ph != phaseOver; i++ {
			applyNoPanic(t, b, garbage(g, b), name)
		}
		checkState(t, b)
	}
}

// FuzzApply: native go fuzzing over a byte-encoded command log (go test -fuzz FuzzApply). The seed corpus runs as a
// normal test. Properties: no panic, no error, the result of two runs is the same.
func FuzzApply(f *testing.F) {
	f.Add([]byte{0, 1, 10, 0, 3, 2, 10, 3, 10, 7, 1})
	f.Add([]byte{255, 255, 255, 0, 0, 0, 9, 9, 9, 4, 4, 4})
	f.Add([]byte("\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b\x0c\x0d"))
	f.Fuzz(func(t *testing.T, data []byte) {
		s := f01Snapshot()
		s.Hero, s.HeroLevel = "h_cover", 1
		s.Spells = allSpells()
		log := decodeLog(data)
		run := func() Result {
			b, err := NewBattle(s, 5)
			if err != nil {
				t.Fatal(err)
			}
			for _, c := range log {
				applyNoPanic(t, b, c, "fuzz")
			}
			if b.ph != phaseOver {
				b.finish(EndLogEnd)
			}
			checkState(t, b)
			return b.Result()
		}
		if r1, r2 := run(), run(); !reflect.DeepEqual(r1, r2) {
			t.Fatalf("two runs differ: %+v %+v", r1, r2)
		}
	})
}

// decodeLog turns bytes into commands, 7 bytes each: op, turn, unit, target, id, x, y; a trailing byte repeats the path.
func decodeLog(data []byte) []Command {
	var out []Command
	for len(data) >= 7 && len(out) < 200 {
		d := data[:7]
		data = data[7:]
		c := Command{Op: Op(d[0] % 11), Turn: int(d[1] % 6), Unit: EntityID(d[2] % 24), Target: EntityID(d[3] % 24),
			ID: garbageIDs[int(d[4])%len(garbageIDs)], Level: int(d[4] % 4), To: cell(int(d[5])%48, int(d[6])%48)}
		for k := 0; k < int(d[6]%5); k++ {
			c.Path = append(c.Path, cell(int(d[5])%48+k, int(d[6])%48))
		}
		out = append(out, c)
	}
	return out
}
