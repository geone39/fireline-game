package sim

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// Meta tests of the golden set: the plan is documented, the set exercises every refusal code, the replays do not
// depend on the order of Snapshot.Buildings, the recorded engine version is the current one.

func loadGoldenFiles(t *testing.T) []goldenFile {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(goldenDir(), "G*.json"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no golden replays: %v", err)
	}
	var out []goldenFile
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var g goldenFile
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if want := strings.TrimSuffix(filepath.Base(f), ".json"); g.Name != want {
			t.Fatalf("%s holds the replay %q", f, g.Name)
		}
		out = append(out, g)
	}
	return out
}

// Every golden is in the plan table of testdata/replays/README.md (plan -> file -> what it checks) and vice versa.
func TestGoldenPlanIsDocumented(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(goldenDir(), "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	row := regexp.MustCompile(`(?m)^\| (G\d\d) \| (.+?) \| (.+?) \|\s*$`)
	doc := map[string]bool{}
	for _, m := range row.FindAllStringSubmatch(string(raw), -1) {
		if doc[m[1]] {
			t.Errorf("README lists %s twice", m[1])
		}
		doc[m[1]] = true
		if _, err := os.Stat(filepath.Join(goldenDir(), m[1]+".json")); err != nil {
			t.Errorf("README row %s has no file", m[1])
		}
	}
	goldens := loadGoldenFiles(t)
	for _, g := range goldens {
		if !doc[g.Name] {
			t.Errorf("%s is not documented in testdata/replays/README.md", g.Name)
		}
		if strings.TrimSpace(g.About) == "" {
			t.Errorf("%s has no 'about'", g.Name)
		}
		delete(doc, g.Name)
	}
	if len(doc) != 0 {
		t.Errorf("README rows without a replay: %v", doc)
	}
	if len(goldens) < 26 { // modules/sim/AGENT.md: G01..G26
		t.Errorf("only %d replays, the DoD wants G01..G26", len(goldens))
	}
}

// The golden set triggers every RejectCode at least once (the plan item "all rejections, state unchanged").
func TestGoldenCoversEveryRejectCode(t *testing.T) {
	seen := map[RejectCode]bool{}
	for _, g := range loadGoldenFiles(t) {
		for _, r := range g.Expect.Rejected {
			seen[r.Code] = true
		}
	}
	for c := RejectBadPhase; c <= RejectBattleOver; c++ {
		if !seen[c] {
			t.Errorf("no golden replay triggers reject code %d", c)
		}
	}
}

// Spec 11.8: the engine normalises Snapshot.Buildings, so every golden gives the same Result for a reversed and a
// rotated building list.
func TestGoldenShuffleInvariant(t *testing.T) {
	for _, g := range loadGoldenFiles(t) {
		cmds := fromGolden(t, g.Commands)
		for name, perm := range map[string]func([]PlacedBuilding) []PlacedBuilding{
			"reversed": func(in []PlacedBuilding) []PlacedBuilding {
				out := make([]PlacedBuilding, len(in))
				for i := range in {
					out[len(in)-1-i] = in[i]
				}
				return out
			},
			"rotated": func(in []PlacedBuilding) []PlacedBuilding {
				if len(in) == 0 {
					return in
				}
				k := len(in) / 3
				return append(append([]PlacedBuilding(nil), in[k:]...), in[:k]...)
			},
		} {
			s := g.Snapshot
			s.Buildings = perm(g.Snapshot.Buildings)
			got, err := Run(s, cmds, g.Seed)
			if err != nil {
				t.Fatalf("%s %s: %v", g.Name, name, err)
			}
			if !reflect.DeepEqual(got, g.Expect) {
				t.Errorf("%s: the %s building order changes the result", g.Name, name)
			}
		}
	}
}

// OQ-7.8: the server recomputes with the full snapshot, the client with the view without hidden traps; the
// divergence is expected and the golden pair G31/G32 documents it.
func TestGoldenHiddenTrapsDiverge(t *testing.T) {
	byName := map[string]goldenFile{}
	for _, g := range loadGoldenFiles(t) {
		byName[g.Name] = g
	}
	full, view := byName["G31"], byName["G32"]
	if full.Expect.StateHash == view.Expect.StateHash || reflect.DeepEqual(full.Expect, view.Expect) {
		t.Fatal("the client view must not reproduce the server result")
	}
	if len(view.Snapshot.Buildings) != len(full.Snapshot.Buildings)-2 {
		t.Fatalf("the client view has %d buildings, the server %d", len(view.Snapshot.Buildings), len(full.Snapshot.Buildings))
	}
}

// Policy: a replay of another engine version fails the build until it is regenerated (TestGolden); and the update
// helper refuses to rewrite a replay of the current version with a different Result.
func TestGoldenUpdatePolicy(t *testing.T) {
	g := loadGoldenFiles(t)[0]
	if g.EngineVersion != EngineVersion {
		t.Fatalf("%s was recorded by %s, the engine is %s", g.Name, g.EngineVersion, EngineVersion)
	}
	file := filepath.Join(goldenDir(), g.Name+".json")
	same := g
	if err := checkUpdatePolicy(file, same); err != nil {
		t.Fatalf("an identical rewrite must be allowed: %v", err)
	}
	changed := g
	changed.StateHashHex = "0000000000000000"
	if err := checkUpdatePolicy(file, changed); err == nil {
		t.Fatal("a different StateHash under the same engine version must be refused")
	}
	changed = g
	changed.Expect.Destruction++
	if err := checkUpdatePolicy(file, changed); err == nil {
		t.Fatal("a different Result under the same engine version must be refused")
	}
	if err := checkUpdatePolicy(filepath.Join(goldenDir(), "G99.json"), changed); err != nil {
		t.Fatalf("a new replay is always allowed: %v", err)
	}
}
