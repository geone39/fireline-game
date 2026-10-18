package sim

// Run executes a battle to completion. SKELETON: owner `sim` implements world stepping per docs/design/battle-sim.md.
func Run(snap Snapshot, cmds []Command, seed uint64) (Result, error) {
	rng := NewRNG(seed)
	_ = rng
	return Result{}, nil
}
