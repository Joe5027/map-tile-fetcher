package main

import (
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
)

func TestPersistenceFailureUnblocksPausedTask(t *testing.T) {
	task := NewTask([]Layer{{Zoom: 1, Tiles: []maptile.Tile{maptile.New(0, 0, 1)}}}, TileMap{Format: PNG}, TaskOptions{})
	task.workerManaged = true
	task.setStatus(TaskRunning)
	if err := task.Pause(); err != nil {
		t.Fatal(err)
	}
	defer task.Cancel()
	waiting := make(chan error, 1)
	go func() { waiting <- task.waitIfPaused() }()
	task.failureSink = func(TileFailureRecord) error { return errors.New("forced persistence I/O failure") }
	task.recordTileFailure(TileJob{Tile: maptile.New(0, 0, 1)}, errors.New("upstream failed"), true)
	select {
	case err := <-waiting:
		if err == nil {
			t.Fatal("paused worker ignored cancelled context")
		}
	case <-time.After(time.Second):
		t.Fatal("persistence failure left paused worker blocked")
	}
}

func TestMBTilesWriteFailureIsFailedNotUserCancelled(t *testing.T) {
	path := filepath.Join(t.TempDir(), "failure.mbtiles")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE tiles(zoom_level INTEGER,tile_column INTEGER CHECK(tile_column<0),tile_row INTEGER,tile_data BLOB); CREATE TABLE metadata(name TEXT,value TEXT)`)
	db.Close()
	if err != nil {
		t.Fatal(err)
	}
	png := testPNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(10 * time.Millisecond):
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer upstream.Close()
	tiles := make([]maptile.Tile, 100)
	for i := range tiles {
		tiles[i] = maptile.New(uint32(i), 0, 8)
	}
	task := NewTask([]Layer{{Zoom: 8, URL: upstream.URL + "/{z}/{x}/{y}", Tiles: tiles}}, TileMap{Format: PNG, Schema: "xyz"}, TaskOptions{WorkerCount: 1, SavePipeSize: 1, BufferSize: 1, OutputFormat: "mbtiles", RequestTTL: time.Second})
	task.File, task.preserveOutput = path, true
	defer task.Cancel()
	done := make(chan struct{})
	go func() { task.Run(); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("storage failure did not stop the pipeline")
	}
	if task.currentStatus() != TaskFailed {
		t.Fatalf("storage failure status=%s error=%s", task.currentStatus(), task.ErrorMessage)
	}
	if task.currentCounts().Success != 0 || task.currentCounts().Failure == 0 {
		t.Fatalf("uncommitted batch counts: %+v", task.currentCounts())
	}
}
