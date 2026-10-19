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

func TestRunNotImplemented(t *testing.T) {
	_, err := Run(Snapshot{}, []Command{{Turn: 0, Op: OpStart}}, 1)
	if !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("want ErrNotImplemented, got %v", err)
	}
}

func TestEngineVersionBumped(t *testing.T) {
	if EngineVersion == "" || EngineVersion == "0.1.0" {
		t.Fatalf("EngineVersion must be bumped for contract 0.2, got %q", EngineVersion)
	}
}

func TestCommandShape(t *testing.T) {
	c := Command{Turn: 1, Op: OpMove, Unit: 3, Path: []Cell{{X: 2, Y: 10}, {X: 3, Y: 10}}}
	if c.Path[1] != (Cell{X: 3, Y: 10}) || c.Op != OpMove {
		t.Fatal("command fields")
	}
}
