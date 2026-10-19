package sim

import (
	"errors"
	"testing"
)

// Compile-time shape of the 0.2 API.
var (
	_ func(Snapshot, []Command, uint64) (Result, error) = Run
	_ func(Snapshot, uint64) (*Battle, error)           = NewBattle
	_ func(*Battle, Command) ([]Event, error)           = (*Battle).Apply
	_ func(*Battle) Result                              = (*Battle).Result
)

func TestUnimplementedOpsReturnErrNotImplemented(t *testing.T) {
	for _, op := range []Op{OpAttack, OpAbility, OpCast} {
		b, _ := NewBattle(f01Snapshot(), 1)
		b.Apply(dep("u_rifleman", 1, 10))
		b.Apply(Command{Op: OpStart})
		before := b.Result().StateHash
		_, err := b.Apply(Command{Turn: 1, Op: op, Unit: 3, Target: 1})
		if !errors.Is(err, ErrNotImplemented) {
			t.Fatalf("op %d: want ErrNotImplemented, got %v", op, err)
		}
		if b.Result().StateHash != before || b.nCmd != 2 {
			t.Fatalf("op %d changed the state or consumed a log index", op)
		}
	}
}

func TestNoRulesIsBadSnapshot(t *testing.T) {
	if _, err := Run(Snapshot{}, []Command{{Op: OpStart}}, 1); !errors.Is(err, ErrBadSnapshot) {
		t.Fatalf("want ErrBadSnapshot, got %v", err)
	}
}

func TestEngineVersionBumped(t *testing.T) {
	if EngineVersion == "" || EngineVersion == "0.1.0" || EngineVersion == "0.2.0" {
		t.Fatalf("EngineVersion must be bumped for implemented rules, got %q", EngineVersion)
	}
}

func TestCommandShape(t *testing.T) {
	c := Command{Turn: 1, Op: OpMove, Unit: 3, Path: []Cell{{X: 2, Y: 10}, {X: 3, Y: 10}}}
	if c.Path[1] != (Cell{X: 3, Y: 10}) || c.Op != OpMove {
		t.Fatal("command fields")
	}
}
