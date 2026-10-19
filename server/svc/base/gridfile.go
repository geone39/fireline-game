package base

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"go.yaml.in/yaml/v3"
)

type gridFile struct {
	Grid struct {
		W int `yaml:"w"`
		H int `yaml:"h"`
	} `yaml:"grid"`
	DefaultMap string `yaml:"default_map"`
	Maps       map[string]struct {
		Water [][]int `yaml:"water"` // rects [x, y, w, h]
	} `yaml:"maps"`
}

// ParseGrid builds a Grid from the bytes of content/base_grid.yaml for the named
// map; an empty name selects `default_map`. Water rects are [x, y, w, h] and are
// clipped to the grid.
func ParseGrid(data []byte, mapName string) (Grid, error) {
	var f gridFile
	if err := yaml.Unmarshal(data, &f); err != nil {
		return Grid{}, fmt.Errorf("base: base_grid.yaml: %w", err)
	}
	if f.Grid.W <= 0 || f.Grid.H <= 0 {
		return Grid{}, fmt.Errorf("base: base_grid.yaml: grid size must be positive")
	}
	if mapName == "" {
		mapName = f.DefaultMap
	}
	m, ok := f.Maps[mapName]
	if !ok {
		names := make([]string, 0, len(f.Maps))
		for n := range f.Maps {
			names = append(names, n)
		}
		sort.Strings(names)
		return Grid{}, fmt.Errorf("base: unknown map %q (have %v)", mapName, names)
	}
	var water []Point
	for _, r := range m.Water {
		if len(r) != 4 {
			return Grid{}, fmt.Errorf("base: map %q: water rect must be [x,y,w,h]", mapName)
		}
		for y := r[1]; y < r[1]+r[3]; y++ {
			for x := r[0]; x < r[0]+r[2]; x++ {
				water = append(water, Point{x, y})
			}
		}
	}
	return NewGrid(f.Grid.W, f.Grid.H, water), nil
}

// LoadGrid reads base_grid.yaml from a content directory (see ParseGrid).
func LoadGrid(contentDir, mapName string) (Grid, error) {
	b, err := os.ReadFile(filepath.Join(contentDir, "base_grid.yaml"))
	if err != nil {
		return Grid{}, err
	}
	return ParseGrid(b, mapName)
}
