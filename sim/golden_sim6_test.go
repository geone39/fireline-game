package sim

// SIM-6 golden scenarios G30..G32: the large base (HQ10 load) and hidden traps seen by the server vs by the client.
// They add files only: no rule changed, EngineVersion stays (see the update policy in TestGolden).

// hiddenTrapsSnapshot is the base of G31/G32: a mine and a spring trap on the riflemen's rows.
func hiddenTrapsSnapshot() Snapshot {
	s := f01Snapshot()
	s.Buildings = append(s.Buildings, pb("d_mine", 6, 10), pb("d_spring_trap", 8, 12))
	s.Army = []ArmySlot{{"u_rifleman", 1, 2}}
	return s
}

// clientView is the snapshot the client gets: no live hidden traps that no radar has revealed (OQ-7.8). The server
// builds it; here it only serves the tests.
func clientView(s Snapshot) Snapshot {
	out := s
	out.Buildings = nil
	for _, b := range s.Buildings {
		hidden := false
		for _, d := range s.Rules.Buildings {
			if d.ID == b.ID && d.Hidden && !b.Revealed {
				hidden = true
			}
		}
		if !hidden {
			out.Buildings = append(out.Buildings, b)
		}
	}
	return out
}

// trapWalk is the log of G31/G32; first is the EntityID of the first attacker (defender entities come first).
func trapWalk(first EntityID) []Command {
	r1, r2 := first, first+1
	return []Command{
		dep("u_rifleman", 1, 10), dep("u_rifleman", 1, 12), {Op: OpStart},
		mv(1, r1, 2, 10, 3, 10, 4, 10), mv(1, r2, 2, 12, 3, 12, 4, 12), end(1),
		mv(2, r1, 5, 10, 6, 10), mv(2, r2, 5, 12, 6, 12, 7, 12), end(2),
		mv(3, r2, 8, 12), mv(3, r1, 7, 10), end(3),
		{Turn: 4, Op: OpRetreat},
	}
}

func scenariosSim6() []scenario {
	var all []scenario

	// G30: the HQ10 load of spec 11.8 (150 buildings, 120 garrison units, 120 attackers): every bunker opens, the
	// garrison walks and shoots, the idle army is shot to pieces. The log is long on purpose: it is also the
	// benchmark load.
	{
		s := fullBaseSnapshot(nil)
		cmds := make([]Command, 0, 140)
		for i := 0; i < 120; i++ {
			c := fullBaseRimCell(i)
			cmds = append(cmds, dep("u_heavy", int(c.X), int(c.Y)))
		}
		cmds = append(cmds, Command{Op: OpStart})
		for turn := 1; turn <= 8; turn++ {
			cmds = append(cmds, end(turn))
		}
		cmds = append(cmds, Command{Turn: 9, Op: OpRetreat})
		all = append(all, scenario{name: "G30", about: "large base HQ10 (150 buildings, 12-man bunkers, 120 garrison, 120 attackers): eight idle turns of the defence phase; the Snapshot.Buildings order does not matter (TestGoldenShuffleInvariant)",
			snap: s, seed: 30, cmds: cmds, wantRejects: -1})
	}

	// G31/G32: the same walk on the full snapshot (server) and on the client view without hidden traps.
	full := hiddenTrapsSnapshot()
	all = append(all, scenario{name: "G31", about: "hidden traps, server recomputation: the mine on (6,10) hurts the first rifleman and its path stops, the spring trap on (8,12) throws the second back; compare G32",
		snap: full, seed: 31, cmds: trapWalk(5), wantRejects: 0})
	all = append(all, scenario{name: "G32", about: "the same log on the client view (no hidden traps, ids shifted by two): the walk is free, the result differs from G31 - the divergence the server resolves (OQ-7.8)",
		snap: clientView(full), seed: 31, cmds: trapWalk(3), wantRejects: 0})
	return all
}
