package wire_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/OWNER/fireline/sim"
	"github.com/OWNER/fireline/sim/wasm/wire"
)

type golden struct {
	Name         string          `json:"name"`
	Seed         uint64          `json:"seed"`
	Snapshot     json.RawMessage `json:"snapshot"`
	Commands     json.RawMessage `json:"commands"`
	StateHashHex string          `json:"state_hash_hex"`
}

const replaysDir = "../../testdata/replays"

func loadGoldens(t *testing.T) []golden {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(replaysDir, "G*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden replays: %v", err)
	}
	var out []golden
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var g golden
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		out = append(out, g)
	}
	return out
}

type reply struct {
	OK     bool            `json:"ok"`
	Error  string          `json:"error"`
	Result json.RawMessage `json:"result"`
}

// nativeStepped feeds commands one by one through the Battles table (the path JS uses for prediction). It returns
// the final Result reply and the State replies before the first command and after every command.
func nativeStepped(t *testing.T, g golden) (string, []string) {
	var tab wire.Battles
	var nb struct{ Result struct{ Handle int } }
	if err := json.Unmarshal([]byte(tab.New(string(g.Snapshot), fmt.Sprint(g.Seed))), &nb); err != nil || nb.Result.Handle == 0 {
		t.Fatalf("%s: newBattle failed", g.Name)
	}
	var cmds []json.RawMessage
	if err := json.Unmarshal(g.Commands, &cmds); err != nil {
		t.Fatal(err)
	}
	states := []string{tab.State(nb.Result.Handle)}
	for i, c := range cmds {
		var r reply
		if err := json.Unmarshal([]byte(tab.Apply(nb.Result.Handle, string(c))), &r); err != nil || !r.OK {
			t.Fatalf("%s cmd %d: %v %s", g.Name, i, err, r.Error)
		}
		states = append(states, tab.State(nb.Result.Handle))
	}
	return tab.Result(nb.Result.Handle), states
}

// The wire layer must reproduce the golden StateHash natively.
func TestWireMatchesGolden(t *testing.T) {
	for _, g := range loadGoldens(t) {
		var r reply
		if err := json.Unmarshal([]byte(wire.Run(string(g.Snapshot), string(g.Commands), fmt.Sprint(g.Seed))), &r); err != nil || !r.OK {
			t.Fatalf("%s: %v %s", g.Name, err, r.Error)
		}
		var res struct{ StateHash, EngineVersion string }
		if err := json.Unmarshal(r.Result, &res); err != nil {
			t.Fatal(err)
		}
		if res.StateHash != g.StateHashHex || res.EngineVersion != sim.EngineVersion {
			t.Errorf("%s: wire hash %s (engine %s), golden %s", g.Name, res.StateHash, res.EngineVersion, g.StateHashHex)
		}
	}
}

// state(handle) is the client view: JSON with the Go field names, no hidden traps, a bad handle is an error.
func TestWireState(t *testing.T) {
	var tab wire.Battles
	var nb struct{ Result struct{ Handle int } }
	g := loadGoldens(t)
	var trap golden
	for _, x := range g {
		if x.Name == "G31" {
			trap = x
		}
	}
	if err := json.Unmarshal([]byte(tab.New(string(trap.Snapshot), fmt.Sprint(trap.Seed))), &nb); err != nil || nb.Result.Handle == 0 {
		t.Fatal("newBattle failed")
	}
	var r reply
	if err := json.Unmarshal([]byte(tab.State(nb.Result.Handle)), &r); err != nil || !r.OK {
		t.Fatalf("state: %v %s", err, r.Error)
	}
	var st sim.State
	if err := json.Unmarshal(r.Result, &st); err != nil {
		t.Fatal(err)
	}
	if st.Phase != sim.PhaseDeploy || len(st.Entities) != 2 {
		t.Fatalf("G31 at the start must show the nest and the HQ only (traps are hidden): %+v", st)
	}
	for _, e := range st.Entities {
		if e.ContentID == "d_mine" || e.ContentID == "d_spring_trap" {
			t.Fatalf("hidden trap leaked: %+v", e)
		}
	}
	if err := json.Unmarshal([]byte(tab.State(99)), &r); err != nil || r.OK || r.Error == "" {
		t.Fatalf("a bad handle must be an error: %+v", r)
	}
}

