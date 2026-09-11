package main

import (
	"context"
	"errors"
	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
	"testing"
)

func TestWorkerLimitsRespectRequestedCount(t *testing.T) {
	for _, requested := range []int{1, 3, 20, 50} {
		for _, cap := range []int{0, 1, 3, 20} {
			want := requested
			if cap > 0 && cap < want {
				want = cap
			}
			task := NewTask([]Layer{{Tiles: []maptile.Tile{maptile.New(0, 0, 0)}}}, TileMap{}, TaskOptions{WorkerCount: requested, TimeDelay: 120, Policy: FetchPolicy{WorkerCount: cap, BaseDelayMS: 50}})
			if task.workerCount != want || task.timeDelay != 120 {
				t.Fatalf("requested %d cap %d got workers=%d delay=%d", requested, cap, task.workerCount, task.timeDelay)
			}
		}
	}
}

func TestTileBudgetCountsEverySourceBeforeEnumeration(t *testing.T) {
	previous := viper.GetInt64("task.max_tiles")
	viper.Set("task.max_tiles", 6)
	t.Cleanup(func() { viper.Set("task.max_tiles", previous) })
	_, _, err := buildTaskRecordsFromRequest(1, CreateTaskRequest{Name: "budget", Mode: "bbox", Area: AreaRequest{BBox: &BBoxRequest{MinLon: -10, MaxLon: 10, MinLat: -10, MaxLat: 10}}, Zoom: &ZoomRangeRequest{Min: 1, Max: 1}, Sources: []SourceRequest{{Name: "a", URL: "http://example.test/{z}/{x}/{y}", Format: PNG}, {Name: "b", URL: "http://example.test/{z}/{x}/{y}", Format: PNG}}})
	var budget *TileBudgetError
	if !errors.As(err, &budget) || budget.Estimated != 8 || budget.Limit != 6 {
		t.Fatalf("budget error %v", err)
	}
}

func TestLargeBBoxBuildStoresDescriptionsOnly(t *testing.T) {
	task, err := buildTaskFromRequest(CreateTaskRequest{Name: "large", URL: "http://example.test/{z}/{x}/{y}", Format: PNG, Levels: []LevelRequest{{MinZoom: 9, MaxZoom: 9, Mode: "bbox", BBox: &BBoxRequest{MinLon: -179, MaxLon: 179, MinLat: -80, MaxLat: 80}}}})
	if err != nil {
		t.Fatal(err)
	}
	if task.Total < 100000 {
		t.Fatalf("fixture too small: %d", task.Total)
	}
	if len(task.Layers) != 1 || len(task.Layers[0].Tiles) != 0 || task.Layers[0].BBox == nil {
		t.Fatal("build materialized tile arrays")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := walkLayer(ctx, task.Layers[0], func(maptile.Tile) error { t.Fatal("enumerated after cancellation"); return nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error %v", err)
	}
}

func TestFourthTaskQueuesAndSurvivesRestart(t *testing.T) {
	useTestStore(t)
	m := NewRuntimeManager()
	for _, id := range []string{"a", "b", "c"} {
		m.active[id] = &ActiveRun{Plan: &TaskRecord{ID: id, Status: TaskRecordPaused}}
	}
	p := storedBBoxTask(t, "fourth", "")
	if err := m.StartTaskRecord(p); err != nil {
		t.Fatal(err)
	}
	q := store.queueState(p.ID)
	if q.State != "queued" || q.Position != 1 {
		t.Fatalf("fourth task: %+v", q)
	}
	runs, err := store.listRunsByTaskRecord(p.ID)
	if err != nil || len(runs) != 0 {
		t.Fatal("queued task started a run")
	}
	if err := m.StartTaskRecord(p); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM execution_queue`).Scan(&count); err != nil || count != 1 {
		t.Fatal("duplicate queue entry")
	}
	if err := store.recoverInterruptedTaskRecords(); err != nil {
		t.Fatal(err)
	}
	if q := store.queueState(p.ID); q.State != "queued" {
		t.Fatalf("queue lost after restart: %+v", q)
	}
	p, err = store.getTaskRecordByID(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Cancel(p); err != nil {
		t.Fatal(err)
	}
	if q := store.queueState(p.ID); q.State != "" {
		t.Fatal("cancelled queued entry survived")
	}
}
