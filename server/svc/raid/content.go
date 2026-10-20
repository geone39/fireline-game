// Package raid is the server side of a raid (docs/03-api.md raids/start, raids/finish): it turns content/*.yaml into
// sim.Rules, builds the battle Snapshot (full for the server, without hidden traps for the client) and re-plays a
// client's command log on the authoritative sim (Verify). RAID-0 has no HTTP and no DB: /v1/raids/* is stage M5.
//
// All game numbers come from content/*.yaml (AGENTS.md rule 3); this file only converts them to the integer
// "per turn" form of docs/design/battle-sim.md §3.2.
package raid

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OWNER/fireline/sim"
	"go.yaml.in/yaml/v3"
)

// Content is everything the raid service takes from content/*.yaml.
type Content struct {
	Rules sim.Rules
	// Version identifies the exact content the rules were built from (hash of the files read). It goes into
	// Snapshot.ContentVersion and the raid record, so a replay is always played on the same numbers.
	Version string
	// TimeLimitS is battle.time_limit_s: the real-time limit of a battle (ticket expires_at). The sim never sees it.
	TimeLimitS int
}

// LoadRules reads content/*.yaml from dir and returns the sim rules in the units of the core.
func LoadRules(dir string) (sim.Rules, error) {
	c, err := LoadContent(dir)
	if err != nil {
		return sim.Rules{}, err
	}
	return c.Rules, nil
}

// LoadContent is LoadRules plus the version hash and the server-only battle keys.
func LoadContent(dir string) (*Content, error) {
	l := &loader{dir: dir, h: sha256.New()}
	var (
		bf battleFile
		gf gridFile
		tf troopsFile
		df defencesFile
		bu buildingsFile
		hf heroesFile
	)
	l.read("battle.yaml", &bf)
	l.read("base_grid.yaml", &gf)
	l.read("troops.yaml", &tf)
	l.read("defences.yaml", &df)
	l.read("buildings.yaml", &bu)
	l.read("heroes.yaml", &hf)
	if l.err != nil {
		return nil, l.err
	}
	var r sim.Rules
	l.battle(&r, bf)
	l.grid(&r, gf)
	l.troops(&r, tf)
	l.heroes(&r, hf)
	l.spells(&r, tf)
	l.defences(&r, df)
	l.buildings(&r, bu)
	if err := errors.Join(l.errs...); err != nil {
		return nil, fmt.Errorf("raid: content %s: %w", dir, err)
	}
	return &Content{Rules: r, Version: hex.EncodeToString(l.h.Sum(nil))[:16], TimeLimitS: *bf.Battle.TimeLimitS}, nil
}

// ---- yaml shapes (only the keys the sim needs) ----

type battleFile struct {
	Battle struct {
		TimeLimitS       *int `yaml:"time_limit_s"`
		TurnS            *int `yaml:"turn_s"`
		MaxTurns         *int `yaml:"max_turns"`
		SplashFalloffPct *int `yaml:"splash_falloff_pct"`
		SpellsPerTurn    *int `yaml:"spells_per_turn"`
		ParadropEdge     *int `yaml:"paradrop_edge_cells"`
	} `yaml:"battle"`
}

type gridFile struct {
	Grid struct {
		W *int `yaml:"w"`
		H *int `yaml:"h"`
	} `yaml:"grid"`
	BuildZone *struct {
		X int `yaml:"x"`
		Y int `yaml:"y"`
		W int `yaml:"w"`
		H int `yaml:"h"`
	} `yaml:"build_zone"`
	Maps map[string]struct {
		Water [][]int `yaml:"water"`
	} `yaml:"maps"`
}

type growthY struct {
	HP  *int `yaml:"hp_permille"`
	Dps *int `yaml:"dps_permille"`
}

