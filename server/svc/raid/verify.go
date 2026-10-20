package raid

import (
	"fmt"

	"github.com/OWNER/fireline/sim"
)

// Divergence says why a client result was not accepted.
type Divergence string

const (
	DivNone          Divergence = ""
	DivHash          Divergence = "state_hash"     // the client's StateHash differs from the server's recompute
	DivRejected      Divergence = "rejected"       // the sim refused some commands (a client never sends those)
	DivLogEnd        Divergence = "log_end"        // the log ends without an ending (OQ-7.7); the server closes it itself
	DivEngineVersion Divergence = "engine_version" // the client played another rule set
)

// Verdict is the outcome of Verify. Result is ALWAYS the authoritative server recompute: the loot, stars and
// trophies of a raid are taken from it and never from the client (AGENTS.md rule 5).
type Verdict struct {
	Result sim.Result
	// ClientAccepted is true only when the client's claim equals the server's recompute and no command was refused.
	// Strict policy (RAID-0, ASSUMPTION OQ-RAID0-1): otherwise the client result is rejected and the server
	// recompute is what counts; Diverged is the flag for the log / anti-abuse.
	ClientAccepted bool
	Diverged       bool
	Reasons        []Divergence
}

// Claim is what the client reports in raids/finish.
type Claim struct {
	StateHash     uint64
	EngineVersion string // "" = not reported, not checked
}

// Verify replays the command log on the FULL snapshot (hidden traps included) with the ticket's seed through the
// native sim and compares the outcome with the client's claim. The error is non-nil only when the engine cannot
// play the snapshot (sim.ErrBadSnapshot) or breaks; a cheating or buggy client is a Diverged verdict, not an error.
// If the battle was ended by the real-time limit the caller appends the Retreat command before calling (OQ-SIM1).
func Verify(snap sim.Snapshot, cmds []sim.Command, seed uint64, claim Claim) (Verdict, error) {
	res, err := sim.Run(snap, cmds, seed)
	if err != nil {
		return Verdict{}, fmt.Errorf("raid: verify: %w", err)
	}
	v := Verdict{Result: res}
	if claim.StateHash != res.StateHash {
		v.Reasons = append(v.Reasons, DivHash)
	}
	if len(res.Rejected) > 0 {
		v.Reasons = append(v.Reasons, DivRejected)
	}
	if res.EndReason == sim.EndLogEnd {
		v.Reasons = append(v.Reasons, DivLogEnd)
	}
	if claim.EngineVersion != "" && claim.EngineVersion != res.EngineVersion {
		v.Reasons = append(v.Reasons, DivEngineVersion)
	}
	v.Diverged = len(v.Reasons) > 0
	v.ClientAccepted = !v.Diverged
	return v, nil
}
