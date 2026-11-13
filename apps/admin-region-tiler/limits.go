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
	Actual       bool
}

func (e *TileBudgetError) Error() string {
	if e.Actual {
		return fmt.Sprintf("tile budget exceeded: %d unique requests counted, limit %d", e.Estimated, e.Limit)
	}
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
	if requested > 256 {
		requested = 256
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
	conservative := len(levels) > 1
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
	// A region's enclosing rectangle is an estimate, not its tile set. The
	// worker reserves the real deduplicated count before issuing requests.
	if n > maxTaskTiles() && !conservative {
		return n, &TileBudgetError{Estimated: n, Limit: maxTaskTiles(), Conservative: conservative}
	}
	return n, nil
}

func (s *SQLiteStore) reserveTaskBudget(plan *TaskRecord, actual int64) error {
	parent := plan.ParentID
	if parent == "" {
		parent = plan.ID
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// The first write obtains SQLite's writer lock before reading the sum.
	if _, err := tx.Exec(`INSERT INTO task_budgets(plan_id,parent_id,tiles) VALUES(?,?,?) ON CONFLICT(plan_id) DO UPDATE SET parent_id=excluded.parent_id,tiles=excluded.tiles`, plan.ID, parent, actual); err != nil {
		return err
	}
	var total int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(tiles),0) FROM task_budgets WHERE parent_id=?`, parent).Scan(&total); err != nil {
		return err
	}
	if total > maxTaskTiles() {
		return &TileBudgetError{Estimated: total, Limit: maxTaskTiles(), Actual: true}
	}
	return tx.Commit()
}