type unitY struct {
	ID         string         `yaml:"id"`
	Class      string         `yaml:"class"`
	HP         *int           `yaml:"hp"`
	Dps        float64        `yaml:"dps"`
	AttackDmg  *int           `yaml:"attack_dmg"`
	Burst      int            `yaml:"burst"`
	WallMult   int            `yaml:"wall_mult"`
	Range      float64        `yaml:"range"`
	Speed      float64        `yaml:"speed"`
	AP         *int           `yaml:"ap"`
	MoveCells  *int           `yaml:"move_cells"`
	RangeCells *int           `yaml:"range_cells"`
	MinRange   float64        `yaml:"min_range"`
	MinCells   *int           `yaml:"min_range_cells"`
	SplashR    float64        `yaml:"splash_r"`
	SplashC    *int           `yaml:"splash_cells"`
	Carries    int            `yaml:"carries"`
	Group      int            `yaml:"group"`
	Paradrop   bool           `yaml:"paradrop"`
	HealPS     float64        `yaml:"heal_ps"`
	HealVs     []string       `yaml:"heal_vs"`
	JamS       float64        `yaml:"jam_s"`
	VsMult     map[string]int `yaml:"vs_mult"`
	AuraCells  int            `yaml:"aura_cells"`
	AuraDmg    int            `yaml:"aura_dmg_permille"`
	AuraMove   int            `yaml:"aura_move_permille"`
	InR1       *bool          `yaml:"in_r1"`
	Ability    *struct {
		ID       string  `yaml:"id"`
		Cooldown float64 `yaml:"cooldown_s"`
		Effect   struct {
			Kind     string  `yaml:"kind"`
			Radius   float64 `yaml:"radius"`
			RadiusC  *int    `yaml:"radius_cells"`
			Duration float64 `yaml:"duration_s"`
			DmgTotal int     `yaml:"dmg_total"`
			Unit     string  `yaml:"unit"`
			Bonus    int     `yaml:"bonus_permille"`
			BonusVs  string  `yaml:"bonus_vs"`
		} `yaml:"effect"`
	} `yaml:"ability"`
}

type spellY struct {
	ID        string  `yaml:"id"`
	Kind      string  `yaml:"kind"`
	RadiusC   *int    `yaml:"radius_cells"`
	Duration  float64 `yaml:"duration_s"`
	DmgTotal  int     `yaml:"dmg_total"`
	HealTotal int     `yaml:"heal_total"`
	AdrMove   int     `yaml:"adrenaline_move_permille"`
	AdrDmg    int     `yaml:"adrenaline_dmg_permille"`
}

type troopsFile struct {
	Growth   growthY  `yaml:"level_growth"`
	Troops   []unitY  `yaml:"troops"`
	Vehicles []unitY  `yaml:"vehicles"`
	Ships    []unitY  `yaml:"ships"`
	Spells   []spellY `yaml:"spells"`
}

type heroesFile struct {
	Growth growthY `yaml:"level_growth"`
	Heroes []unitY `yaml:"heroes"`
	Decoys []unitY `yaml:"decoys"`
}

type defY struct {
	ID          string   `yaml:"id"`
	Size        []int    `yaml:"size"`
	HP          *int     `yaml:"hp"`
	Dps         float64  `yaml:"dps"`
	AttackDmg   *int     `yaml:"attack_dmg"`
	Range       float64  `yaml:"range"`
	RangeCells  *int     `yaml:"range_cells"`
	MinRange    float64  `yaml:"min_range"`
	MinCells    *int     `yaml:"min_range_cells"`
	SplashR     float64  `yaml:"splash_r"`
	SplashC     *int     `yaml:"splash_cells"`
	Vs          []string `yaml:"vs"`
	Blocks      any      `yaml:"blocks"`
	SlowPct     int      `yaml:"slow_pct"`
	DotDps      float64  `yaml:"dot_dps"`
	Burst       int      `yaml:"burst"`
	Trap        string   `yaml:"trap"`
	Hidden      bool     `yaml:"hidden"`
	MaxHPFlung  int      `yaml:"max_hp_flung"`
	FlingCells  int      `yaml:"fling_cells"`
	Garrison    int      `yaml:"garrison"`
	TriggerC    int      `yaml:"trigger_cells"`
	AuraR       float64  `yaml:"aura_r"`
	JamPct      int      `yaml:"jam_pct"`
	Chain       int      `yaml:"chain"`
	ChainCells  int      `yaml:"chain_cells"`
	OverheatT   int      `yaml:"overheat_fire_turns"`
	CoolT       int      `yaml:"cool_turns"`
	DestrWeight *int     `yaml:"destruction_weight"`
}

