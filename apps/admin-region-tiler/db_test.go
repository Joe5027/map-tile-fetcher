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

	parent := &TaskRecord{
		ID:           "task-1",
		UserID:       7,
		Kind:         TaskRecordKindGroup,
		Name:         "range task",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels: []LevelConfig{{
			MinZoom: 1,
			MaxZoom: 3,
			Geojson: "data/generated-areas/bbox-test.geojson",
		}},
	}
	child := &TaskRecord{
		ID:           "source-1",
		UserID:       7,
		ParentID:     parent.ID,
		Kind:         TaskRecordKindChild,
		Name:         parent.Name,
		SourceName:   "天地图 img 卫星图",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       parent.Levels,
	}

	if err := store.createTaskRecord(parent); err != nil {
		t.Fatalf("create parent plan failed: %v", err)
	}
	if err := store.createTaskRecord(child); err != nil {
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
	parent := &TaskRecord{
		ID:           "task-2",
		UserID:       7,
		Kind:         TaskRecordKindGroup,
		Name:         "region task",
		URL:          "https://example.test/vec/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       []LevelConfig{{MinZoom: 4, MaxZoom: 5, Geojson: "geojson/china.geojson"}},
	}
	child := &TaskRecord{
		ID:           "source-2",
		UserID:       7,
		ParentID:     parent.ID,
		Kind:         TaskRecordKindChild,
		Name:         parent.Name,
		SourceName:   "天地图 vec 电子图",
		URL:          parent.URL,
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       parent.Levels,
	}
	if err := store.createTaskRecord(parent); err != nil {
		t.Fatalf("create parent plan failed: %v", err)
	}
	if err := store.createTaskRecord(child); err != nil {
		t.Fatalf("create child plan failed: %v", err)
	}

	started := time.Unix(2100, 0)
	finished := time.Unix(2200, 0)
	run := &TaskRunRecord{
		ID:             "run-1",
		TaskRecordID:   child.ID,
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

func TestReplaceFailureRecordsPersistsRetryableTiles(t *testing.T) {
	store := newSQLiteTestStore(t)
	runAt := time.Unix(2000, 0)
	plan := &TaskRecord{
		ID:           "task-3",
		UserID:       7,
		Kind:         TaskRecordKindSingle,
		Name:         "failure task",
		SourceName:   "天地图 img 卫星图",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       []LevelConfig{{MinZoom: 1, MaxZoom: 1, Geojson: "data/generated-areas/bbox-test.geojson"}},
	}
	if err := store.createTaskRecord(plan); err != nil {
		t.Fatalf("create plan failed: %v", err)
	}
	run := &TaskRunRecord{
		ID:             "run-2",
		TaskRecordID:   plan.ID,
		UserID:         7,
		Status:         TaskRunning,
		TriggerMode:    string(ScheduleImmediate),
		ArtifactStatus: ArtifactNone,
	}
	if err := store.createRun(run); err != nil {
		t.Fatalf("create run failed: %v", err)
	}

	records := []TileFailureRecord{{
		Z:            1,
		X:            2,
		Y:            3,
		URL:          "https://example.test/img/1/2/3.png",
		ErrorMessage: "temporary upstream failure",
		Retryable:    true,
		Attempt:      1,
		CreatedAt:    time.Unix(2200, 0),
	}}
	if err := store.replaceFailureRecords(run, records); err != nil {
		t.Fatalf("replace failure records failed: %v", err)
	}
	if err := store.replaceFailureRecords(run, records); err != nil {
		t.Fatalf("second replace failure records failed: %v", err)
	}

	var count int
	var retryable int
	var attempt int
	if err := store.db.QueryRow(`SELECT COUNT(1), MAX(retryable), MAX(attempt) FROM failures WHERE run_id = ?`, run.ID).Scan(&count, &retryable, &attempt); err != nil {
		t.Fatalf("read failure records failed: %v", err)
	}
	if count != 1 || retryable != 1 || attempt != 1 {
		t.Fatalf("unexpected failure record values: count=%d retryable=%d attempt=%d", count, retryable, attempt)
	}

	listed, err := store.listFailureRecords(plan.ID)
	if err != nil {
		t.Fatalf("list failure records failed: %v", err)
	}
	if len(listed) != 1 || !listed[0].Retryable || listed[0].Z != 1 || listed[0].X != 2 || listed[0].Y != 3 {
		t.Fatalf("unexpected listed failure records: %#v", listed)
	}
}

func TestFailureRecordsAreScopedByChildSource(t *testing.T) {
	store := newSQLiteTestStore(t)
	runAt := time.Unix(2000, 0)
	parent := &TaskRecord{
		ID:           "task-4",
		UserID:       7,
		Kind:         TaskRecordKindGroup,
		Name:         "multi-source task",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       []LevelConfig{{MinZoom: 1, MaxZoom: 1, Geojson: "data/generated-areas/bbox-test.geojson"}},
	}
	childA := &TaskRecord{
		ID:           "source-a",
		UserID:       7,
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
	childB := *childA
	childB.ID = "source-b"
	childB.SourceName = "cia"
	childB.URL = "https://example.test/cia/{z}/{x}/{y}.png"

	for _, plan := range []*TaskRecord{parent, childA, &childB} {
		if err := store.createTaskRecord(plan); err != nil {
			t.Fatalf("create plan %s failed: %v", plan.ID, err)
		}
	}

	runA := &TaskRunRecord{ID: "run-a", TaskRecordID: childA.ID, UserID: 7, Status: TaskRunning, TriggerMode: string(ScheduleImmediate)}
	runB := &TaskRunRecord{ID: "run-b", TaskRecordID: childB.ID, UserID: 7, Status: TaskRunning, TriggerMode: string(ScheduleImmediate)}
	if err := store.createRun(runA); err != nil {
		t.Fatalf("create run A failed: %v", err)
	}
	if err := store.createRun(runB); err != nil {
		t.Fatalf("create run B failed: %v", err)
	}
	if err := store.replaceFailureRecords(runA, []TileFailureRecord{{
		Z: 1, X: 2, Y: 3, URL: "https://example.test/img/1/2/3.png", ErrorMessage: "retry", Retryable: true,
	}}); err != nil {
		t.Fatalf("replace failures A failed: %v", err)
	}
	if err := store.replaceFailureRecords(runB, []TileFailureRecord{{
		Z: 1, X: 4, Y: 5, URL: "https://example.test/cia/1/4/5.png", ErrorMessage: "final", Retryable: false,
	}}); err != nil {
		t.Fatalf("replace failures B failed: %v", err)
	}

	parentSummary, err := store.failureSummary(parent.ID)
	if err != nil {
		t.Fatalf("parent summary failed: %v", err)
	}
	if parentSummary.Total != 2 || parentSummary.Retryable != 1 {
		t.Fatalf("unexpected parent summary: %#v", parentSummary)
	}

	childSummary, err := store.failureSummary(childA.ID)
	if err != nil {
		t.Fatalf("child summary failed: %v", err)
	}
	if childSummary.Total != 1 || childSummary.Retryable != 1 {
		t.Fatalf("unexpected child summary: %#v", childSummary)
	}

	retryable, err := store.listRetryableFailureRecords(childA.ID)
	if err != nil {
		t.Fatalf("retryable child records failed: %v", err)
	}
	if len(retryable) != 1 || retryable[0].SourceID != childA.ID {
		t.Fatalf("unexpected retryable child records: %#v", retryable)
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
