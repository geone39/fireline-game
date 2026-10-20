package main

import (
	_ "embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/OWNER/fireline/sim"
	"github.com/geone39/fireline/server/svc/base"
	"github.com/geone39/fireline/server/svc/raid"
	"go.yaml.in/yaml/v3"
)

//go:embed assumptions.yaml
var defaultAssumptions []byte

//go:embed strategies.yaml
var defaultStrategies []byte

// Assumptions is tools/simbattle/assumptions.yaml: what the bot assumes where content/ is silent.
type Assumptions struct {
	Housing struct {
		CampL1              int `yaml:"camp_l1"`
		CampMax             int `yaml:"camp_max"`
		GarageL1            int `yaml:"garage_l1"`
		GarageMax           int `yaml:"garage_max"`
		SpellsPerSupportBay int `yaml:"spells_per_support_bay"`
	} `yaml:"housing"`
	Levels struct {
		Policy  string `yaml:"policy"`
		UnitCap int    `yaml:"unit_cap"`
	} `yaml:"levels"`
	Battle struct {
		TurnCap    int `yaml:"turn_cap"`
		StuckTurns int `yaml:"stuck_turns"`
	} `yaml:"battle"`
	Garrison struct {
		Policy string `yaml:"policy"`
		Unit   string `yaml:"unit"`
	} `yaml:"garrison"`
	Win struct {
		MinStars int `yaml:"min_stars"`
	} `yaml:"win"`
	Targets struct {
		MinPct int `yaml:"win_rate_min_pct"`
		MaxPct int `yaml:"win_rate_max_pct"`
	} `yaml:"targets"`
}

// Share is one line of a strategy composition.
type Share struct {
	Unit string `yaml:"unit" json:"unit"`
	Pct  int    `yaml:"pct" json:"pct"`
}

// SpellWant is a spell a strategy brings (at most Count, limited by the spell housing).
type SpellWant struct {
	ID    string `yaml:"id" json:"id"`
	Count int    `yaml:"count" json:"count"`
}

// Strategy is a bot composition + targeting mode (strategies.yaml).
type Strategy struct {
	ID       string      `yaml:"id" json:"id"`
	Name     string      `yaml:"name" json:"name"`
	Mode     string      `yaml:"mode" json:"mode"`
	Naval    bool        `yaml:"naval" json:"naval,omitempty"`
	Requires []string    `yaml:"requires" json:"requires,omitempty"`
	Camp     []Share     `yaml:"camp" json:"camp,omitempty"`
	Garage   []Share     `yaml:"garage" json:"garage,omitempty"`
	Riders   string      `yaml:"riders" json:"riders,omitempty"`
	Spells   []SpellWant `yaml:"spells" json:"spells,omitempty"`
	Hero     []string    `yaml:"hero" json:"hero,omitempty"`
}

// UnitMeta is what troops.yaml says about a unit beyond the sim numbers.
type UnitMeta struct {
	Housing int
	Pool    string // camp | garage
	Unlock  map[string]int
	Fav     string
}

// SpellMeta is the spell housing / unlock.
type SpellMeta struct {
	Housing  int
	UnlockHQ int
}

// ResearchNode is one research.yaml node that raises a unit / spell level cap.
type ResearchNode struct {
	Unit, Spell string
	ToLevel     int
	Lab         int
}

// RefBase is a content/reference_bases/*.yaml file.
type RefBase struct {
	File     string
	ID       string
	HQ       int
	Map      string
	Layout   base.Layout
	Count    map[string]int // type -> number of items
	MaxLevel map[string]int // type -> highest item level
}

// Coastal says whether the base is on a map with water.
func (r *RefBase) Coastal() bool { return r.Map != "plain" }

// Env is everything loaded once per run; it is read-only afterwards, so battles can share it between goroutines.
type Env struct {
	Dir        string
	C          *raid.Content
	A          Assumptions
	Strategies []Strategy
	Unit       map[string]*sim.UnitDef
	Def        map[string]*sim.BuildingDef
	Spell      map[string]*sim.SpellDef
	Meta       map[string]*UnitMeta
	SpellMeta  map[string]SpellMeta
	HeroUnlock map[string]int // hero id -> command post level
	Research   []ResearchNode
	MaxLevels  map[string]int  // building id -> levels (buildings.yaml)
	Producer   map[string]bool // resource buildings (produces: ...), the targets of raiders
	Water      map[string][]bool
	Refs       []*RefBase
}

type yamlUnlockUnit struct {
	ID      string         `yaml:"id"`
	Class   string         `yaml:"class"`
	Housing int            `yaml:"housing"`
	Unlock  map[string]int `yaml:"unlock"`
	Fav     string         `yaml:"fav"`
}