type defencesFile struct {
	Growth  growthY `yaml:"level_growth"`
	Passive []defY  `yaml:"passive_obstacles"`
	Active  []defY  `yaml:"active"`
}

type bldY struct {
	ID           string   `yaml:"id"`
	Size         []int    `yaml:"size"`
	HPL1         *int     `yaml:"hp_l1"`
	HPGrowth     *float64 `yaml:"hp_growth"`
	Stores       string   `yaml:"stores"`
	Untargetable bool     `yaml:"untargetable"`
	DestrWeight  *int     `yaml:"destruction_weight"`
}

type buildingsFile struct {
	Core      []bldY `yaml:"core"`
	Resources []bldY `yaml:"resources"`
	Military  []bldY `yaml:"military"`
	Decor     []bldY `yaml:"decor"`
	Obstacles []bldY `yaml:"obstacles"`
}

// ---- loader ----

type loader struct {
	dir  string
	h    hash.Hash // content version
	err  error     // fatal read/parse error
	errs []error   // missing / invalid keys, all collected
	t10  int       // battle.turn_s * 10
}

func (l *loader) read(name string, into any) {
	if l.err != nil {
		return
	}
	b, err := os.ReadFile(filepath.Join(l.dir, name))
	if err != nil {
		l.err = fmt.Errorf("raid: read content: %w", err)
		return
	}
	if err := yaml.Unmarshal(b, into); err != nil {
		l.err = fmt.Errorf("raid: parse %s: %w", name, err)
		return
	}
	l.h.Write([]byte(name))
	l.h.Write(b)
}

func (l *loader) fail(format string, a ...any) { l.errs = append(l.errs, fmt.Errorf(format, a...)) }

// need records an error for a missing required int key and returns its value (0 if absent).
func (l *loader) need(p *int, where, key string) int {
	if p == nil {
		l.fail("%s: missing %q", where, key)
		return 0
	}
	return *p
}

func tenths(v float64) int { return int(math.Round(v * 10)) }

// turns is turns(s) of spec 3.2: max(1, ceil(s10 / T10)).
func (l *loader) turns(s float64) int {
	if l.t10 <= 0 {
		return 1
	}
	return max(1, (tenths(s)+l.t10-1)/l.t10)
}

func (l *loader) battle(r *sim.Rules, f battleFile) {
	b := f.Battle
	l.need(b.TimeLimitS, "battle", "time_limit_s")
	turnS := l.need(b.TurnS, "battle", "turn_s")
	if b.TurnS != nil && turnS < 1 {
		l.fail("battle: turn_s must be >= 1")
	}
	l.t10 = turnS * 10
	r.MaxTurns = l.need(b.MaxTurns, "battle", "max_turns")
	r.SplashFalloffPct = l.need(b.SplashFalloffPct, "battle", "splash_falloff_pct")
	r.SpellsPerTurn = l.need(b.SpellsPerTurn, "battle", "spells_per_turn")
	r.ParadropEdgeCells = l.need(b.ParadropEdge, "battle", "paradrop_edge_cells")
}

func (l *loader) grid(r *sim.Rules, f gridFile) {
	r.GridW, r.GridH = l.need(f.Grid.W, "base_grid", "grid.w"), l.need(f.Grid.H, "base_grid", "grid.h")
	if f.BuildZone == nil {
		l.fail("base_grid: missing %q", "build_zone")
	} else {
		r.BuildZone = sim.Rect{X: f.BuildZone.X, Y: f.BuildZone.Y, W: f.BuildZone.W, H: f.BuildZone.H}
	}
	ids := make([]string, 0, len(f.Maps))
	for id := range f.Maps {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		m := sim.MapDef{ID: id}
		for _, w := range f.Maps[id].Water {
			if len(w) != 4 {
				l.fail("base_grid: map %s: water rect must be [x,y,w,h]", id)
				continue
			}
			m.Water = append(m.Water, sim.Rect{X: w[0], Y: w[1], W: w[2], H: w[3]})
		}
		r.Maps = append(r.Maps, m)
	}
	if len(r.Maps) == 0 {
		l.fail("base_grid: no maps")
	}
}

