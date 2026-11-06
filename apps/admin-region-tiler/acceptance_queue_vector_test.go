package main

import (
	"bytes"
	"compress/gzip"
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func TestLargeBaselineRetryQueuesWithoutReadingOrBlockingOtherTask(t *testing.T) {
	useFileTestStore(t)
	root := setTestOutput(t)
	previousManager, previousActive := runtimeManager, viper.GetInt("task.max_active")
	runtimeManager = NewRuntimeManager()
	viper.Set("task.max_active", 1)
	t.Cleanup(func() { runtimeManager = previousManager; viper.Set("task.max_active", previousActive) })
	runtimeManager.active["occupied"] = &ActiveRun{Plan: &TaskRecord{ID: "occupied"}}
	plan := storedBBoxTask(t, "large-retry", "")
	plan.Levels = []LevelConfig{{MinZoom: 8, MaxZoom: 8, Mode: "bbox", BBox: &BBoxRequest{MinLon: -179.99, MinLat: -85, MaxLon: 179.99, MaxLat: 85}, OutputFormat: "mbtiles"}}
	levels, err := json.Marshal(plan.Levels)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`UPDATE plans SET levels_json=? WHERE id=?`, string(levels), plan.ID); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "large.mbtiles")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE tiles(zoom_level INTEGER,tile_column INTEGER,tile_row INTEGER,tile_data BLOB,PRIMARY KEY(zoom_level,tile_column,tile_row)); CREATE TABLE metadata(name TEXT,value TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`WITH RECURSIVE n(v) AS (VALUES(0) UNION ALL SELECT v+1 FROM n WHERE v<65534) INSERT INTO tiles SELECT 8,v/256,255-(v%256),? FROM n`, testPNG(t)); err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "large-baseline", TaskRecordID: plan.ID, UserID: plan.UserID, Status: TaskPartialFailed, ArtifactStatus: ArtifactReady, OutputPath: path, ArtifactPath: path, Total: 65536, SuccessCount: 65535, FailureCount: 1}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.markTaskRecordRunning(plan.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := store.updateTaskRecordStatus(plan.ID, TaskRecordPartialFailed); err != nil {
		t.Fatal(err)
	}
	if err := store.replaceFailureRecords(run, []TileFailureRecord{{Z: 8, X: 255, Y: 255, URL: prepareTileURL(maptile.New(255, 255, 8), plan.URL), Retryable: true}}); err != nil {
		t.Fatal(err)
	}
	// Any synchronous baseline read is now impossible. Admission and unrelated
	// scheduler operations must still complete while this exclusive lock is held.
	if _, err := db.Exec(`BEGIN EXCLUSIVE`); err != nil {
		t.Fatal(err)
	}
	defer db.Exec(`ROLLBACK`)
	request := func(handler gin.HandlerFunc, id, method string) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &UserRecord{ID: 1})
		c.Params = gin.Params{{Key: "id", Value: id}}
		c.Request = httptest.NewRequest(method, "/api/tasks/"+id, nil)
		done := make(chan struct{})
		go func() { handler(c); close(done) }()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("task operation waited for large baseline")
		}
		return w
	}
	started := time.Now()
	w := request(retryTaskFailures, plan.ID, http.MethodPost)
	if w.Code != http.StatusAccepted {
		t.Fatalf("retry did not enqueue: %d %s", w.Code, w.Body.String())
	}
	if q := store.queueState(plan.ID); q.State != "queued" {
		t.Fatalf("retry not queued: %+v", q)
	}
	other := storedBBoxTask(t, "unrelated", "")
	if err := runtimeManager.StartTaskRecord(other); err != nil {
		t.Fatal(err)
	}
	w = request(cancelTask, other.ID, http.MethodDelete)
	if w.Code != http.StatusOK {
		t.Fatalf("other operation blocked: %d %s", w.Code, w.Body.String())
	}
	published, err := store.publishedRun(plan.ID)
	if err != nil || published.ID != run.ID {
		t.Fatalf("enqueue changed published artifact: %+v %v", published, err)
	}
	t.Logf("65535-tile baseline locked; retry 202 and unrelated enqueue/cancel finished in %s", time.Since(started))
}

func TestVectorDownloadPreservesProtobufBytes(t *testing.T) {
	// A valid empty MVT layer named roads, with version and extent. It is not
	// an image and includes bytes that image normalization must never rewrite.
	payload := []byte{0x1a, 0x0c, 0x0a, 0x05, 'r', 'o', 'a', 'd', 's', 0x28, 0x80, 0x20, 0x78, 0x02}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/x-protobuf")
		_, _ = w.Write(payload)
	}))
	defer upstream.Close()
	for _, format := range []string{"zip", "mbtiles"} {
		t.Run(format, func(t *testing.T) {
			root := setTestOutput(t)
			tile := maptile.New(0, 0, 0)
			task := NewTask([]Layer{{Zoom: 0, Tiles: []maptile.Tile{tile}}}, TileMap{Format: PBF, Schema: "xyz", URL: upstream.URL + "/{z}/{x}/{y}"}, TaskOptions{OutputFormat: format, WorkerCount: 1, RequestTTL: time.Second})
			defer task.cancel()
			task.File = filepath.Join(root, "tiles")
			if format == "mbtiles" {
				task.File += ".mbtiles"
			}
			task.Run()
			if task.currentStatus() != TaskCompleted {
				t.Fatalf("vector download failed: %+v", task.snapshot())
			}
			reader, err := openTileOutput(&TaskRecord{Format: PBF, Schema: "xyz"}, &TaskRunRecord{OutputPath: task.File})
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			compressed, err := reader.read(tile)
			if err != nil {
				t.Fatal(err)
			}
			zr, err := gzip.NewReader(bytes.NewReader(compressed))
			if err != nil {
				t.Fatal(err)
			}
			actual, err := io.ReadAll(zr)
			zr.Close()
			if err != nil || !bytes.Equal(actual, payload) {
				t.Fatalf("vector bytes changed: %x want %x err=%v", actual, payload, err)
			}
		})
	}
}
