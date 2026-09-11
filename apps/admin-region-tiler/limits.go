package main

import (
	"fmt"
	"github.com/paulmach/orb"
	"github.com/spf13/viper"
	"tiler/internal/area"
	"tiler/internal/downloader"
)

type TileBudgetError struct {
	Estimated    int64
	Limit        int64
	Conservative bool
}

func (e *TileBudgetError) Error() string {
	return fmt.Sprintf("tile budget exceeded: estimated %d, limit %d (conservative geometry estimate: %t)", e.Estimated, e.Limit, e.Conservative)
}
func maxTaskTiles() int64 {
	n := viper.GetInt64("task.max_tiles")
	if n <= 0 {
		return 1000000
	}
	return n
}
func maxActiveTasks() int {
	n := viper.GetInt("task.max_active")
	if n <= 0 {
		return 3
	}
	return n
}
func effectiveWorkers(requested int, policy FetchPolicy) int {
	if requested <= 0 {
		requested = firstPositive(viper.GetInt("task.workers"), 3)
	}
	if requested > 50 {
		requested = 50
	}
	if policy.WorkerCount > 0 && requested > policy.WorkerCount {
		requested = policy.WorkerCount
	}
	return requested
}
func boundBudget(bound orb.Bound, minZoom, maxZoom int) (int64, error) {
	box := area.BBox{MinLon: bound.Min[0], MinLat: bound.Min[1], MaxLon: bound.Max[0], MaxLat: bound.Max[1]}
	if box.MinLat < -85.05112878 {
		box.MinLat = -85.05112878
	}
	if box.MaxLat > 85.05112878 {
		box.MaxLat = 85.05112878
	}
	return downloader.CountBBoxTiles(box, area.ZoomRange{Min: minZoom, Max: maxZoom})
}
func estimateLevels(levels []LevelConfig) (int64, bool, error) {
	var total int64
	conservative := false
	for _, level := range levels {
		var count int64
		var err error
		if level.BBox != nil {
			b := level.BBox
			count, err = downloader.CountBBoxTiles(area.BBox{MinLon: b.MinLon, MinLat: b.MinLat, MaxLon: b.MaxLon, MaxLat: b.MaxLat}, area.ZoomRange{Min: level.MinZoom, Max: level.MaxZoom})
		} else {
			conservative = true
			collection, readErr := loadCollection(level.Geojson)
			if readErr != nil {
				return 0, conservative, readErr
			}
			count, err = boundBudget(collection.Bound(), level.MinZoom, level.MaxZoom)
		}
		if err != nil {
			return 0, conservative, err
		}
		total += count
	}
	return total, conservative, nil
}
func validateTaskBudget(levels []LevelConfig, sources int) (int64, error) {
	n, conservative, err := estimateLevels(levels)
	if err != nil {
		return 0, err
	}
	n *= int64(sources)
	if n > maxTaskTiles() {
		return n, &TileBudgetError{Estimated: n, Limit: maxTaskTiles(), Conservative: conservative}
	}
	return n, nil
}
