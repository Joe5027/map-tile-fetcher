package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestBuildPlansFromRequestBBoxCreatesDirectLevel(t *testing.T) {
	withTempWorkingDir(t)

	parent, children, err := buildTaskRecordsFromRequest(42, CreateTaskRequest{
		Name: "bbox task",
		Mode: "bbox",
		Area: AreaRequest{BBox: &BBoxRequest{
			MinLon: 100,
			MinLat: 20,
			MaxLon: 101,
			MaxLat: 21,
		}},
		Zoom: &ZoomRangeRequest{Min: 1, Max: 2},
		Sources: []SourceRequest{
			{Name: "img", URL: "https://example.test/img/{z}/{x}/{y}.png", Format: PNG, Schema: "xyz"},
			{Name: "cia", URL: "https://example.test/cia/{z}/{x}/{y}.png", Format: PNG, Schema: "xyz"},
		},
		Output: OutputRequest{Format: "mbtiles"},
	})
	if err != nil {
		t.Fatalf("buildTaskRecordsFromRequest returned error: %v", err)
	}
	if parent.Kind != TaskRecordKindGroup {
		t.Fatalf("expected group plan, got %s", parent.Kind)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 child plans, got %d", len(children))
	}
	if len(parent.Levels) != 1 {
		t.Fatalf("expected 1 bbox level, got %d", len(parent.Levels))
	}

	level := parent.Levels[0]
	if level.MinZoom != 1 || level.MaxZoom != 2 {
		t.Fatalf("unexpected zoom range: %#v", level)
	}
	if level.Mode != "bbox" || level.BBox == nil {
		t.Fatalf("expected direct bbox level, got %#v", level)
	}
	if level.OutputFormat != "mbtiles" {
		t.Fatalf("expected mbtiles output metadata, got %s", level.OutputFormat)
	}
	if level.Geojson != "" {
		t.Fatalf("bbox level should not depend on generated geojson, got %s", level.Geojson)
	}
	if children[0].SourceName != "img" || children[1].SourceName != "cia" {
		t.Fatalf("unexpected child source names: %q %q", children[0].SourceName, children[1].SourceName)
	}
	if _, statErr := os.Stat("data"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("bbox task should not write generated data, stat error: %v", statErr)
	}

	task, err := buildTaskFromRecord(children[0])
	if err != nil {
		t.Fatalf("direct bbox child task should build: %v", err)
	}
	if task.Total == 0 {
		t.Fatal("direct bbox task should enumerate tiles")
	}
	if task.outformat != "mbtiles" {
		t.Fatalf("expected task output format mbtiles, got %s", task.outformat)
	}
}

func TestBuildPlansFromRequestRejectsInvalidBBoxWithoutWritingData(t *testing.T) {
	withTempWorkingDir(t)

	_, _, err := buildTaskRecordsFromRequest(42, CreateTaskRequest{
		Name: "bad bbox",
		Mode: "bbox",
		Area: AreaRequest{BBox: &BBoxRequest{
			MinLon: 101,
			MinLat: 20,
			MaxLon: 100,
			MaxLat: 21,
		}},
		Zoom: &ZoomRangeRequest{Min: 1, Max: 2},
		Sources: []SourceRequest{{
			Name:   "img",
			URL:    "https://example.test/img/{z}/{x}/{y}.png",
			Format: PNG,
			Schema: "xyz",
		}},
	})
	if err == nil {
		t.Fatal("expected invalid bbox error")
	}
	if _, statErr := os.Stat("data"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid bbox should not write generated data, stat error: %v", statErr)
	}
}

func TestBuildPlansFromRequestPolygonCreatesGeneratedGeoJSONLevel(t *testing.T) {
	withTempWorkingDir(t)

	parent, children, err := buildTaskRecordsFromRequest(42, CreateTaskRequest{
		Name: "polygon task",
		Mode: "bbox",
		Area: AreaRequest{Polygon: []CoordinateRequest{
			{Lon: 100, Lat: 20},
			{Lon: 101, Lat: 20},
			{Lon: 101, Lat: 21},
			{Lon: 100, Lat: 21},
		}},
		Zoom: &ZoomRangeRequest{Min: 2, Max: 2},
		Sources: []SourceRequest{{
			Name:   "img",
			URL:    "https://example.test/img/{z}/{x}/{y}.png",
			Format: PNG,
			Schema: "xyz",
		}},
		Output: OutputRequest{Format: "zip"},
	})
	if err != nil {
		t.Fatalf("buildTaskRecordsFromRequest returned error: %v", err)
	}
	if len(parent.Levels) != 1 {
		t.Fatalf("expected 1 polygon level, got %d", len(parent.Levels))
	}
	level := parent.Levels[0]
	if level.BBox != nil || level.Mode == "bbox" {
		t.Fatalf("polygon level should use generated geojson, got %#v", level)
	}
	if level.Geojson == "" {
		t.Fatal("expected generated geojson path")
	}
	normalizedPath := filepath.ToSlash(level.Geojson)
	if !strings.HasPrefix(normalizedPath, "data/generated-areas/") && !strings.Contains(normalizedPath, "/data/generated-areas/") {
		t.Fatalf("expected generated area path, got %s", level.Geojson)
	}
	if _, err := os.Stat(level.Geojson); err != nil {
		t.Fatalf("generated polygon geojson missing: %v", err)
	}
	task, err := buildTaskFromRecord(children[0])
	if err != nil {
		t.Fatalf("polygon child task should build: %v", err)
	}
	if task.Total == 0 {
		t.Fatal("polygon task should enumerate tiles")
	}
}

