package sim

import "errors"

// ErrNotImplemented is returned while the 0.2 rules are not implemented yet (SIM-1..).
var ErrNotImplemented = errors.New("sim: not implemented")

// Run replays a full command log against the snapshot and returns the result.
// Same arguments => identical Result on every platform.
func Run(snap Snapshot, cmds []Command, seed uint64) (Result, error) {
	b, err := NewBattle(snap, seed)
	if err != nil {
		return Result{}, err
	}
	for _, c := range cmds {
		if _, err := b.Apply(c); err != nil {
			return Result{}, err
		}
	}
	return b.Result(), nil
}

// Battle is the incremental form of Run for the interactive client: same code, same rules.
type Battle struct {
	snap Snapshot
	rng  *RNG
}

// NewBattle prepares a battle in phase 0 (deployment).
func NewBattle(snap Snapshot, seed uint64) (*Battle, error) {
	return nil, ErrNotImplemented
}

// Apply feeds one command and returns the animation events. A refused command is not an error:
// it is recorded in Result.Rejected and leaves the state unchanged.
func (b *Battle) Apply(cmd Command) ([]Event, error) {
	return nil, ErrNotImplemented
}

// Result returns the outcome so far (final once the battle has ended).
func (b *Battle) Result() Result {
	return Result{EngineVersion: EngineVersion}
}
