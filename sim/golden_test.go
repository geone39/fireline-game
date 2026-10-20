package sim

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// Golden replays: testdata/replays/G*.json = {snapshot (with rules), commands, seed, expect}. They are
// regenerated ONLY together with an EngineVersion bump:  go test -run TestGolden -update  (separate commit).
// A mismatch without a version bump is a rules change in disguise and fails the build.

var update = flag.Bool("update", false, "regenerate golden replays (only with an EngineVersion bump)")
var only = flag.String("only", "", "with -update: regenerate only the scenarios whose name starts with this prefix")

var opNames = []string{"deploy", "load", "start", "move", "attack", "ability", "cast", "end_turn", "retreat"}

type goldenCmd struct {
	Turn   int    `json:"turn"`
	Op     string `json:"op"`
	Unit   uint32 `json:"unit,omitempty"`
	Target uint32 `json:"target,omitempty"`
	ID     string `json:"id,omitempty"`
	Level  int    `json:"level,omitempty"`
	To     *Cell  `json:"to,omitempty"`
	Path   []Cell `json:"path,omitempty"`
}

type goldenFile struct {
	Name          string      `json:"name"`
	About         string      `json:"about"`
	EngineVersion string      `json:"engine_version"`
	Seed          uint64      `json:"seed"`
	Snapshot      Snapshot    `json:"snapshot"`
	Commands      []goldenCmd `json:"commands"`
	Expect        Result      `json:"expect"`
	StateHashHex  string      `json:"state_hash_hex"`
}

func toGolden(cmds []Command) []goldenCmd {
	out := make([]goldenCmd, len(cmds))
	for i, c := range cmds {
		g := goldenCmd{Turn: c.Turn, Op: opNames[c.Op], Unit: uint32(c.Unit), Target: uint32(c.Target), ID: c.ID, Level: c.Level, Path: c.Path}
		if c.To != (Cell{}) {
			to := c.To
			g.To = &to
		}
		out[i] = g
	}
	return out
}

func fromGolden(t *testing.T, gs []goldenCmd) []Command {
	out := make([]Command, len(gs))
	for i, g := range gs {
		op := -1
		for k, n := range opNames {
			if n == g.Op {
				op = k
			}
		}
		if op < 0 {
			t.Fatalf("unknown op %q", g.Op)
		}
		c := Command{Turn: g.Turn, Op: Op(op), Unit: EntityID(g.Unit), Target: EntityID(g.Target), ID: g.ID, Level: g.Level, Path: g.Path}
		if g.To != nil {
			c.To = *g.To
		}
		out[i] = c
	}
	return out
}

type scenario struct {
	name, about string
	snap        Snapshot
	cmds        []Command
	seed        uint64
	wantRejects int // sanity check at generation time
}

func ld(u, c EntityID) Command { return Command{Op: OpLoad, Unit: u, Target: c} }

