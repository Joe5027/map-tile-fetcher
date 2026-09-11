package main

import (
	"archive/zip"
	"context"
	"database/sql"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/viper"
)

func TestWorkerSubprocess(t *testing.T) {
	if os.Getenv("TILER_TEST_WORKER") != "1" {
		return
	}
	db, err := sql.Open("sqlite", os.Getenv("TILER_TEST_DB"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	store = &SQLiteStore{db: db}
	viper.Set("output.directory", os.Getenv("TILER_TEST_OUTPUT"))
	viper.Set("task.max_retries", 1)
	viper.Set("task.retry_backoff_ms", 1)
	viper.Set("task.slow_backoff_ms", 1)
	viper.Set("task.request_timeout_seconds", 2)
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	if err := runWorkerProcess(os.Getenv("TILER_TEST_PLAN"), os.Getenv("TILER_TEST_RUN")); err != nil {
		t.Fatal(err)
	}
}

func useFileTestStore(t *testing.T) {
	t.Helper()
	previous := store
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	store = &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close(); store = previous })
}

func executeStoredWorker(t *testing.T, plan *TaskRecord, id, trigger string) *TaskRunRecord {
	t.Helper()
	dir, err := managedRunDirectory(plan, id)
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "tiles")
	if plan.Levels[0].OutputFormat == "mbtiles" {
		output = filepath.Join(dir, "tiles.mbtiles")
	}
	run := &TaskRunRecord{ID: id, TaskRecordID: plan.ID, UserID: 1, Status: TaskRunning, TriggerMode: trigger, ArtifactStatus: ArtifactNone, OutputPath: output}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := store.markTaskRecordRunning(plan.ID, id); err != nil {
		t.Fatal(err)
	}
	var seq int
	var name, dbPath string
	if err := store.db.QueryRow(`PRAGMA database_list`).Scan(&seq, &name, &dbPath); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, exe, "-test.run=^TestWorkerSubprocess$", "-test.v")
	cmd.Env = append(os.Environ(), "TILER_TEST_WORKER=1", "TILER_TEST_DB="+dbPath, "TILER_TEST_OUTPUT="+viper.GetString("output.directory"), "TILER_TEST_PLAN="+plan.ID, "TILER_TEST_RUN="+id)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("worker: %v\n%s", err, output)
	}
	run, err = store.getRun(id)
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func TestRetryPublishesCompleteCumulativeArtifact(t *testing.T) {
	for _, format := range []string{"zip", "mbtiles"} {
		t.Run(format, func(t *testing.T) {
			useFileTestStore(t)
			setTestOutput(t)
			var failing atomic.Bool
			failing.Store(true)
			var requests atomic.Int32
			png := testPNG(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if failing.Load() && r.URL.Path == "/1/1/1" {
					w.WriteHeader(503)
					return
				}
				w.Header().Set("Content-Type", "image/png")
				w.Write(png)
			}))
			defer upstream.Close()
			p := storedBBoxTask(t, "retry-"+format, "")
			p.URL = upstream.URL + "/{z}/{x}/{y}"
			p.Levels = []LevelConfig{{MinZoom: 1, MaxZoom: 1, Mode: "bbox", BBox: &BBoxRequest{MinLon: -10, MaxLon: 10, MinLat: -10, MaxLat: 10}, OutputFormat: format}}
			if _, err := store.db.Exec(`UPDATE plans SET url=?,levels_json=? WHERE id=?`, p.URL, `[{"minZoom":1,"maxZoom":1,"mode":"bbox","bbox":{"minLon":-10,"maxLon":10,"minLat":-10,"maxLat":10},"outputFormat":"`+format+`"}]`, p.ID); err != nil {
				t.Fatal(err)
			}
			first := executeStoredWorker(t, p, "initial-"+format, "immediate")
			if first.Status != TaskPartialFailed || first.SuccessCount != 3 || first.FailureCount != 1 {
				t.Fatalf("initial run %+v", first)
			}
			if first.ArtifactStatus != ArtifactReady {
				t.Fatalf("partial baseline unpublished: %+v", first)
			}
			failing.Store(false)
			requests.Store(0)
			second := executeStoredWorker(t, p, "retry-"+format, triggerRetryFailures)
			if second.Status != TaskCompleted || second.Total != 1 || second.SuccessCount != 1 {
				t.Fatalf("retry run %+v", second)
			}
			if requests.Load() != 1 {
				t.Fatalf("retry fetched %d tiles, expected one", requests.Load())
			}
			if second.ArtifactPath == first.ArtifactPath {
				t.Fatal("overwrote previous artifact")
			}
			state := store.integrityState(p.ID)
			if state.Status != "complete" || state.Expected != 4 || state.Available != 4 {
				t.Fatalf("cumulative state %+v", state)
			}
			summary, err := store.failureSummary(p.ID)
			if err != nil || summary.Total != 0 {
				t.Fatalf("unresolved failures %+v %v", summary, err)
			}
			history, err := store.listFailureHistory(p.ID, true)
			if err != nil || len(history) != 1 || history[0].ResolvedRunID != second.ID {
				t.Fatalf("history %+v %v", history, err)
			}
			reader, err := openTileOutput(p, second)
			if err != nil {
				t.Fatal(err)
			}
			reader.Close()
			if format == "zip" {
				archive, err := zip.OpenReader(second.ArtifactPath)
				if err != nil {
					t.Fatal(err)
				}
				defer archive.Close()
				names := map[string]bool{}
				for _, f := range archive.File {
					names[f.Name] = true
				}
				for _, name := range []string{"1/0/0.png", "1/0/1.png", "1/1/0.png", "1/1/1.png"} {
					if !names[name] {
						t.Fatalf("missing coordinate %s", name)
					}
				}
				if len(names) != 4 {
					t.Fatalf("unexpected coordinate set %v", names)
				}
			} else {
				db, err := sql.Open("sqlite", second.ArtifactPath)
				if err != nil {
					t.Fatal(err)
				}
				defer db.Close()
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM tiles WHERE zoom_level=1 AND tile_column IN (0,1) AND tile_row IN (0,1)`).Scan(&count); err != nil || count != 4 {
					t.Fatalf("artifact coordinates %d %v", count, err)
				}
			}
			if err := store.initSchema(); err != nil {
				t.Fatal("repeat migration", err)
			}
		})
	}
}

func TestDuplicateFailuresAndMissingBaseline(t *testing.T) {
	useTestStore(t)
	root := setTestOutput(t)
	p := storedBBoxTask(t, "dedup", "")
	for _, id := range []string{"a", "b"} {
		run := &TaskRunRecord{ID: id, TaskRecordID: p.ID, UserID: 1, Status: TaskFailed, ArtifactStatus: ArtifactNone}
		if err := store.createRun(run); err != nil {
			t.Fatal(err)
		}
		if err := store.replaceFailureRecords(run, []TileFailureRecord{{Z: 1, X: 1, Y: 0, URL: "http://tiles.example.test/1/1/0", Retryable: true}}); err != nil {
			t.Fatal(err)
		}
	}
	summary, err := store.failureSummary(p.ID)
	if err != nil || summary.Total != 1 {
		t.Fatalf("duplicates counted %+v %v", summary, err)
	}
	if _, err := store.validateRetryBaseline(p); !errors.Is(err, errMissingBaseline) {
		t.Fatalf("missing baseline: %v", err)
	}
	_ = root
}

func TestPublicationRecoveryDoesNotRedownload(t *testing.T) {
	useTestStore(t)
	root := setTestOutput(t)
	p := storedBBoxTask(t, "recover", "")
	output := filepath.Join(root, "saved")
	if err := os.MkdirAll(output, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(output, "tile.png"), testPNG(t), 0644); err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "recover-run", TaskRecordID: p.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactNone, OutputPath: output}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	if err := prepareArtifactForRun(p, &Task{File: output, Status: TaskCompleted, outformat: "zip"}, run); err != nil {
		t.Fatal(err)
	}
	run.ArtifactStatus = ArtifactReady
	if _, err := store.db.Exec(`CREATE TRIGGER reject_publish BEFORE INSERT ON artifacts BEGIN SELECT RAISE(ABORT,'publication interrupted'); END`); err != nil {
		t.Fatal(err)
	}
	if err := store.publishRun(run, IntegrityState{Status: "complete", Expected: 1, Available: 1, RunID: run.ID}); err == nil {
		t.Fatal("expected publication failure")
	}
	if _, err := store.publishedRun(p.ID); err == nil {
		t.Fatal("uncommitted artifact visible")
	}
	if _, err := store.db.Exec(`DROP TRIGGER reject_publish`); err != nil {
		t.Fatal(err)
	}
	if err := store.recoverInterruptedTaskRecords(); err != nil {
		t.Fatal(err)
	}
	published, err := store.publishedRun(p.ID)
	if err != nil || published.ID != run.ID {
		t.Fatalf("publication did not recover: %+v %v", published, err)
	}
	due, err := store.listDueTaskRecords(time.Now().Add(time.Hour))
	if err != nil || len(due) != 0 {
		t.Fatalf("recovery queued a download: %v %v", due, err)
	}
}

func TestCancelledRunDoesNotPublish(t *testing.T) {
	useTestStore(t)
	root := setTestOutput(t)
	p := storedBBoxTask(t, "cancelled", "")
	run := &TaskRunRecord{ID: "cancelled-run", TaskRecordID: p.ID, Status: TaskCancelled, OutputPath: root}
	if err := prepareArtifactForRun(p, &Task{File: root, Status: TaskCancelled}, run); err != nil {
		t.Fatal(err)
	}
	if run.ArtifactPath != "" {
		t.Fatal("cancelled output was published")
	}
}
