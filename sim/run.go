package sim

import "errors"

// ErrNotImplemented is returned for rules that are not implemented yet. Nothing returns it since SIM-5 (Ability and Cast
// are in); the variable stays for source compatibility. A command that returns it changes nothing.
var ErrNotImplemented = errors.New("sim: not implemented")

// Run replays a full command log against the snapshot and returns the result.
// Same arguments => identical Result on every platform. A log that ends without finishing the battle is
// closed as EndLogEnd (the server refuses such client results).
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
	if b.ph != phaseOver {
		b.finish(EndLogEnd)
	}
	return b.Result(), nil
}