func scenarios() []scenario {
	var all []scenario

	// G01: spec 12 F01 base, a walk without shooting (Attack is SIM-2), ends with Retreat at turn 4.
	all = append(all, scenario{name: "G01", about: "F01 base on plain: deploy rifleman+brawler, walk 3 turns by explicit paths, Retreat on turn 4",
		snap: f01Snapshot(), seed: 1, cmds: []Command{
			dep("u_rifleman", 1, 10), dep("u_brawler", 1, 12), {Op: OpStart},
			mv(1, 3, 2, 10, 3, 10, 4, 10), mv(1, 4, 2, 12, 3, 12, 4, 12, 5, 12), end(1),
			mv(2, 3, 5, 10, 6, 10, 7, 10), mv(2, 4, 6, 12, 7, 12, 8, 12, 9, 12), end(2),
			mv(3, 3, 8, 10), mv(3, 4, 10, 12, 11, 12), end(3),
			{Turn: 4, Op: OpRetreat},
		}})

	// G02: terrain rules of Move: wire cost, tank trap vs INF/VEH, air over walls, implicit EndTurn.
	{
		s := f01Snapshot()
		s.Buildings = []PlacedBuilding{pb("d_wall", 3, 11), pb("d_wall", 2, 12), pb("d_wire", 3, 20), pb("d_tank_trap", 4, 25), pb("d_wall", 6, 8)}
		s.Army = []ArmySlot{{"u_brawler", 1, 2}, {"u_drone", 1, 1}, {"u_rifleman", 1, 2}, {"v_tank", 1, 1}}
		// ids by (Y,X): wall(6,8)=1 wall(3,11)=2 wall(2,12)=3 wire=4 trap=5; units from 6
		all = append(all, scenario{name: "G02", about: "Move over terrain: drone over walls, wire costs 2 for infantry, tank trap blocks vehicles only, implicit EndTurn via Turn 2",
			snap: s, seed: 2, cmds: []Command{
				dep("u_brawler", 1, 11), dep("u_drone", 1, 12), dep("u_rifleman", 1, 20), dep("u_rifleman", 1, 26), dep("v_tank", 1, 25), {Op: OpStart},
				mv(1, 7, 2, 12, 3, 11, 4, 10, 5, 9, 6, 8), // drone: 5 cells over walls, ends above (6,8)
				mv(1, 8, 2, 20, 3, 20),                    // rifleman: 1 + 2 = 3 = move_cells
				mv(1, 9, 2, 26, 3, 26, 4, 25),             // rifleman ends ON the tank trap (infantry passes)
				mv(1, 10, 2, 25, 3, 25),                   // tank, 2 cells, before the trap
				mv(1, 6, 2, 11, 2, 10, 3, 10, 4, 10),      // brawler around the staircase
				mv(2, 10, 3, 24, 4, 24),                   // tank goes around the trap; Turn 2 closes turn 1 implicitly
				mv(2, 8, 4, 20, 5, 20, 6, 20),             // rifleman, free ground
				{Turn: 2, Op: OpRetreat},
			}})
	}

	// G03: every reject code reachable in SIM-1, state unchanged by each.
	{
		s := f01Snapshot()
		s.Hero, s.HeroLevel = "h_commander", 1
		// ids: nest 1, hq 2; rifleman 3, hero 4, brawler 5
		all = append(all, scenario{name: "G03", about: "rejected commands: bad_phase, bad_turn, no_stock, out_of_zone, cell_occupied, hero_limit, unknown_entity, not_owner, invalid_target, no_ap, bad_path, path_cost, battle_over",
			snap: s, seed: 3, wantRejects: 19, cmds: []Command{
				mv(0, 3, 2, 10), // 0 bad_phase (Move before Start)
				{Turn: 1, Op: OpDeploy, ID: "u_rifleman", Level: 1, To: cell(1, 10)}, // 1 bad_turn
				dep("u_rifleman", 5, 10), // 2 out_of_zone
				dep("u_sniper", 1, 10),   // 3 no_stock
				dep("u_rifleman", 1, 10), // ok id 3
				dep("u_rifleman", 1, 10), // 5 cell_occupied
				dep("h_commander", 0, 5), // ok id 4
				dep("h_commander", 0, 6), // 7 hero_limit
				ld(3, 99),                // 8 unknown_entity
				ld(3, 1),                 // 9 not_owner
				ld(3, 3),                 // 10 invalid_target
				dep("u_brawler", 1, 12),  // ok id 5
				{Op: OpStart},            // ok
				{Turn: 1, Op: OpDeploy, ID: "u_rifleman", Level: 1, To: cell(1, 14)}, // 13 bad_phase
				mv(1, 99, 2, 10),                     // 14 unknown_entity
				mv(1, 1, 2, 10),                      // 15 not_owner
				mv(1, 3, 3, 10),                      // 16 bad_path (not adjacent)
				mv(1, 3, 2, 10, 3, 10, 4, 10, 5, 10), // 17 path_cost (move_cells 3)
				mv(1, 5, 1, 11, 1, 10),               // 18 cell_occupied
				mv(1, 3, 2, 10),                      // ok
				mv(1, 5, 2, 12, 3, 12),               // ok (brawler ap 1)
				mv(1, 5, 4, 12),                      // 21 no_ap
				mv(3, 5, 4, 12),                      // 22 bad_turn (skips turn 2)
				end(1),                               // ok
				mv(1, 5, 4, 12),                      // 24 bad_turn (back in time)
				mv(2, 5, 4, 12, 5, 12, 6, 12),        // ok
				{Turn: 2, Op: OpRetreat},             // ok
				mv(2, 5, 7, 12),                      // 27 battle_over
			}})
	}

	// G04: special deployment: paratrooper group, hero, carrier loading, capacity.
	{
		s := f01Snapshot()
		s.Hero, s.HeroLevel = "h_commander", 2
		s.Army = []ArmySlot{{"u_para", 1, 8}, {"v_apc", 1, 1}, {"u_rifleman", 1, 3}}
		// ids: nest 1, hq 2; para group 3..6, hero 7, apc 8, riflemen 9,10,11
		all = append(all, scenario{name: "G04", about: "deployment variants: paratrooper group of 4, hero level 2, APC loading up to carries=2 (third passenger rejected)",
			snap: s, seed: 4, wantRejects: 1, cmds: []Command{
				dep("u_para", 2, 20), {Op: OpDeploy, ID: "h_commander", Level: 2, To: cell(0, 5)}, dep("v_apc", 1, 30),
				dep("u_rifleman", 1, 31), dep("u_rifleman", 1, 32), dep("u_rifleman", 1, 33),
				ld(9, 8), ld(10, 8), ld(11, 8),
				{Op: OpStart},
				mv(1, 8, 2, 30, 3, 30, 4, 30), mv(1, 7, 1, 5, 2, 5), mv(1, 3, 3, 20, 4, 20), end(1),
				{Turn: 2, Op: OpRetreat},
			}})
	}

	// G05: Retreat during deployment.
	all = append(all, scenario{name: "G05", about: "Retreat in phase 0: TurnsUsed=0; later commands are battle_over",
		snap: f01Snapshot(), seed: 5, wantRejects: 1, cmds: []Command{
			dep("u_rifleman", 1, 10), {Op: OpRetreat}, dep("u_brawler", 1, 12),
		}})
	all = append(all, scenariosSim2()...)
	all = append(all, scenariosSim3()...)
	all = append(all, scenariosSim5()...)
	return append(all, scenariosSim6()...)
}