type yamlTroops struct {
	Troops   []yamlUnlockUnit `yaml:"troops"`
	Vehicles []yamlUnlockUnit `yaml:"vehicles"`
	Ships    []yamlUnlockUnit `yaml:"ships"`
	Spells   []struct {
		ID      string         `yaml:"id"`
		Housing int            `yaml:"housing"`
		Unlock  map[string]int `yaml:"unlock"`
	} `yaml:"spells"`
}

type yamlHeroes struct {
	Heroes []struct {
		ID     string         `yaml:"id"`
		Unlock map[string]int `yaml:"unlock"`
		InR1   *bool          `yaml:"in_r1"`
	} `yaml:"heroes"`
}

type yamlResearch struct {
	Nodes []struct {
		Unit    string `yaml:"unit"`
		Spell   string `yaml:"spell"`
		ToLevel int    `yaml:"to_level"`
		Lab     int    `yaml:"lab"`
	} `yaml:"nodes"`
}

type yamlGrid struct {
	Grid struct {
		W int `yaml:"w"`
		H int `yaml:"h"`
	} `yaml:"grid"`
}

type yamlRef struct {
	ID      string `yaml:"id"`
	HQLevel int    `yaml:"hq_level"`
	Map     string `yaml:"map"`
	Layout  struct {
		Items []base.Item `yaml:"items"`
	} `yaml:"layout"`
}

// FindContentDir walks up from dir until it finds content/battle.yaml.
func FindContentDir(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for d := abs; ; d = filepath.Dir(d) {
		if _, err := os.Stat(filepath.Join(d, "content", "battle.yaml")); err == nil {
			return filepath.Join(d, "content"), nil
		}
		if filepath.Dir(d) == d {
			return "", fmt.Errorf("content/battle.yaml not found above %s (use -content)", abs)
		}
	}
}

func readYAML(path string, into any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(b, into); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}

// LoadEnv reads the content and the QA assumption files. assumptionsPath / strategiesPath may be empty (embedded).
func LoadEnv(contentDir, assumptionsPath, strategiesPath string) (*Env, error) {
	c, err := raid.LoadContent(contentDir)
	if err != nil {
		return nil, err
	}
	e := &Env{Dir: contentDir, C: c, Unit: map[string]*sim.UnitDef{}, Def: map[string]*sim.BuildingDef{},
		Spell: map[string]*sim.SpellDef{}, Meta: map[string]*UnitMeta{}, SpellMeta: map[string]SpellMeta{},
		HeroUnlock: map[string]int{}, MaxLevels: map[string]int{}, Producer: map[string]bool{}, Water: map[string][]bool{}}

	ab, sb := defaultAssumptions, defaultStrategies
	if assumptionsPath != "" {
		if ab, err = os.ReadFile(assumptionsPath); err != nil {
			return nil, err
		}
	}
	if strategiesPath != "" {
		if sb, err = os.ReadFile(strategiesPath); err != nil {
			return nil, err
		}
	}
	if err := yaml.Unmarshal(ab, &e.A); err != nil {
		return nil, fmt.Errorf("assumptions: %w", err)
	}
	var sf struct {
		Strategies []Strategy `yaml:"strategies"`
	}
	if err := yaml.Unmarshal(sb, &sf); err != nil {
		return nil, fmt.Errorf("strategies: %w", err)
	}
	e.Strategies = sf.Strategies

	r := &e.C.Rules
	for i := range r.Units {
		e.Unit[r.Units[i].ID] = &r.Units[i]
	}
	for i := range r.Buildings {
		e.Def[r.Buildings[i].ID] = &r.Buildings[i]
	}
	for i := range r.Spells {
		e.Spell[r.Spells[i].ID] = &r.Spells[i]
	}

	var tf yamlTroops
	if err := readYAML(filepath.Join(contentDir, "troops.yaml"), &tf); err != nil {
		return nil, err
	}
	add := func(list []yamlUnlockUnit, pool string) {
		for _, u := range list {
			e.Meta[u.ID] = &UnitMeta{Housing: u.Housing, Pool: pool, Unlock: u.Unlock, Fav: u.Fav}
		}
	}
	add(tf.Troops, "camp")
	add(tf.Vehicles, "garage")
	add(tf.Ships, "garage")
	for _, s := range tf.Spells {
		e.SpellMeta[s.ID] = SpellMeta{Housing: s.Housing, UnlockHQ: s.Unlock["hq"]}
	}
	var hf yamlHeroes
	if err := readYAML(filepath.Join(contentDir, "heroes.yaml"), &hf); err != nil {
		return nil, err
	}
	for _, h := range hf.Heroes {
		if h.InR1 != nil && !*h.InR1 {
			continue
		}
		e.HeroUnlock[h.ID] = h.Unlock["command"]
	}
	var bf map[string]yaml.Node
	if err := readYAML(filepath.Join(contentDir, "buildings.yaml"), &bf); err != nil {
		return nil, err
	}
	for _, n := range bf {
		if n.Kind != yaml.SequenceNode {
			continue
		}
		var list []struct {
			ID       string `yaml:"id"`
			Levels   int    `yaml:"levels"`
			Produces string `yaml:"produces"`
		}
		if err := n.Decode(&list); err != nil {
			return nil, fmt.Errorf("buildings.yaml: %w", err)
		}
		for _, b := range list {
			e.MaxLevels[b.ID] = b.Levels
			e.Producer[b.ID] = b.Produces != ""
		}
	}
	var rf yamlResearch
	if err := readYAML(filepath.Join(contentDir, "research.yaml"), &rf); err != nil {
		return nil, err
	}
	for _, n := range rf.Nodes {
		e.Research = append(e.Research, ResearchNode{Unit: n.Unit, Spell: n.Spell, ToLevel: n.ToLevel, Lab: n.Lab})
	}
	var gf yamlGrid
	if err := readYAML(filepath.Join(contentDir, "base_grid.yaml"), &gf); err != nil {
		return nil, err
	}
	for _, m := range r.Maps {
		w := make([]bool, gf.Grid.W*gf.Grid.H)
		for _, rc := range m.Water {
			for y := rc.Y; y < rc.Y+rc.H; y++ {
				for x := rc.X; x < rc.X+rc.W; x++ {
					if x >= 0 && y >= 0 && x < gf.Grid.W && y < gf.Grid.H {
						w[y*gf.Grid.W+x] = true
					}
				}
			}
		}
		e.Water[m.ID] = w
	}
	if e.Refs, err = loadRefs(filepath.Join(contentDir, "reference_bases")); err != nil {
		return nil, err
	}
	return e, nil
}