func TestPlanResponseIncludesUnifiedTaskContractFields(t *testing.T) {
	previousStore := store
	store = newSQLiteTestStore(t)
	t.Cleanup(func() {
		store = previousStore
	})

	runAt := time.Unix(2000, 0)
	parent := &TaskRecord{
		ID:           "task-contract",
		UserID:       42,
		Kind:         TaskRecordKindGroup,
		Name:         "bbox contract",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels: []LevelConfig{{
			MinZoom: 3,
			MaxZoom: 4,
			Mode:    "bbox",
			BBox:    &BBoxRequest{MinLon: 100, MinLat: 20, MaxLon: 101, MaxLat: 21},
		}},
	}
	child := &TaskRecord{
		ID:           "task-contract-img",
		UserID:       42,
		ParentID:     parent.ID,
		Kind:         TaskRecordKindChild,
		Name:         parent.Name,
		SourceName:   "img",
		URL:          parent.URL,
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       parent.Levels,
	}
	parent.Children = []*TaskRecord{child}
	if err := store.createTaskRecord(parent); err != nil {
		t.Fatalf("create parent failed: %v", err)
	}
	if err := store.createTaskRecord(child); err != nil {
		t.Fatalf("create child failed: %v", err)
	}

	response := taskResponseFromRecord(parent)
	if response.Mode != "bbox" || response.Area.BBox == nil {
		t.Fatalf("expected bbox response area, got %#v", response.Area)
	}
	if response.Zoom == nil || response.Zoom.Min != 3 || response.Zoom.Max != 4 {
		t.Fatalf("unexpected response zoom: %#v", response.Zoom)
	}
	if len(response.Sources) != 1 || response.Sources[0].Layer != "img" {
		t.Fatalf("unexpected response sources: %#v", response.Sources)
	}
	if response.Progress.Total != response.Total || response.Artifact.Status != response.ArtifactStatus {
		t.Fatalf("unified response mirrors are inconsistent: %#v", response)
	}
}

func TestArtifactDownloadGrantSupportsDirectDownloadWithoutSession(t *testing.T) {
	withTempWorkingDir(t)
	resetArtifactDownloadGrantsForTest(t)
	previousOutputDirectory := viper.GetString("output.directory")
	viper.Set("output.directory", "output")
	t.Cleanup(func() { viper.Set("output.directory", previousOutputDirectory) })

	if err := os.MkdirAll("output", directoryPermissions); err != nil {
		t.Fatal(err)
	}
	artifactPath := filepath.Join("output", "direct-download.zip")
	if err := os.WriteFile(artifactPath, []byte("artifact-data"), filePermissions); err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{
		ID:             "run-direct-download",
		TaskRecordID:   "task-direct-download",
		ArtifactPath:   artifactPath,
		ArtifactName:   "download.zip",
		ArtifactStatus: ArtifactReady,
	}
	plan := &TaskRecord{
		ID:           run.TaskRecordID,
		Kind:         TaskRecordKindSingle,
		Name:         "direct download",
		ScheduleMode: ScheduleImmediate,
		RunAt:        time.Now(),
		LastRun:      run,
	}

	response := taskResponseFromRecord(plan)
	if !strings.HasPrefix(response.DownloadURL, "/api/downloads/") {
		t.Fatalf("expected public artifact grant URL, got %q", response.DownloadURL)
	}
	if second := taskResponseFromRecord(plan).DownloadURL; second != response.DownloadURL {
		t.Fatalf("expected grant URL reuse, got %q then %q", response.DownloadURL, second)
	}

	token := strings.TrimPrefix(response.DownloadURL, "/api/downloads/")
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "token", Value: token}}
	context.Request = httptest.NewRequest(http.MethodGet, response.DownloadURL, nil)
	downloadArtifactByGrant(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected direct download status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "artifact-data" {
		t.Fatalf("unexpected artifact body %q", recorder.Body.String())
	}
	if disposition := recorder.Header().Get("Content-Disposition"); !strings.Contains(disposition, "download.zip") {
		t.Fatalf("unexpected content disposition %q", disposition)
	}
	if recorder.Header().Get("Referrer-Policy") != "no-referrer" {
		t.Fatalf("expected no-referrer policy, got %q", recorder.Header().Get("Referrer-Policy"))
	}
}

func TestExpiredArtifactDownloadGrantIsRejected(t *testing.T) {
	resetArtifactDownloadGrantsForTest(t)
	artifactDownloadGrants.Lock()
	artifactDownloadGrants.values["expired"] = artifactDownloadGrant{
		TaskID:    "task",
		RunID:     "run",
		ExpiresAt: time.Now().Add(-time.Second),
	}
	artifactDownloadGrants.byRun["task\x00run"] = "expired"
	artifactDownloadGrants.Unlock()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Params = gin.Params{{Key: "token", Value: "expired"}}
	context.Request = httptest.NewRequest(http.MethodGet, "/api/downloads/expired", nil)
	downloadArtifactByGrant(context)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected expired grant status %d, got %d", http.StatusNotFound, recorder.Code)
	}
}

func resetArtifactDownloadGrantsForTest(t *testing.T) {
	t.Helper()
	artifactDownloadGrants.Lock()
	previousValues := artifactDownloadGrants.values
	previousByRun := artifactDownloadGrants.byRun
	artifactDownloadGrants.values = make(map[string]artifactDownloadGrant)
	artifactDownloadGrants.byRun = make(map[string]string)
	artifactDownloadGrants.Unlock()
	t.Cleanup(func() {
		artifactDownloadGrants.Lock()
		artifactDownloadGrants.values = previousValues
		artifactDownloadGrants.byRun = previousByRun
		artifactDownloadGrants.Unlock()
	})
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir temp failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	})
}