// checkUpdatePolicy is the EngineVersion policy (README of the replays): -update may create a new replay or
// regenerate one recorded by an OLDER engine version (a bump, committed on its own). A replay of the CURRENT
// version may only be rewritten with the same log, seed, Result and StateHash (the embedded fixture Rules may be
// refreshed): anything else is a rules change without a bump and is refused.
func checkUpdatePolicy(file string, fresh goldenFile) error {
	old, err := os.ReadFile(file)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var g goldenFile
	if err := json.Unmarshal(old, &g); err != nil {
		return err
	}
	// the fresh file goes through JSON like the old one, so that nil/empty slices compare equal
	raw, err := json.Marshal(fresh)
	if err != nil {
		return err
	}
	var f goldenFile
	if err := json.Unmarshal(raw, &f); err != nil {
		return err
	}
	if g.EngineVersion == EngineVersion && (g.Seed != f.Seed || !reflect.DeepEqual(g.Commands, f.Commands) ||
		!reflect.DeepEqual(g.Expect, f.Expect) || g.StateHashHex != f.StateHashHex) {
		return fmt.Errorf("the replay of engine %s would change: bump EngineVersion first (the regenerated replays go in a separate commit)", EngineVersion)
	}
	return nil
}

func goldenDir() string { return filepath.Join("testdata", "replays") }

func TestGolden(t *testing.T) {
	if *update {
		for _, sc := range scenarios() {
			if *only != "" && !strings.HasPrefix(sc.name, *only) {
				continue
			}
			res, err := Run(sc.snap, sc.cmds, sc.seed)
			if err != nil {
				t.Fatalf("%s: %v", sc.name, err)
			}
			if sc.wantRejects >= 0 && len(res.Rejected) != sc.wantRejects {
				t.Fatalf("%s: %d rejections, scenario expects %d: %+v", sc.name, len(res.Rejected), sc.wantRejects, res.Rejected)
			}
			g := goldenFile{Name: sc.name, About: sc.about, EngineVersion: EngineVersion, Seed: sc.seed, Snapshot: sc.snap,
				Commands: toGolden(sc.cmds), Expect: res, StateHashHex: fmt.Sprintf("%016x", res.StateHash)}
			data, err := json.MarshalIndent(g, "", " ")
			if err != nil {
				t.Fatal(err)
			}
			data = append(data, 10)
			file := filepath.Join(goldenDir(), sc.name+".json")
			if err := checkUpdatePolicy(file, g); err != nil {
				t.Fatalf("%s: %v", sc.name, err)
			}
			if err := os.WriteFile(file, data, 0o644); err != nil {
				t.Fatal(err)
			}
		}
	}
	files, err := filepath.Glob(filepath.Join(goldenDir(), "G*.json"))
	if err != nil || len(files) < 20 {
		t.Fatalf("golden replays missing: %v %v", files, err)
	}
	for _, f := range files {
		raw, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		var g goldenFile
		if err := json.Unmarshal(raw, &g); err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		t.Run(g.Name, func(t *testing.T) {
			if g.EngineVersion != EngineVersion {
				t.Fatalf("%s was recorded by engine %s, current is %s: regenerate with -update in a separate commit", g.Name, g.EngineVersion, EngineVersion)
			}
			got, err := Run(g.Snapshot, fromGolden(t, g.Commands), g.Seed)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, g.Expect) || fmt.Sprintf("%016x", got.StateHash) != g.StateHashHex {
				t.Fatalf("rules changed without an EngineVersion bump:\n got %+v\nwant %+v", got, g.Expect)
			}
		})
	}
}
