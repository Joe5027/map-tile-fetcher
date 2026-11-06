package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
)

func TestEnumerationReportsActualCountAndReusesCatalogue(t *testing.T) {
	tiles := make([]maptile.Tile, 300)
	for i := range tiles {
		tiles[i] = maptile.New(uint32(i), 0, 9)
	}
	task := NewTask([]Layer{{Zoom: 9, Tiles: tiles}}, TileMap{Format: PNG}, TaskOptions{})
	defer task.cancel()
	task.expectedPath = filepath.Join(t.TempDir(), "expected.sqlite")
	task.expandExpected = func(job TileJob, emit func(TileJob) error) error {
		time.Sleep(time.Millisecond)
		if err := emit(job); err != nil {
			return err
		}
		return emit(job)
	}
	var progress [][2]int64
	task.expectedProgress = func(current, estimate int64) error {
		progress = append(progress, [2]int64{current, estimate})
		return nil
	}
	count, err := task.expectedCount()
	if err != nil || count != 300 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	if len(progress) < 3 || len(progress) > 8 || progress[0] != [2]int64{0, 300} || progress[len(progress)-1] != [2]int64{300, 300} {
		t.Fatalf("missing or unbounded enumeration progress: %v", progress)
	}
	before := len(progress)
	task.expandExpected = func(TileJob, func(TileJob) error) error { t.Fatal("cache rebuilt"); return nil }
	count, err = task.expectedCount()
	if err != nil || count != 300 || len(progress) != before+1 || progress[len(progress)-1][0] != 300 {
		t.Fatalf("cached count not reported: count=%d progress=%v err=%v", count, progress, err)
	}
}
