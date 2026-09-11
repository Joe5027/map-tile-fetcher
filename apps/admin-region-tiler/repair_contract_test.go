package main

// These contracts use pre-repair interfaces so the identical tests can run
// against an archived baseline as well as the current implementation.
import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func contractStore(t *testing.T) *TaskRecord {
	t.Helper()
	t.Chdir(t.TempDir())
	previous, manager, output := store, runtimeManager, viper.GetString("output.directory")
	db, err := sql.Open("sqlite", "contract.db")
	if err != nil {
		t.Fatal(err)
	}
	store = &SQLiteStore{db: db}
	runtimeManager = NewRuntimeManager()
	viper.Set("output.directory", "output")
	t.Cleanup(func() { db.Close(); store = previous; runtimeManager = manager; viper.Set("output.directory", output) })
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	p := &TaskRecord{ID: "contract", UserID: 1, Kind: TaskRecordKindSingle, Name: "same", SourceName: "same", URL: "https://example.test/{z}/{x}/{y}", Format: PNG, Schema: "xyz", Workers: 3, Status: TaskRecordCompleted, ScheduleMode: ScheduleImmediate, RunAt: time.Now(), Levels: []LevelConfig{{MinZoom: 1, MaxZoom: 1, Mode: "bbox", BBox: &BBoxRequest{MinLon: 1, MaxLon: 2, MinLat: 1, MaxLat: 2}, OutputFormat: "zip"}}}
	if err := store.createTaskRecord(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestRepairContractIssue1RunIsolation(t *testing.T) {
	p := contractStore(t)
	paths := map[string]bool{}
	for _, id := range []string{"first", "second"} {
		output := filepath.Join("output", id)
		if err := os.MkdirAll(output, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(output, "tile.png"), []byte(id), 0644); err != nil {
			t.Fatal(err)
		}
		run := &TaskRunRecord{ID: id, TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, OutputPath: output}
		if err := store.createRun(run); err != nil {
			t.Fatal(err)
		}
		if err := prepareArtifactForRun(p, &Task{File: output, Status: TaskCompleted, outformat: "zip"}, run); err != nil {
			t.Fatal(err)
		}
		if paths[run.ArtifactPath] {
			t.Fatal("two runs publish to the same path")
		}
		paths[run.ArtifactPath] = true
	}
}

func TestRepairContractIssue3LastPublishedDownload(t *testing.T) {
	p := contractStore(t)
	if err := os.MkdirAll("output", 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("output/old.zip", []byte("published bytes"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, run := range []*TaskRunRecord{
		{ID: "published", TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactReady, ArtifactPath: "output/old.zip"},
		{ID: "failed", TaskRecordID: p.ID, UserID: 1, Status: TaskFailed, ArtifactStatus: ArtifactNone},
	} {
		if err := store.createRun(run); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.markTaskRecordRunning(p.ID, "failed"); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &UserRecord{ID: 1})
	c.Params = gin.Params{{Key: "id", Value: p.ID}}
	c.Request = httptest.NewRequest("GET", "/api/tasks/contract/download", nil)
	downloadTaskArtifact(c)
	if w.Code != 200 || w.Body.String() != "published bytes" {
		t.Fatalf("previous published artifact unavailable: %d %s", w.Code, w.Body.String())
	}
}

func TestRepairContractIssue4GroupDeletion(t *testing.T) {
	p := contractStore(t)
	p.Kind = TaskRecordKindGroup
	if _, err := store.db.Exec(`UPDATE plans SET kind='group' WHERE id=?`, p.ID); err != nil {
		t.Fatal(err)
	}
	child := *p
	child.ID = "child"
	child.ParentID = p.ID
	child.Kind = TaskRecordKindChild
	if err := store.createTaskRecord(&child); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll("output/child", 0755); err != nil {
		t.Fatal(err)
	}
	if err := store.createRun(&TaskRunRecord{ID: "child-run", TaskRecordID: child.ID, UserID: 1, Status: TaskCompleted, OutputPath: "output/child"}); err != nil {
		t.Fatal(err)
	}
	if err := runtimeManager.Purge(p); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat("output/child"); !os.IsNotExist(err) {
		t.Fatal("group deletion left child output")
	}
}

func TestRepairContractIssue5CreationFileOwnership(t *testing.T) {
	contractStore(t)
	path, err := writeGeneratedPolygonGeoJSON([]CoordinateRequest{{Lon: 1, Lat: 1}, {Lon: 2, Lat: 1}, {Lon: 2, Lat: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_create BEFORE INSERT ON plans BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(CreateTaskRequest{Name: "new", URL: "https://example.test/{z}/{x}/{y}", Format: PNG, Levels: []LevelRequest{{MinZoom: 1, MaxZoom: 1, Geojson: path}}})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &UserRecord{ID: 1})
	c.Request = httptest.NewRequest("POST", "/api/tasks", bytes.NewReader(raw))
	c.Request.Header.Set("Content-Type", "application/json")
	createTask(c)
	if w.Code != 500 {
		t.Fatalf("fixture did not fail: %d", w.Code)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("failed creation removed a pre-existing area")
	}
}

func TestRepairContractIssue6ParentBudget(t *testing.T) {
	previous := viper.Get("task.max_tiles")
	viper.Set("task.max_tiles", 6)
	defer viper.Set("task.max_tiles", previous)
	_, _, err := buildTaskRecordsFromRequest(1, CreateTaskRequest{Name: "budget", Mode: "bbox", Area: AreaRequest{BBox: &BBoxRequest{MinLon: -10, MaxLon: 10, MinLat: -10, MaxLat: 10}}, Zoom: &ZoomRangeRequest{Min: 1, Max: 1}, Sources: []SourceRequest{{Name: "a", URL: "https://example.test/{z}/{x}/{y}", Format: PNG}, {Name: "b", URL: "https://example.test/{z}/{x}/{y}", Format: PNG}}})
	if err == nil {
		t.Fatal("8 tiles accepted despite parent limit of 6")
	}
}

func TestRepairContractIssue7RequestedWorkers(t *testing.T) {
	task := NewTask([]Layer{{Tiles: []maptile.Tile{maptile.New(0, 0, 0)}}}, TileMap{}, TaskOptions{WorkerCount: 1})
	defer task.cancel()
	if task.workerCount != 1 {
		t.Fatalf("requested 1 worker, got %d", task.workerCount)
	}
}

func TestRepairContractIssue8PreviewExpiry(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &UserRecord{ID: 987})
	c.Request = httptest.NewRequest("POST", "/api/tile-preview/tianditu-token", bytes.NewBufferString(`{"token":"fixture-token"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	registerTiandituPreviewToken(c)
	var response map[string]json.RawMessage
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response["expiresAt"]) == 0 {
		t.Fatal("preview registration omitted expiration")
	}
}

func TestRepairContractIssue9AreaLevels(t *testing.T) {
	p := contractStore(t)
	if err := os.MkdirAll("data/generated-areas", 0755); err != nil {
		t.Fatal(err)
	}
	path := "data/generated-areas/contract.geojson"
	if err := os.WriteFile(path, []byte(`{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},"geometry":{"type":"Point","coordinates":[1,1]}},{"type":"Feature","properties":{},"geometry":{"type":"Point","coordinates":[2,2]}}]}`), 0644); err != nil {
		t.Fatal(err)
	}
	levels, _ := json.Marshal([]LevelConfig{{MinZoom: 1, MaxZoom: 2, Geojson: path}, {MinZoom: 3, MaxZoom: 4, Geojson: path}})
	if _, err := store.db.Exec(`UPDATE plans SET levels_json=? WHERE id=?`, string(levels), p.ID); err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &UserRecord{ID: 1})
	c.Params = gin.Params{{Key: "id", Value: p.ID}}
	c.Request = httptest.NewRequest("GET", "/api/tasks/contract/area?level=0", nil)
	getTaskArea(c)
	var response struct {
		Features []json.RawMessage
		Levels   []json.RawMessage
		Selected int `json:"selectedLevel"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if len(response.Features) != 2 || len(response.Levels) != 2 || response.Selected != 0 {
		t.Fatalf("area contract missing: %s", w.Body.String())
	}
}
