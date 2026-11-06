package main

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func TestRecoveryKeepsRunAndPausedIntent(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	for _, paused := range []bool{false, true} {
		name := "running"
		if paused {
			name = "paused"
		}
		p := storedBBoxTask(t, name, "")
		run := &TaskRunRecord{ID: name + "-run", TaskRecordID: p.ID, UserID: 1, Status: TaskRunning, ArtifactStatus: ArtifactNone}
		if err := store.createRun(run); err != nil {
			t.Fatal(err)
		}
		if err := store.markTaskRecordRunning(p.ID, run.ID); err != nil {
			t.Fatal(err)
		}
		if paused {
			if err := store.updateTaskRecordStatus(p.ID, TaskRecordPaused); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := store.db.Exec(`INSERT INTO execution_queue(plan_id,trigger_mode,due_at,enqueued_at,state) VALUES(?,'immediate',0,0,'running')`, p.ID); err != nil {
			t.Fatal(err)
		}
		if err := store.recoverInterruptedTaskRecords(); err != nil {
			t.Fatal(err)
		}
		fresh, err := store.getTaskRecordByID(p.ID)
		if err != nil {
			t.Fatal(err)
		}
		if fresh.LastRunID != run.ID {
			t.Fatal("recovery replaced the run")
		}
		wantStatus, wantQueue := TaskRecordQueued, "queued"
		if paused {
			wantStatus, wantQueue = TaskRecordPaused, "paused"
		}
		if fresh.Status != wantStatus || store.queueState(p.ID).State != wantQueue {
			t.Fatalf("%s recovery: %+v queue=%+v", name, fresh, store.queueState(p.ID))
		}
	}
}

func TestExpandedCatalogueIsDeduplicatedAndReused(t *testing.T) {
	task := NewTask([]Layer{{Zoom: 1, URL: "fixture/{z}/{x}/{y}", Tiles: []maptile.Tile{maptile.New(0, 0, 1), maptile.New(0, 0, 1)}}}, TileMap{Format: PNG}, TaskOptions{})
	defer task.cancel()
	task.expectedPath = filepath.Join(t.TempDir(), "expected.sqlite")
	visits := 0
	task.expandExpected = func(job TileJob, emit func(TileJob) error) error {
		visits++
		if err := emit(job); err != nil {
			return err
		}
		job.Tile = maptile.New(1, 0, 1)
		return emit(job)
	}
	count, err := task.expectedCount()
	if err != nil || count != 2 {
		t.Fatalf("count=%d err=%v", count, err)
	}
	initialVisits := visits
	var iterated int
	if err := task.forEachExpected(func(TileJob) error { iterated++; return nil }); err != nil {
		t.Fatal(err)
	}
	if iterated != 2 || visits != initialVisits {
		t.Fatalf("catalogue rebuilt: count=%d expansions=%d/%d", iterated, visits, initialVisits)
	}
	task.expectedLimit = 1
	if _, err := task.expectedCount(); err == nil {
		t.Fatal("cached catalogue bypassed the current budget")
	}
}

func TestParentBudgetReservesActualCountsIdempotently(t *testing.T) {
	useFileTestStore(t)
	old := viper.GetInt64("task.max_tiles")
	viper.Set("task.max_tiles", 10)
	t.Cleanup(func() { viper.Set("task.max_tiles", old) })
	a := &TaskRecord{ID: "source-a", ParentID: "parent"}
	b := &TaskRecord{ID: "source-b", ParentID: "parent"}
	for i := 0; i < 2; i++ {
		if err := store.reserveTaskBudget(a, 7); err != nil {
			t.Fatal(err)
		}
	}
	var budget *TileBudgetError
	if err := store.reserveTaskBudget(b, 4); !errors.As(err, &budget) {
		t.Fatalf("overflow: %v", err)
	}
	if err := store.reserveTaskBudget(b, 3); err != nil {
		t.Fatal(err)
	}
	var total int64
	if err := store.db.QueryRow(`SELECT SUM(tiles) FROM task_budgets WHERE parent_id='parent'`).Scan(&total); err != nil || total != 10 {
		t.Fatalf("total=%d err=%v", total, err)
	}
}

func TestResumedFailureEventsDoNotCollide(t *testing.T) {
	useTestStore(t)
	p := storedBBoxTask(t, "events", "")
	run := &TaskRunRecord{ID: "event-run", TaskRecordID: p.ID, UserID: 1, Status: TaskRunning}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	record := TileFailureRecord{Z: 1, X: 0, Y: 0, URL: p.URL, Retryable: true, CreatedAt: time.Now()}
	if err := store.failureSink(p, run.ID)(record); err != nil {
		t.Fatal(err)
	}
	if err := store.failureSink(p, run.ID)(record); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM failures WHERE run_id=?`, run.ID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("events=%d err=%v", count, err)
	}
}

func TestOverlappingBBoxBudgetUsesUniqueRequests(t *testing.T) {
	useFileTestStore(t)
	old := viper.GetInt64("task.max_tiles")
	viper.Set("task.max_tiles", 6)
	t.Cleanup(func() { viper.Set("task.max_tiles", old) })
	level := LevelConfig{MinZoom: 1, MaxZoom: 1, Mode: "bbox", BBox: &BBoxRequest{MinLon: -10, MaxLon: 10, MinLat: -10, MaxLat: 10}}
	levels := []LevelConfig{level, level}
	estimate, err := validateTaskBudget(levels, 1)
	if err != nil || estimate != 8 {
		t.Fatalf("overlap estimate rejected: %d %v", estimate, err)
	}
	plan := &TaskRecord{ID: "overlap", Name: "overlap", URL: "http://fixture/{z}/{x}/{y}", Format: PNG, Schema: "xyz", Levels: levels}
	task, err := buildTaskFromRecord(plan)
	if err != nil {
		t.Fatal(err)
	}
	defer task.cancel()
	task.expectedLimit = 6
	task.expectedPath = filepath.Join(t.TempDir(), "expected.sqlite")
	actual, err := task.expectedCount()
	if err != nil || actual != 4 {
		t.Fatalf("unique count=%d err=%v", actual, err)
	}
	if err := store.reserveTaskBudget(plan, actual); err != nil {
		t.Fatal(err)
	}
	err = store.reserveTaskBudget(plan, 7)
	var budget *TileBudgetError
	if !errors.As(err, &budget) || !budget.Actual {
		t.Fatalf("actual overflow: %v", err)
	}
}
