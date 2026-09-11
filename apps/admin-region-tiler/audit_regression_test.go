package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func TestTaskDoesNotPersistRetryThatLaterSucceeds(t *testing.T) {
	withTempWorkingDir(t)
	previousOutputDirectory := viper.GetString("output.directory")
	viper.Set("output.directory", "output")
	t.Cleanup(func() { viper.Set("output.directory", previousOutputDirectory) })
	var attempts int
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		attempts++
		if attempts == 1 {
			writer.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		writer.Header().Set("Content-Type", "image/png")
		_, _ = writer.Write(testPNG(t))
	}))
	defer server.Close()

	task := NewTask([]Layer{{
		URL:   server.URL + "/{z}/{x}/{y}.png",
		Zoom:  1,
		Tiles: []maptile.Tile{maptile.New(0, 0, 1)},
	}}, TileMap{Name: "retry", Format: PNG, Schema: "xyz"}, TaskOptions{
		WorkerCount:  minimumSubtaskWorkers,
		SavePipeSize: 1,
		RetryPasses:  1,
		BufferSize:   1,
		OutputFormat: "zip",
		RequestTTL:   time.Second,
		RetryBackoff: time.Millisecond,
	})
	if task == nil {
		t.Fatal("expected task")
	}

	task.Run()
	if attempts != 2 {
		t.Fatalf("expected one retry, got %d requests", attempts)
	}
	if task.currentCounts().Success != 1 || task.currentCounts().Failure != 0 {
		t.Fatalf("unexpected task counters: %#v", task.currentCounts())
	}
	if records := task.FailureRecords(); len(records) != 0 {
		t.Fatalf("successful retry must not leave a failure record: %#v", records)
	}
}

func TestSaveToFilesUsesTMSSchemaWhenRequested(t *testing.T) {
	directory := t.TempDir()
	task := &Task{File: directory, TileMap: TileMap{Format: PNG, Schema: "tms"}}
	if err := saveToFiles(Tile{T: maptile.New(0, 0, 1), C: []byte("tile")}, task); err != nil {
		t.Fatalf("save TMS tile: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "1", "0", "1.png")); err != nil {
		t.Fatalf("expected flipped TMS y path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(directory, "1", "0", "0.png")); !os.IsNotExist(err) {
		t.Fatalf("unexpected XYZ y path, stat error: %v", err)
	}
}

func TestManagedPathBoundaryRejectsOutsideAndRoot(t *testing.T) {
	root := filepath.Join(t.TempDir(), "output")
	inside := filepath.Join(root, "artifact.zip")
	if err := os.MkdirAll(root, directoryPermissions); err != nil {
		t.Fatal(err)
	}
	if _, err := ensurePathWithinRoot(inside, root, false); err != nil {
		t.Fatalf("expected managed child path: %v", err)
	}
	if _, err := ensurePathWithinRoot(root, root, false); err == nil {
		t.Fatal("managed root itself must not be removable")
	}
	if _, err := ensurePathWithinRoot(filepath.Join(filepath.Dir(root), "outside.txt"), root, false); err == nil {
		t.Fatal("outside path must be rejected")
	}
}

func TestTaskOutputNameCannotEscapeOutputDirectory(t *testing.T) {
	withTempWorkingDir(t)
	previousOutputDirectory := viper.GetString("output.directory")
	viper.Set("output.directory", "output")
	t.Cleanup(func() { viper.Set("output.directory", previousOutputDirectory) })

	task := &Task{Name: "../outside", ID: "task-id", Min: 1, Max: 1, outformat: "zip"}
	if err := task.setupOutput(); err != nil {
		t.Fatalf("set up output: %v", err)
	}
	if _, err := ensurePathWithinRoot(task.File, "output", false); err != nil {
		t.Fatalf("task output escaped its managed directory: %s (%v)", task.File, err)
	}
	if strings.Contains(filepath.Base(task.File), "..") {
		t.Fatalf("unsafe task file name: %s", task.File)
	}
}

func TestManagedTaskGeoJSONRejectsArbitraryLocalFile(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.MkdirAll("geojson", directoryPermissions); err != nil {
		t.Fatal(err)
	}
	managed := filepath.Join("geojson", "area.geojson")
	if err := os.WriteFile(managed, []byte(`{"type":"FeatureCollection","features":[]}`), filePermissions); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveManagedTaskGeoJSONPath(managed); err != nil {
		t.Fatalf("managed geojson rejected: %v", err)
	}
	outside := filepath.Join(t.TempDir(), "outside.geojson")
	if err := os.WriteFile(outside, []byte(`{"type":"FeatureCollection","features":[]}`), filePermissions); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveManagedTaskGeoJSONPath(outside); err == nil {
		t.Fatal("arbitrary local geojson must be rejected")
	}
}

func TestNormalizeTileSourceURLAcceptsHTTPAndRejectsOtherSchemes(t *testing.T) {
	valid, err := normalizeTileSourceURL(" https://tiles.example.test/{z}/{x}/{y}.png ")
	if err != nil || valid != "https://tiles.example.test/{z}/{x}/{y}.png" {
		t.Fatalf("expected normalized HTTPS tile URL, got %q, %v", valid, err)
	}
	for _, raw := range []string{"file:///sensitive.txt", "//tiles.example.test/{z}", "tiles.example.test/{z}"} {
		if _, err := normalizeTileSourceURL(raw); err == nil {
			t.Fatalf("expected invalid tile URL to be rejected: %q", raw)
		}
	}
}

func TestCreateTaskRecordsRollsBackAsOneUnit(t *testing.T) {
	store := newSQLiteTestStore(t)
	now := time.Now()
	parent := &TaskRecord{
		ID: "duplicate", UserID: 1, Kind: TaskRecordKindGroup, Name: "parent",
		URL: "https://example.test/{z}/{x}/{y}", Format: PNG, Schema: "xyz",
		ScheduleMode: ScheduleImmediate, RunAt: now, Status: TaskRecordScheduled,
		Levels: []LevelConfig{{MinZoom: 1, MaxZoom: 1, Mode: "bbox", BBox: &BBoxRequest{MinLon: 0, MinLat: 0, MaxLon: 1, MaxLat: 1}}},
	}
	child := *parent
	child.ParentID = parent.ID
	child.Kind = TaskRecordKindChild
	child.SourceName = "img"

	if err := store.createTaskRecords(parent, &child); err == nil {
		t.Fatal("expected duplicate record error")
	}
	var count int
	if err := store.db.QueryRow(`SELECT COUNT(1) FROM plans`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("expected transaction rollback, found %d plans", count)
	}
}

func TestLegacyPasswordHashUpgradesAfterSuccessfulLogin(t *testing.T) {
	store := newSQLiteTestStore(t)
	if _, err := store.db.Exec(
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		"legacy", legacyPasswordHash("secret"), time.Now().Unix(),
	); err != nil {
		t.Fatal(err)
	}
	if _, err := store.authenticateUser("legacy", "secret"); err != nil {
		t.Fatalf("authenticate legacy password: %v", err)
	}
	var storedHash string
	if err := store.db.QueryRow(`SELECT password_hash FROM users WHERE username = ?`, "legacy").Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(storedHash, "$2") {
		t.Fatalf("expected bcrypt upgrade, got %q", storedHash)
	}
}

func testPNG(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	imageValue := image.NewRGBA(image.Rect(0, 0, 1, 1))
	imageValue.Set(0, 0, color.RGBA{R: 1, G: 2, B: 3, A: 255})
	if err := png.Encode(&buffer, imageValue); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
