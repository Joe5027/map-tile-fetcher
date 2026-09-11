package main

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/maptile"
)

func TestLegacyDatabaseCopyMigration(t *testing.T) {
	useFileTestStore(t)
	p := storedBBoxTask(t, "legacy", "")
	run := &TaskRunRecord{ID: "legacy-run", TaskRecordID: p.ID, UserID: 1, Status: TaskFailed, ArtifactStatus: ArtifactNone}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.replaceFailureRecords(run, []TileFailureRecord{{Z: 1, X: 1, Y: 0, URL: p.URL, Retryable: true}}); err != nil {
		t.Fatal(err)
	}
	// Emulate the pre-repair failure schema, then migrate a database copy.
	for _, statement := range []string{
		`DROP INDEX idx_failure_coordinates`,
		`ALTER TABLE failures DROP COLUMN resolved_at`,
		`ALTER TABLE failures DROP COLUMN resolved_run_id`,
		`DROP TABLE task_integrity`, `DROP TABLE run_coverage`, `DROP TABLE pending_publications`, `DROP TABLE execution_queue`,
	} {
		if _, err := store.db.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	copyPath := filepath.Join(t.TempDir(), "legacy-copy.db")
	if _, err := store.db.Exec(`VACUUM INTO ?`, copyPath); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("sqlite", copyPath)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	copyStore := &SQLiteStore{db: db}
	for i := 0; i < 2; i++ {
		if err := copyStore.initSchema(); err != nil {
			t.Fatal(err)
		}
	}
	if err := copyStore.recoverInterruptedTaskRecords(); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM failures WHERE resolved_at=0 AND resolved_run_id=''`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("history lost: %d %v", count, err)
	}
	if err := db.QueryRow(`SELECT COUNT(*) FROM execution_queue`).Scan(&count); err != nil || count != 0 {
		t.Fatal("migration started downloads", err)
	}
	if _, err := copyStore.getTaskRecordByID(p.ID); err != nil {
		t.Fatal(err)
	}
	columns, err := store.tableColumnSet("failures")
	if err != nil || columns["resolved_at"] {
		t.Fatal("migration modified original", err)
	}
}

func TestCancellationBeforePublicationKeepsOldArtifact(t *testing.T) {
	useTestStore(t)
	root := setTestOutput(t)
	p := storedBBoxTask(t, "publish-cancel", "")
	output := filepath.Join(root, "tiles")
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "tile.png"), testPNG(t), 0644); err != nil {
		t.Fatal(err)
	}
	old := &TaskRunRecord{ID: "old", TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactReady, ArtifactPath: filepath.Join(root, "old.zip")}
	if err := zipDirectory(output, old.ArtifactPath, nil); err != nil {
		t.Fatal(err)
	}
	if err := store.createRun(old); err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "new", TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactNone, OutputPath: output}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := prepareArtifactForRun(p, &Task{File: output, Status: TaskCompleted, outformat: "zip"}, run); err != nil {
		t.Fatal(err)
	}
	run.ArtifactStatus = ArtifactReady
	if err := store.updateTaskRecordStatus(p.ID, TaskRecordCancelled); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRun(run, IntegrityState{Status: "complete", Expected: 1, Available: 1}); err != nil {
		t.Fatal(err)
	}
	latest, err := store.publishedRun(p.ID)
	if err != nil || latest.ID != old.ID {
		t.Fatal("cancel replaced baseline", err)
	}
	var pending int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM pending_publications`).Scan(&pending); err != nil || pending != 0 {
		t.Fatal("cancelled publication is recoverable", err)
	}
}

func TestCancelledCopyAndGeometryEnumeration(t *testing.T) {
	useTestStore(t)
	setTestOutput(t)
	p := storedBBoxTask(t, "copy-cancel", "")
	task, err := buildTaskFromRecord(p)
	if err != nil {
		t.Fatal(err)
	}
	task.workerManaged = true
	if err := task.Cancel(); err != nil {
		t.Fatal(err)
	}
	if _, err := store.validateRetryBaselineWithTask(p, task); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel validation: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	layer := Layer{Zoom: 9, Collection: orb.Collection{orb.Polygon{{{-170, -80}, {170, -80}, {170, 80}, {-170, 80}, {-170, -80}}}}}
	started := time.Now()
	visits := 0
	err = walkLayer(ctx, layer, func(maptile.Tile) error { visits++; cancel(); return nil })
	if !errors.Is(err, context.Canceled) || visits != 1 || time.Since(started) > 2*time.Second {
		t.Fatalf("geometry cancellation: visits=%d err=%v elapsed=%s", visits, err, time.Since(started))
	}
}

func TestHistoricalFailureOutsideCoverageCannotComplete(t *testing.T) {
	useTestStore(t)
	setTestOutput(t)
	p := storedBBoxTask(t, "history-gap", "")
	run := &TaskRunRecord{ID: "gap-run", TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactNone}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.replaceFailureRecords(run, []TileFailureRecord{{Z: 9, X: 0, Y: 0, URL: "http://old.example/9/0/0", Retryable: true}}); err != nil {
		t.Fatal(err)
	}
	if err := store.commitPublication(run, IntegrityState{Status: "complete", Expected: 1, Available: 1}); err != nil {
		t.Fatal(err)
	}
	if run.Status != TaskPartialFailed || store.integrityState(p.ID).Status == "complete" {
		t.Fatal("unresolved history was marked complete")
	}
}
