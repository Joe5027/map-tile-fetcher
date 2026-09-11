package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func setTestOutput(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	previous := viper.GetString("output.directory")
	viper.Set("output.directory", root)
	t.Cleanup(func() { viper.Set("output.directory", previous) })
	return root
}

func TestPublicationFailurePreservesPreviousArtifact(t *testing.T) {
	useTestStore(t)
	root := setTestOutput(t)
	p := storedBBoxTask(t, "published-task", "")
	old := &TaskRunRecord{ID: "old", TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactReady, ArtifactPath: filepath.Join(root, "old.zip")}
	if err := store.createRun(old); err != nil {
		t.Fatal(err)
	}
	current := &TaskRunRecord{ID: "current", TaskRecordID: p.ID, UserID: 1, Status: TaskRunning, ArtifactStatus: ArtifactNone}
	if err := store.createRun(current); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_artifact BEFORE INSERT ON artifacts BEGIN SELECT RAISE(ABORT,'disk transaction failure'); END`); err != nil {
		t.Fatal(err)
	}
	current.Status = TaskCompleted
	current.ArtifactStatus = ArtifactReady
	current.ArtifactPath = filepath.Join(root, "current.zip")
	if err := store.finalizeRun(current); err == nil {
		t.Fatal("expected publication transaction to fail")
	}
	published, err := store.publishedRun(p.ID)
	if err != nil || published.ID != old.ID {
		t.Fatalf("previous publication lost: %+v %v", published, err)
	}
	actual, err := store.getRun(current.ID)
	if err != nil || actual.ArtifactStatus == ArtifactReady {
		t.Fatalf("partial publication persisted: %+v %v", actual, err)
	}
}

func TestPurgeSharedPolygonAndInterruptedCleanup(t *testing.T) {
	useTestStore(t)
	setTestOutput(t)
	areaDir := filepath.Join(defaultDataDir, "generated-areas")
	if err := os.MkdirAll(areaDir, 0755); err != nil {
		t.Fatal(err)
	}
	file, err := os.CreateTemp(areaDir, "purge-test-*.geojson")
	if err != nil {
		t.Fatal(err)
	}
	path := file.Name()
	file.Close()
	t.Cleanup(func() { os.Remove(path) })
	first := storedBBoxTask(t, "first-child", "")
	second := storedBBoxTask(t, "second-child", "")
	for _, p := range []*TaskRecord{first, second} {
		p.Levels = []LevelConfig{{Geojson: path, MinZoom: 1, MaxZoom: 1}}
		data, _ := json.Marshal(p.Levels)
		if _, err := store.db.Exec(`UPDATE plans SET levels_json = ? WHERE id = ?`, string(data), p.ID); err != nil {
			t.Fatal(err)
		}
	}
	m := NewRuntimeManager()
	if err := m.Purge(first); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("shared polygon was removed: %v", err)
	}
	if _, err := store.db.Exec(`CREATE TRIGGER reject_delete BEFORE DELETE ON plans BEGIN SELECT RAISE(ABORT,'interrupted cleanup'); END`); err != nil {
		t.Fatal(err)
	}
	if err := m.Purge(second); err == nil {
		t.Fatal("expected interrupted database deletion")
	}
	if err := store.checkNotDeleting(second.ID); err == nil {
		t.Fatal("pending cleanup can be scheduled")
	}
	if _, err := store.getTaskRecordByID(second.ID); err != nil {
		t.Fatal("cleanup lost task record", err)
	}
	if _, err := store.db.Exec(`DROP TRIGGER reject_delete`); err != nil {
		t.Fatal(err)
	}
	if err := m.Purge(second); err != nil {
		t.Fatal(err)
	}
	if err := m.Purge(second); err != nil {
		t.Fatal("repeated deletion", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("last polygon reference leaked: %v", err)
	}
}

func TestManagedPathRejectsSymlinkEscape(t *testing.T) {
	root := setTestOutput(t)
	outside := t.TempDir()
	link := filepath.Join(root, "escape")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symlink creation unavailable: %v", err)
	}
	if _, err := ensurePathWithinRoot(filepath.Join(link, "file"), root, false); err == nil {
		t.Fatal("accepted a symlink escape")
	}
}

func useTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	previous := store
	store = newSQLiteTestStore(t)
	t.Cleanup(func() { store = previous })
	return store
}

func storedBBoxTask(t *testing.T, id, parent string) *TaskRecord {
	t.Helper()
	p := &TaskRecord{ID: id, ParentID: parent, UserID: 1, Name: "same name", SourceName: "img", Kind: TaskRecordKindSingle,
		URL: "http://tiles.example.test/{z}/{x}/{y}", Format: PNG, Schema: "xyz", ScheduleMode: ScheduleImmediate,
		RunAt: time.Now(), Status: TaskRecordCompleted,
		Levels: []LevelConfig{{MinZoom: 1, MaxZoom: 1, Mode: "bbox", BBox: &BBoxRequest{MinLon: 1, MaxLon: 2, MinLat: 1, MaxLat: 2}, OutputFormat: "zip"}}}
	if parent != "" {
		p.Kind = TaskRecordKindChild
	}
	if err := store.createTaskRecord(p); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSameNameArchivesNeverOverwrite(t *testing.T) {
	useTestStore(t)
	root := t.TempDir()
	previous := viper.GetString("output.directory")
	viper.Set("output.directory", root)
	t.Cleanup(func() { viper.Set("output.directory", previous) })
	p := storedBBoxTask(t, "source", "")
	paths := make(map[string]bool)
	for _, id := range []string{"first-run", "second-run"} {
		dir := filepath.Join(root, id)
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "tile.png"), testPNG(t), 0644); err != nil {
			t.Fatal(err)
		}
		run := &TaskRunRecord{ID: id, TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted}
		if err := store.createRun(run); err != nil {
			t.Fatal(err)
		}
		task := &Task{File: dir, outformat: "zip", Status: TaskCompleted}
		if err := prepareArtifactForRun(p, task, run); err != nil {
			t.Fatal(err)
		}
		if paths[run.ArtifactPath] {
			t.Fatalf("runs share archive path: %s", run.ArtifactPath)
		}
		paths[run.ArtifactPath] = true
	}
}

func TestPurgeGroupRemovesChildOutput(t *testing.T) {
	useTestStore(t)
	root := t.TempDir()
	previous := viper.GetString("output.directory")
	viper.Set("output.directory", root)
	t.Cleanup(func() { viper.Set("output.directory", previous) })
	group := storedBBoxTask(t, "group", "")
	group.Kind = TaskRecordKindGroup
	if _, err := store.db.Exec("UPDATE plans SET kind = 'group' WHERE id = ?", group.ID); err != nil {
		t.Fatal(err)
	}
	child := storedBBoxTask(t, "child", group.ID)
	output := filepath.Join(root, "child-output")
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "run", TaskRecordID: child.ID, UserID: 1, Status: TaskCompleted, OutputPath: output}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := NewRuntimeManager().Purge(group); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(output); !os.IsNotExist(err) {
		t.Fatalf("child output still exists: %v", err)
	}
}
