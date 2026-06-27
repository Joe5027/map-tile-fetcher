package main

import (
	"database/sql"
	"testing"
	"time"
)

func TestInitSchemaCreatesNormalizedTables(t *testing.T) {
	store := newSQLiteTestStore(t)

	for _, table := range []string{"tasks", "task_runs", "task_sources", "artifacts", "failures", "users", "sessions"} {
		if !sqliteTableExists(t, store.db, table) {
			t.Fatalf("expected table %s to exist", table)
		}
	}
	if !sqliteColumnExists(t, store.db, "task_runs", "task_id") {
		t.Fatal("expected task_runs.task_id migration column to exist")
	}
}

func TestCreatePlanMirrorsUnifiedTaskAndSources(t *testing.T) {
	store := newSQLiteTestStore(t)
	runAt := time.Unix(2000, 0)

	parent := &PlanRecord{
		ID:           "task-1",
		UserID:       7,
		Kind:         PlanKindGroup,
		Name:         "range task",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels: []LevelConfig{{
			MinZoom: 1,
			MaxZoom: 3,
			Geojson: "data/generated-areas/bbox-test.geojson",
		}},
	}
	child := &PlanRecord{
		ID:           "source-1",
		UserID:       7,
		ParentID:     parent.ID,
		Kind:         PlanKindChild,
		Name:         parent.Name,
		SourceName:   "天地图 img 卫星图",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels:       parent.Levels,
	}

	if err := store.createPlan(parent); err != nil {
		t.Fatalf("create parent plan failed: %v", err)
	}
	if err := store.createPlan(child); err != nil {
		t.Fatalf("create child plan failed: %v", err)
	}

	var mode string
	var zoomMin int
	var zoomMax int
	if err := store.db.QueryRow(`SELECT mode, zoom_min, zoom_max FROM tasks WHERE id = ?`, parent.ID).Scan(&mode, &zoomMin, &zoomMax); err != nil {
		t.Fatalf("read normalized task failed: %v", err)
	}
	if mode != "bbox" || zoomMin != 1 || zoomMax != 3 {
		t.Fatalf("unexpected normalized task values: mode=%s zoom=%d-%d", mode, zoomMin, zoomMax)
	}

	var taskID string
	var layer string
	if err := store.db.QueryRow(`SELECT task_id, layer FROM task_sources WHERE id = ?`, child.ID).Scan(&taskID, &layer); err != nil {
		t.Fatalf("read normalized source failed: %v", err)
	}
	if taskID != parent.ID || layer != "img" {
		t.Fatalf("unexpected normalized source: taskID=%s layer=%s", taskID, layer)
	}
}

func TestCreateRunAndFinalizeRunMirrorTaskIDAndArtifact(t *testing.T) {
	store := newSQLiteTestStore(t)
	runAt := time.Unix(2000, 0)
	parent := &PlanRecord{
		ID:           "task-2",
		UserID:       7,
		Kind:         PlanKindGroup,
		Name:         "region task",
		URL:          "https://example.test/vec/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels:       []LevelConfig{{MinZoom: 4, MaxZoom: 5, Geojson: "geojson/china.geojson"}},
	}
	child := &PlanRecord{
		ID:           "source-2",
		UserID:       7,
		ParentID:     parent.ID,
		Kind:         PlanKindChild,
		Name:         parent.Name,
		SourceName:   "天地图 vec 电子图",
		URL:          parent.URL,
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels:       parent.Levels,
	}
	if err := store.createPlan(parent); err != nil {
		t.Fatalf("create parent plan failed: %v", err)
	}
	if err := store.createPlan(child); err != nil {
		t.Fatalf("create child plan failed: %v", err)
	}

	started := time.Unix(2100, 0)
	finished := time.Unix(2200, 0)
	run := &TaskRunRecord{
		ID:             "run-1",
		PlanID:         child.ID,
		UserID:         7,
		Status:         TaskRunning,
		TriggerMode:    string(ScheduleImmediate),
		StartedAt:      &started,
		ArtifactStatus: ArtifactNone,
	}
	if err := store.createRun(run); err != nil {
		t.Fatalf("create run failed: %v", err)
	}

	var taskID string
	if err := store.db.QueryRow(`SELECT task_id FROM task_runs WHERE id = ?`, run.ID).Scan(&taskID); err != nil {
		t.Fatalf("read run task_id failed: %v", err)
	}
	if taskID != parent.ID {
		t.Fatalf("expected run task_id %s, got %s", parent.ID, taskID)
	}

	run.Status = TaskCompleted
	run.ArtifactStatus = ArtifactReady
	run.ArtifactPath = "output/task.zip"
	run.ArtifactName = "task.zip"
	run.FinishedAt = &finished
	if err := store.finalizeRun(run); err != nil {
		t.Fatalf("finalize run failed: %v", err)
	}

	var artifactStatus string
	var packageFormat string
	if err := store.db.QueryRow(`SELECT status, package_format FROM artifacts WHERE run_id = ?`, run.ID).Scan(&artifactStatus, &packageFormat); err != nil {
		t.Fatalf("read artifact failed: %v", err)
	}
	if artifactStatus != string(ArtifactReady) || packageFormat != "zip" {
		t.Fatalf("unexpected artifact mirror: status=%s package=%s", artifactStatus, packageFormat)
	}
}

func newSQLiteTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite failed: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Close()
	})

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		t.Fatalf("init schema failed: %v", err)
	}
	return store
}

func sqliteTableExists(t *testing.T, db *sql.DB, table string) bool {
	t.Helper()
	var count int
	if err := db.QueryRow(`SELECT COUNT(1) FROM sqlite_master WHERE type = 'table' AND name = ?`, table).Scan(&count); err != nil {
		t.Fatalf("table check failed: %v", err)
	}
	return count == 1
}

func sqliteColumnExists(t *testing.T, db *sql.DB, table string, column string) bool {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("column check failed: %v", err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			t.Fatalf("column scan failed: %v", err)
		}
		if name == column {
			return true
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("column rows failed: %v", err)
	}
	return false
}
