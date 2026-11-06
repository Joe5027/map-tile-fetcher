package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func tileRectangle(x0, y0, x1, y1 uint32, zoom maptile.Zoom) orb.Polygon {
	first, last := maptile.New(x0, y0, zoom).Bound(), maptile.New(x1, y1, zoom).Bound()
	const inset = 0.0001
	left, right := first.Min[0]+inset, last.Max[0]-inset
	bottom, top := last.Min[1]+inset, first.Max[1]-inset
	return orb.Polygon{{{left, bottom}, {right, bottom}, {right, top}, {left, top}, {left, bottom}}}
}

func TestRealRegionGeometryUsesUniqueBudget(t *testing.T) {
	thin := tileRectangle(11, 2, 11, 27, 5)
	thin[0][1][0] = thin[0][0][0] + 0.0002
	thin[0][2][0] = thin[0][1][0]
	previous := viper.GetInt64("task.max_tiles")
	t.Cleanup(func() { viper.Set("task.max_tiles", previous) })
	for _, fixture := range []struct {
		name    string
		regions []orb.Collection
		want    int64
	}{
		{"long_thin", []orb.Collection{{thin}}, 26},
		{"distant_islands", []orb.Collection{{orb.MultiPolygon{tileRectangle(1, 2, 1, 2, 5), tileRectangle(29, 28, 29, 28, 5)}}}, 2},
		{"overlapping_regions", []orb.Collection{{tileRectangle(10, 10, 11, 11, 5)}, {tileRectangle(11, 10, 12, 11, 5)}}, 6},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			withTempWorkingDir(t)
			viper.Set("task.max_tiles", fixture.want)
			var levels []LevelConfig
			for i, region := range fixture.regions {
				collection := geojson.NewFeatureCollection()
				for _, shape := range region {
					collection.Append(geojson.NewFeature(shape))
				}
				data, err := json.Marshal(collection)
				if err != nil {
					t.Fatal(err)
				}
				path := filepath.Join(defaultDataDir, "generated-areas", fmt.Sprintf("region-%d.geojson", i))
				if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, data, 0600); err != nil {
					t.Fatal(err)
				}
				levels = append(levels, LevelConfig{Geojson: path, MinZoom: 5, MaxZoom: 5})
			}
			estimate, err := validateTaskBudget(levels, 1)
			if err != nil {
				t.Fatalf("estimate rejected real geometry: %v", err)
			}
			if fixture.name != "long_thin" && estimate <= fixture.want {
				t.Fatalf("fixture did not exercise overestimate: %d", estimate)
			}
			plan := &TaskRecord{ID: fixture.name, Name: fixture.name, URL: "http://fixture/{z}/{x}/{y}", Format: PNG, Schema: "xyz", Levels: levels}
			task, err := buildTaskFromRecord(plan)
			if err != nil {
				t.Fatal(err)
			}
			defer task.cancel()
			task.expectedPath = filepath.Join(t.TempDir(), "expected.sqlite")
			task.expectedLimit = fixture.want
			actual, err := task.expectedCount()
			if err != nil || actual != fixture.want {
				t.Fatalf("estimate=%d actual=%d want=%d err=%v", estimate, actual, fixture.want, err)
			}
			seen := map[maptile.Tile]bool{}
			if err := task.forEachExpected(func(job TileJob) error {
				if seen[job.Tile] {
					t.Errorf("duplicate coordinate %v", job.Tile)
				}
				seen[job.Tile] = true
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			task.expectedLimit = fixture.want - 1
			_, err = task.expectedCount()
			var budget *TileBudgetError
			if !errors.As(err, &budget) || !budget.Actual || budget.Estimated != fixture.want {
				t.Fatalf("actual excess not rejected: %v", err)
			}
			t.Logf("geometry=%s estimated=%d unique=%d", fixture.name, estimate, actual)
		})
	}
}
