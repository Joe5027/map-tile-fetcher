package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func taskCreationRequest(t *testing.T, req CreateTaskRequest) *httptest.ResponseRecorder {
	t.Helper()
	previous := runtimeManager
	runtimeManager = NewRuntimeManager()
	defer func() { runtimeManager = previous }()
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &UserRecord{ID: 1})
	c.Request = httptest.NewRequest(http.MethodPost, "/api/tasks", bytes.NewReader(data))
	c.Request.Header.Set("Content-Type", "application/json")
	createTask(c)
	return w
}

func TestFailedCreationPreservesReferencedArea(t *testing.T) {
	withTempWorkingDir(t)
	useTestStore(t)
	path, err := writeGeneratedPolygonGeoJSON([]CoordinateRequest{{Lon: 1, Lat: 1}, {Lon: 2, Lat: 1}, {Lon: 2, Lat: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_create BEFORE INSERT ON plans BEGIN SELECT RAISE(ABORT,'simulated failure'); END`); err != nil {
		t.Fatal(err)
	}
	w := taskCreationRequest(t, CreateTaskRequest{Name: "new", URL: "https://example.test/{z}/{x}/{y}", Format: PNG, Levels: []LevelRequest{{MinZoom: 1, MaxZoom: 1, Geojson: path}}})
	if w.Code != 500 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal("existing area removed by unrelated failed request", err)
	}
}

func TestSecondChildFailureRollsBackAPIAndGeneratedFile(t *testing.T) {
	withTempWorkingDir(t)
	useTestStore(t)
	if _, err := store.db.Exec(`CREATE TRIGGER reject_second BEFORE INSERT ON task_sources WHEN NEW.name = 'second' BEGIN SELECT RAISE(ABORT,'second child failure'); END`); err != nil {
		t.Fatal(err)
	}
	w := taskCreationRequest(t, CreateTaskRequest{Name: "transaction", Mode: "bbox", Area: AreaRequest{Polygon: []CoordinateRequest{{Lon: 1, Lat: 1}, {Lon: 2, Lat: 1}, {Lon: 2, Lat: 2}}}, Zoom: &ZoomRangeRequest{Min: 1, Max: 1}, Sources: []SourceRequest{{Name: "first", URL: "https://example.test/{z}/{x}/{y}", Format: PNG}, {Name: "second", URL: "https://example.test/{z}/{x}/{y}", Format: PNG}}})
	if w.Code != 500 {
		t.Fatalf("status %d: %s", w.Code, w.Body.String())
	}
	for _, table := range []string{"plans", "tasks", "task_sources", "task_runs"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s has %d leftover records", table, count)
		}
	}
	files, err := filepath.Glob("data/generated-areas/*.geojson")
	if err != nil || len(files) > 0 {
		t.Fatalf("request-owned file leaked: %v %v", files, err)
	}
}

func TestLegacyLongTaskNameRemainsReadable(t *testing.T) {
	useTestStore(t)
	p := storedBBoxTask(t, "legacy-name", "")
	name := strings.Repeat("旧", 120)
	if _, err := store.db.Exec(`UPDATE plans SET name = ? WHERE id = ?`, name, p.ID); err != nil {
		t.Fatal(err)
	}
	p, err := store.getTaskRecordForUser(1, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	if actual := taskResponseFromRecord(p).Name; actual != name {
		t.Fatalf("legacy name lost: %s", actual)
	}
}