var classByName = map[string]sim.UnitClass{"INF": sim.ClassINF, "VEH": sim.ClassVEH, "AIR": sim.ClassAIR, "SEA": sim.ClassSEA}

func (l *loader) classMask(where string, names []string) uint8 {
	var m uint8
	for _, n := range names {
		c, ok := classByName[n]
		if !ok {
			l.fail("%s: unknown class %q", where, n)
			continue
		}
		m |= sim.ClassBit(c)
	}
	return m
}

var vsTags = map[string]sim.VsTag{
	"wire": sim.VsWire, "wall": sim.VsWall, "building": sim.VsBuilding, "defence": sim.VsDefence,
	"inf": sim.VsINF, "veh": sim.VsVEH, "air": sim.VsAIR, "sea": sim.VsSEA, "hq": sim.VsHQ,
}

func (l *loader) vsTag(where, s string) sim.VsTag {
	t, ok := vsTags[strings.ToLower(s)]
	if !ok {
		l.fail("%s: unknown vs tag %q", where, s)
	}
	return t
}

// unit converts a troops/vehicles/ships/heroes/decoys entry. g is the level growth of its file.
func (l *loader) unit(y unitY, g growthY, hero bool) sim.UnitDef {
	where := "unit " + y.ID
	if y.ID == "" {
		l.fail("unit without id")
	}
	cl, ok := classByName[y.Class]
	if !ok {
		l.fail("%s: unknown class %q", where, y.Class)
	}
	u := sim.UnitDef{ID: y.ID, Class: cl, Hero: hero}
	u.HP = l.need(y.HP, where, "hp")
	u.HPGrowthPermille = l.need(g.HP, "level_growth", "hp_permille")
	u.DmgGrowthPermille = l.need(g.Dps, "level_growth", "dps_permille")
	u.AP = l.need(y.AP, where, "ap")
	if y.MoveCells != nil {
		u.MoveCells = *y.MoveCells
	} else if y.Speed > 0 {
		u.MoveCells = max(1, tenths(y.Speed)*l.t10/100)
	}
	if y.RangeCells != nil {
		u.RangeCells = *y.RangeCells
	} else {
		u.RangeCells = max(1, tenths(y.Range)/10) // range 0 = melee = 1
	}
	if y.MinCells != nil {
		u.MinRangeCells = *y.MinCells
	} else {
		u.MinRangeCells = tenths(y.MinRange) / 10
	}
	if y.SplashC != nil {
		u.SplashCells = *y.SplashC
	} else {
		u.SplashCells = tenths(y.SplashR) / 10
	}
	if y.AttackDmg != nil {
		u.Dmg = *y.AttackDmg
	} else {
		u.Dmg = tenths(y.Dps) * l.t10 / 100 // floor(dps10 * 1000 * T10 / 100000), level 1
	}
	u.Burst, u.WallMult = y.Burst, y.WallMult
	if u.Burst > 0 {
		u.Dmg = 0 // a detonation replaces the shot
	}
	u.Carries, u.Group, u.Paradrop = y.Carries, y.Group, y.Paradrop
	if y.HealPS > 0 {
		u.Heal = tenths(y.HealPS) * l.t10 / 100
		if len(y.HealVs) == 0 {
			l.fail("%s: heal_ps without heal_vs", where)
		}
		u.HealVs = l.classMask(where+" heal_vs", y.HealVs)
	}
	if y.JamS > 0 {
		u.JamTurns = l.turns(y.JamS)
	}
	tags := make([]string, 0, len(y.VsMult))
	for t := range y.VsMult {
		tags = append(tags, t)
	}
	sort.Strings(tags) // reproducible Rules (the engine sums the entries)
	for _, t := range tags {
		u.VsMult = append(u.VsMult, sim.VsMult{Tag: l.vsTag(where+" vs_mult", t), Permille: y.VsMult[t]})
	}
	u.AuraCells, u.AuraDmgPermille, u.AuraMovePermille = y.AuraCells, y.AuraDmg, y.AuraMove
	return u
}

