// Package wire is the JSON facade over package sim used by the WASM build (sim/wasm) and testable natively.
// Every function takes and returns JSON strings and never panics: failures come back as
// {"ok":false,"error":"..."}.
//
// Wire format (see sim/wasm/api.d.ts):
//   - Snapshot and Result use the Go field names (the same JSON as the golden replays in sim/testdata/replays);
//   - Command uses lower-case keys and an op NAME ("move", "attack", ...), as in the golden files;
//   - uint64 values (seed, StateHash) travel as strings: JS numbers lose precision above 2^53.
package wire

import (
	"encoding/json"
	"fmt"
	"strconv"

	"github.com/OWNER/fireline/sim"
)

// OpNames maps sim.Op values to wire names; index = sim.Op.
var OpNames = []string{"deploy", "load", "start", "move", "attack", "ability", "cast", "end_turn", "retreat"}

// Command is the wire form of sim.Command.
type Command struct {
	Turn   int        `json:"turn"`
	Op     string     `json:"op"`
	Unit   uint32     `json:"unit,omitempty"`
	Target uint32     `json:"target,omitempty"`
	ID     string     `json:"id,omitempty"`
	Level  int        `json:"level,omitempty"`
	To     *sim.Cell  `json:"to,omitempty"`
	Path   []sim.Cell `json:"path,omitempty"`
}

// ToSim converts a wire command; an unknown op is an error.
func (c Command) ToSim() (sim.Command, error) {
	op := -1
	for i, n := range OpNames {
		if n == c.Op {
			op = i
		}
	}
	if op < 0 {
		return sim.Command{}, fmt.Errorf("unknown op %q", c.Op)
	}
	out := sim.Command{Turn: c.Turn, Op: sim.Op(op), Unit: sim.EntityID(c.Unit), Target: sim.EntityID(c.Target), ID: c.ID, Level: c.Level, Path: c.Path}
	if c.To != nil {
		out.To = *c.To
	}
	return out, nil
}

// Result is sim.Result with StateHash as a 16-digit hex string (shadows the numeric field).
type Result struct {
	sim.Result
	StateHash string
}

func wrapResult(r sim.Result) Result {
	return Result{Result: r, StateHash: fmt.Sprintf("%016x", r.StateHash)}
}

type envelope struct {
	OK     bool   `json:"ok"`
	Error  string `json:"error,omitempty"`
	Result any    `json:"result,omitempty"`
}

func reply(res any, err error) string {
	e := envelope{OK: true, Result: res}
	if err != nil {
		e = envelope{Error: err.Error()}
	}
	b, merr := json.Marshal(e)
	if merr != nil {
		b, _ = json.Marshal(envelope{Error: "marshal: " + merr.Error()})
	}
	return string(b)
}

func parseSeed(s string) (uint64, error) {
	v, err := strconv.ParseUint(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("bad seed %q: want decimal uint64 string", s)
	}
	return v, nil
}

func parseCmds(commandsJSON string) ([]sim.Command, error) {
	var wc []Command
	if err := json.Unmarshal([]byte(commandsJSON), &wc); err != nil {
		return nil, fmt.Errorf("commands: %w", err)
	}
	out := make([]sim.Command, len(wc))
	for i, c := range wc {
		sc, err := c.ToSim()
		if err != nil {
			return nil, fmt.Errorf("commands[%d]: %w", i, err)
		}
		out[i] = sc
	}
	return out, nil
}

func parseSnap(snapshotJSON string) (sim.Snapshot, error) {
	var s sim.Snapshot
	if err := json.Unmarshal([]byte(snapshotJSON), &s); err != nil {
		return s, fmt.Errorf("snapshot: %w", err)
	}
	return s, nil
}

// EngineVersion returns sim.EngineVersion.
func EngineVersion() string { return sim.EngineVersion }

// Run replays a full log (sim.Run). The reply result is Result.
func Run(snapshotJSON, commandsJSON, seed string) string {
	snap, err := parseSnap(snapshotJSON)
	if err != nil {
		return reply(nil, err)
	}
	cmds, err := parseCmds(commandsJSON)
	if err != nil {
		return reply(nil, err)
	}
	sd, err := parseSeed(seed)
	if err != nil {
		return reply(nil, err)
	}
	r, err := sim.Run(snap, cmds, sd)
	if err != nil {
		return reply(nil, err)
	}
	return reply(wrapResult(r), nil)
}

// Battles is a table of live battles addressed by integer handles (start at 1, never reused).
type Battles struct {
	list []*sim.Battle // index = handle-1; nil after Free
}

// New is sim.NewBattle; the reply result is {"handle": n}.
func (t *Battles) New(snapshotJSON, seed string) string {
	snap, err := parseSnap(snapshotJSON)
	if err != nil {
		return reply(nil, err)
	}
	sd, err := parseSeed(seed)
	if err != nil {
		return reply(nil, err)
	}
	b, err := sim.NewBattle(snap, sd)
	if err != nil {
		return reply(nil, err)
	}
	t.list = append(t.list, b)
	return reply(map[string]int{"handle": len(t.list)}, nil)
}

func (t *Battles) get(h int) (*sim.Battle, error) {
	if h < 1 || h > len(t.list) || t.list[h-1] == nil {
		return nil, fmt.Errorf("bad battle handle %d", h)
	}
	return t.list[h-1], nil
}

// Apply is Battle.Apply; the reply result is {"events": Event[]}.
func (t *Battles) Apply(h int, commandJSON string) string {
	b, err := t.get(h)
	if err != nil {
		return reply(nil, err)
	}
	var wc Command
	if err := json.Unmarshal([]byte(commandJSON), &wc); err != nil {
		return reply(nil, fmt.Errorf("command: %w", err))
	}
	c, err := wc.ToSim()
	if err != nil {
		return reply(nil, err)
	}
	ev, err := b.Apply(c)
	if err != nil {
		return reply(nil, err)
	}
	if ev == nil {
		ev = []sim.Event{}
	}
	return reply(map[string][]sim.Event{"events": ev}, nil)
}

// Result is Battle.Result.
func (t *Battles) Result(h int) string {
	b, err := t.get(h)
	if err != nil {
		return reply(nil, err)
	}
	return reply(wrapResult(b.Result()), nil)
}

// Free releases a battle.
func (t *Battles) Free(h int) string {
	if _, err := t.get(h); err != nil {
		return reply(nil, err)
	}
	t.list[h-1] = nil
	return reply(map[string]bool{"freed": true}, nil)
}

// State is Battle.State: the client view of the world at this moment (entities, hp, ap, positions, zone effects;
// no hidden traps). Reply result: sim.State with Go field names.
func (t *Battles) State(h int) string {
	b, err := t.get(h)
	if err != nil {
		return reply(nil, err)
	}
	return reply(b.State(), nil)
}