func TestWireErrors(t *testing.T) {
	var tab wire.Battles
	for name, s := range map[string]string{
		"bad snapshot": wire.Run("{", "[]", "1"),
		"bad op":       wire.Run("{}", `[{"turn":0,"op":"nope"}]`, "1"),
		"bad seed":     wire.Run("{}", "[]", "x"),
		"empty rules":  wire.Run("{}", "[]", "1"),
		"bad handle":   tab.Apply(7, `{"turn":0,"op":"start"}`),
		"free handle":  tab.Free(1),
		"state handle": tab.State(1),
	} {
		var r reply
		if err := json.Unmarshal([]byte(s), &r); err != nil || r.OK || r.Error == "" {
			t.Errorf("%s: want error envelope, got %s", name, s)
		}
	}
}

// TestWasmParity builds the WASM module, runs G*.json through node and requires byte-identical replies
// (hence identical StateHash) with the native wire layer. Skipped with -short or when go/node are missing.
func TestWasmParity(t *testing.T) {
	if testing.Short() {
		t.Skip("-short")
	}
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node not found")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found")
	}
	dist := t.TempDir()
	build := exec.Command(goBin, "build", "-trimpath", "-ldflags=-s -w", "-o", filepath.Join(dist, "sim.wasm"), "..")
	build.Env = append(os.Environ(), "GOOS=js", "GOARCH=wasm")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("wasm build: %v\n%s", err, out)
	}
	root, err := exec.Command(goBin, "env", "GOROOT").Output()
	if err != nil {
		t.Fatal(err)
	}
	execJS := ""
	for _, rel := range []string{"lib/wasm/wasm_exec.js", "misc/wasm/wasm_exec.js"} {
		p := filepath.Join(string(bytes.TrimSpace(root)), filepath.FromSlash(rel))
		if _, err := os.Stat(p); err == nil {
			execJS = p
			break
		}
	}
	if execJS == "" {
		t.Skip("wasm_exec.js not found in GOROOT")
	}
	data, err := os.ReadFile(execJS)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "wasm_exec.js"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(node, "../run_golden.js", dist, replaysDir)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	byName := map[string]golden{}
	for _, g := range loadGoldens(t) {
		byName[g.Name] = g
	}
	seen := 0
	sc := bufio.NewScanner(stdout)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var line struct {
			Name, Engine, Run, Stepped string
			States                     []string
		}
		if err := json.Unmarshal(sc.Bytes(), &line); err != nil {
			t.Fatalf("node output: %v", err)
		}
		g := byName[line.Name]
		seen++
		if line.Engine != sim.EngineVersion {
			t.Errorf("%s: wasm engine %s, native %s", line.Name, line.Engine, sim.EngineVersion)
		}
		if want := wire.Run(string(g.Snapshot), string(g.Commands), fmt.Sprint(g.Seed)); line.Run != want {
			t.Errorf("%s: run differs\n wasm   %s\n native %s", line.Name, line.Run, want)
		}
		var r reply
		_ = json.Unmarshal([]byte(line.Run), &r)
		var res struct{ StateHash string }
		_ = json.Unmarshal(r.Result, &res)
		if res.StateHash != g.StateHashHex {
			t.Errorf("%s: wasm StateHash %s, golden %s", line.Name, res.StateHash, g.StateHashHex)
		}
		want, wantStates := nativeStepped(t, g)
		if line.Stepped != want {
			t.Errorf("%s: newBattle/apply result differs\n wasm   %s\n native %s", line.Name, line.Stepped, want)
		}
		if len(line.States) != len(wantStates) {
			t.Errorf("%s: wasm returned %d states, native %d", line.Name, len(line.States), len(wantStates))
		}
		for i := range wantStates {
			h := fnv.New32a()
			h.Write([]byte(wantStates[i]))
			if want := fmt.Sprintf("%08x", h.Sum32()); i < len(line.States) && line.States[i] != want {
				t.Errorf("%s: state() after %d commands differs (wasm hash %s, native %s)", line.Name, i, line.States[i], want)
				break
			}
		}
	}
	if err := cmd.Wait(); err != nil {
		t.Fatalf("node: %v\n%s", err, stderr.String())
	}
	if seen != len(byName) {
		t.Fatalf("node processed %d replays, expected %d", seen, len(byName))
	}
}