func (l *loader) troops(r *sim.Rules, f troopsFile) {
	for _, list := range [][]unitY{f.Troops, f.Vehicles, f.Ships} {
		for _, y := range list {
			r.Units = append(r.Units, l.unit(y, f.Growth, false))
		}
	}
}

func (l *loader) heroes(r *sim.Rules, f heroesFile) {
	for _, y := range f.Heroes {
		if y.InR1 != nil && !*y.InR1 {
			continue // OQ-7.6: h_warden / a_bulwark are out of R1
		}
		u := l.unit(y, f.Growth, true)
		if a := y.Ability; a != nil {
			where := "hero " + y.ID + " ability " + a.ID
			e := a.Effect
			ab := sim.AbilityDef{ID: a.ID, CooldownTurns: l.turns(a.Cooldown)}
			if e.RadiusC != nil {
				ab.RadiusCells = *e.RadiusC
			} else {
				ab.RadiusCells = tenths(e.Radius) / 10
			}
			switch e.Kind {
			case "zone_damage":
				ab.Kind, ab.Dmg, ab.DurationTurns = sim.AbilityZoneDamage, e.DmgTotal, l.turns(e.Duration)
			case "spawn_decoy":
				ab.Kind, ab.DecoyUnit = sim.AbilityDecoy, e.Unit
				if e.Duration > 0 {
					ab.DurationTurns = l.turns(e.Duration)
				}
			case "stealth":
				ab.Kind, ab.DurationTurns, ab.BonusPermille = sim.AbilityStealth, l.turns(e.Duration), e.Bonus
				if e.BonusVs != "" {
					ab.BonusVs = l.vsTag(where, e.BonusVs)
				}
			default:
				l.fail("%s: unsupported effect kind %q", where, e.Kind)
			}
			u.Ability = ab
		}
		r.Units = append(r.Units, u)
	}
	for _, y := range f.Decoys { // ASSUMPTION: decoys grow like heroes (OQ-RAID0-3)
		r.Units = append(r.Units, l.unit(y, f.Growth, false))
	}
}

var spellKinds = map[string]sim.SpellKind{
	"airstrike": sim.SpellAirstrike, "medkit": sim.SpellMedkit, "smoke": sim.SpellSmoke,
	"adrenaline": sim.SpellAdrenaline, "emp": sim.SpellEMP, "breach": sim.SpellBreach,
}

func (l *loader) spells(r *sim.Rules, f troopsFile) {
	for _, y := range f.Spells {
		where := "spell " + y.ID
		k, ok := spellKinds[y.Kind]
		if !ok {
			l.fail("%s: unknown or missing kind %q", where, y.Kind)
		}
		sp := sim.SpellDef{ID: y.ID, Kind: k, DmgTotal: y.DmgTotal, HealTotal: y.HealTotal, MovePermille: y.AdrMove, DmgPermille: y.AdrDmg}
		sp.RadiusCells = l.need(y.RadiusC, where, "radius_cells")
		if y.Duration > 0 {
			sp.DurationTurns = l.turns(y.Duration)
		}
		r.Spells = append(r.Spells, sp)
	}
}

