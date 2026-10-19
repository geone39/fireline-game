package economy

import (
	"os"
	"testing"

	"go.yaml.in/yaml/v3"
)

// The pricing file is read by svc/base (LoadPriceBook); economy cannot import base (base imports economy),
// so this test checks the file's shape here: every buildable type is priced, spending only known resources,
// with the growth knobs equal to economy.yaml. Pacing is checked by tools/econ_sim.py --check.
type pricingFile struct {
	Schema          int                                `yaml:"schema"`
	CostGrowth      float64                            `yaml:"cost_growth"`
	TimeGrowth      float64                            `yaml:"time_growth"`
	CancelRefundPct int                                `yaml:"cancel_refund_pct"`
	SpeedupGems     struct{ Factor, Exponent float64 } `yaml:"speedup_gems"`
	Buildings       map[string]struct {
		Cost       map[string]int64 `yaml:"cost"`
		TimeS      int64            `yaml:"time_s"`
		CostGrowth float64          `yaml:"cost_growth"`
		TimeGrowth float64          `yaml:"time_growth"`
		MaxLevel   int              `yaml:"max_level"`
	} `yaml:"buildings"`
}

func readYAML(t *testing.T, path string, out any) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := yaml.Unmarshal(b, out); err != nil {
		t.Fatalf("%s: %v", path, err)
	}
}

func TestBuildingCostsFile(t *testing.T) {
	const dir = "../../../content/"
	var pf pricingFile
	readYAML(t, dir+"building_costs.yaml", &pf)
	var econ struct {
		Resources map[string]any `yaml:"resources"`
		Costs     struct {
			Cash struct{ Growth float64 } `yaml:"cash"`
			Time float64                  `yaml:"time_growth_per_level"`
		} `yaml:"costs"`
	}
	readYAML(t, dir+"economy.yaml", &econ)
	var bld struct {
		Core, Resources, Military []struct {
			ID     string `yaml:"id"`
			Levels int    `yaml:"levels"`
		}
	}
	readYAML(t, dir+"buildings.yaml", &bld)
	var def struct {
		Passive []struct{ ID string } `yaml:"passive_obstacles"`
		Active  []struct{ ID string } `yaml:"active"`
	}
	readYAML(t, dir+"defences.yaml", &def)

	if pf.Schema != 1 || pf.SpeedupGems.Factor <= 0 || pf.SpeedupGems.Exponent <= 0 || pf.CancelRefundPct < 0 || pf.CancelRefundPct > 100 {
		t.Fatalf("bad header: %+v", pf)
	}
	if pf.CostGrowth != econ.Costs.Cash.Growth || pf.TimeGrowth != econ.Costs.Time {
		t.Errorf("growth %v/%v != economy.yaml %v/%v", pf.CostGrowth, pf.TimeGrowth, econ.Costs.Cash.Growth, econ.Costs.Time)
	}
	want := map[string]int{} // id -> levels from buildings.yaml (0 = needs max_level here)
	for _, g := range [][]struct {
		ID     string `yaml:"id"`
		Levels int    `yaml:"levels"`
	}{bld.Core, bld.Resources, bld.Military} {
		for _, b := range g {
			want[b.ID] = b.Levels
		}
	}
	for _, d := range append(def.Passive, def.Active...) {
		want[d.ID] = 0
	}
	for id, levels := range want {
		e, ok := pf.Buildings[id]
		if !ok {
			t.Errorf("%s: no price", id)
			continue
		}
		if e.TimeS <= 0 || len(e.Cost) == 0 || (e.CostGrowth != 0 && e.CostGrowth < 1) || (e.TimeGrowth != 0 && e.TimeGrowth < 1) {
			t.Errorf("%s: bad entry %+v", id, e)
		}
		if levels == 0 && e.MaxLevel < 1 {
			t.Errorf("%s: no levels in content and no max_level", id)
		}
		for r, a := range e.Cost {
			if _, ok := econ.Resources[r]; !ok || a < 0 {
				t.Errorf("%s: bad cost %s=%d", id, r, a)
			}
		}
	}
	for id := range pf.Buildings {
		if _, ok := want[id]; !ok {
			t.Errorf("%s: priced but not in content", id)
		}
	}
}