func loadRefs(dir string) ([]*RefBase, error) {
	files, err := filepath.Glob(filepath.Join(dir, "*.yaml"))
	if err != nil {
		return nil, err
	}
	sort.Strings(files)
	var out []*RefBase
	for _, f := range files {
		var y yamlRef
		if err := readYAML(f, &y); err != nil {
			return nil, err
		}
		rb := &RefBase{File: filepath.Base(f), ID: y.ID, HQ: y.HQLevel, Map: y.Map, Layout: base.Layout{Items: y.Layout.Items},
			Count: map[string]int{}, MaxLevel: map[string]int{}}
		for _, it := range y.Layout.Items {
			rb.Count[it.Type]++
			rb.MaxLevel[it.Type] = max(rb.MaxLevel[it.Type], it.Level)
		}
		out = append(out, rb)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("no reference bases in %s", dir)
	}
	// order: HQ ascending, plain before coastal
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].HQ != out[j].HQ {
			return out[i].HQ < out[j].HQ
		}
		return (out[i].Map != "plain") && (out[j].Map == "plain")
	})
	return out, nil
}

// short is the reference base name without the "ref_" prefix (hq05, hq05_coastal).
func (r *RefBase) Short() string { return strings.TrimPrefix(r.ID, "ref_") }

// SelectRefs filters the bases by a comma list of short names ("hq05,hq10_coastal") or "all".
func (e *Env) SelectRefs(spec string) ([]*RefBase, error) {
	if spec == "" || spec == "all" {
		return e.Refs, nil
	}
	var out []*RefBase
	for _, name := range strings.Split(spec, ",") {
		name = strings.TrimSpace(name)
		found := false
		for _, r := range e.Refs {
			if r.Short() == name || r.ID == name {
				out = append(out, r)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown reference base %q", name)
		}
	}
	return out, nil
}

// SelectStrategies filters the strategies by a comma list of ids or "all".
func (e *Env) SelectStrategies(spec string) ([]Strategy, error) {
	if spec == "" || spec == "all" {
		return e.Strategies, nil
	}
	var out []Strategy
	for _, id := range strings.Split(spec, ",") {
		id = strings.TrimSpace(id)
		found := false
		for _, s := range e.Strategies {
			if s.ID == id {
				out = append(out, s)
				found = true
			}
		}
		if !found {
			return nil, fmt.Errorf("unknown strategy %q", id)
		}
	}
	return out, nil
}