func (l *loader) defences(r *sim.Rules, f defencesFile) {
	for _, list := range []struct {
		defs    []defY
		counted bool
	}{{f.Passive, false}, {f.Active, true}} {
		for _, y := range list.defs {
			where := "defence " + y.ID
			d := sim.BuildingDef{ID: y.ID, Kind: sim.KindDefence, Counted: list.counted}
			if !list.counted {
				d.Kind = sim.KindWall
			}
			d.W, d.H = l.size(where, y.Size)
			d.HP = l.need(y.HP, where, "hp")
			d.HPGrowthPermille = l.need(f.Growth.HP, "level_growth", "hp_permille")
			dmgGrowth := l.need(f.Growth.Dps, "level_growth", "dps_permille")
			switch b := y.Blocks.(type) {
			case nil:
				d.Block = sim.BlockAll
			case bool:
				if b {
					d.Block = sim.BlockAll
				} else {
					d.Block = sim.BlockNone
				}
			case string:
				if b != "vehicles" {
					l.fail("%s: unknown blocks %q", where, b)
				}
				d.Block = sim.BlockVehicles
			default:
				l.fail("%s: bad blocks value", where)
			}
			if y.Hidden { // a hidden trap never blocks
				d.Block = sim.BlockNone
			}
			d.SlowPct = y.SlowPct
			d.DotDmg = tenths(y.DotDps) * l.t10 / 100 // dot_dps x turn_s, whole
			if y.AttackDmg != nil {
				d.Dmg = *y.AttackDmg
			} else {
				d.Dmg = tenths(y.Dps) * l.t10 / 100
			}
			if y.RangeCells != nil {
				d.RangeCells = *y.RangeCells
			} else {
				d.RangeCells = tenths(y.Range) / 10
			}
			if y.MinCells != nil {
				d.MinRangeCells = *y.MinCells
			} else {
				d.MinRangeCells = tenths(y.MinRange) / 10
			}
			if y.SplashC != nil {
				d.SplashCells = *y.SplashC
			} else {
				d.SplashCells = tenths(y.SplashR) / 10
			}
			if d.Dmg > 0 { // only shooting defences grow; traps, walls and Burst/DotDmg stay flat (OQ-SIM13)
				d.DmgGrowthPermille = dmgGrowth
			}
			d.Vs = l.classMask(where+" vs", y.Vs)
			d.Hidden = y.Hidden
			switch y.Trap {
			case "":
			case "mine":
				d.Trap = sim.TrapMine
			case "spring":
				d.Trap = sim.TrapSpring
			default:
				l.fail("%s: unknown trap %q", where, y.Trap)
			}
			d.Burst, d.MaxHPFlung, d.FlingCells = y.Burst, y.MaxHPFlung, y.FlingCells
			d.Garrison, d.TriggerCells = y.Garrison, y.TriggerC
			d.AuraCells, d.JamPct = tenths(y.AuraR)/10, y.JamPct
			d.Chain, d.ChainCells = y.Chain, y.ChainCells
			d.OverheatFireTurns, d.CoolTurns = y.OverheatT, y.CoolT
			if d.Counted {
				d.Weight = l.need(y.DestrWeight, where, "destruction_weight")
			}
			r.Buildings = append(r.Buildings, d)
		}
	}
}

func (l *loader) size(where string, s []int) (int, int) {
	if len(s) != 2 || s[0] < 1 || s[1] < 1 {
		l.fail("%s: size must be [w,h]", where)
		return 1, 1
	}
	return s[0], s[1]
}

func (l *loader) buildings(r *sim.Rules, f buildingsFile) {
	groups := []struct {
		defs    []bldY
		counted bool
		hq      bool
	}{{f.Core, true, true}, {f.Resources, true, false}, {f.Military, true, false}, {f.Decor, false, false}, {f.Obstacles, false, false}}
	for _, g := range groups {
		for _, y := range g.defs {
			where := "building " + y.ID
			d := sim.BuildingDef{ID: y.ID, Kind: sim.KindBuilding, Block: sim.BlockAll, Counted: g.counted, HQ: g.hq,
				Storage: y.Stores != "", Untargetable: y.Untargetable}
			d.W, d.H = l.size(where, y.Size)
			d.HP = l.need(y.HPL1, where, "hp_l1")
			if y.HPGrowth == nil {
				if g.counted {
					l.fail("%s: missing %q", where, "hp_growth")
				}
			} else {
				d.HPGrowthPermille = tenths(*y.HPGrowth * 100)
			}
			if g.counted {
				d.Weight = l.need(y.DestrWeight, where, "destruction_weight")
			}
			r.Buildings = append(r.Buildings, d)
		}
	}
}
