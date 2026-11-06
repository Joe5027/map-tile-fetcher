package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func testDuplicateServerOwnership(t *testing.T, exe string) {
	app := newRecoveryTestApp(t, exe)
	app.start()
	rejectSecond := func() {
		t.Helper()
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		port := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		second := exec.Command(exe, "-c", filepath.Join(app.dir, "conf.toml"))
		second.Dir = app.dir
		second.Env = append(append([]string{}, app.cmd.Env...), "APP_PORT="+strconv.Itoa(port))
		var output bytes.Buffer
		second.Stdout, second.Stderr = &output, &output
		if err := second.Start(); err != nil {
			t.Fatal(err)
		}
		done := make(chan error, 1)
		go func() { done <- second.Wait() }()
		select {
		case err := <-done:
			if err == nil || !strings.Contains(output.String(), "already owned by another server") {
				t.Fatalf("second server exited without ownership rejection: %v\n%s", err, output.String())
			}
		case <-time.After(5 * time.Second):
			_ = second.Process.Kill()
			<-done
			t.Fatal("second server remained active against the first server's database")
		}
	}
	// Without active runs the worker locks cannot incidentally reject this process.
	rejectSecond()
	png := testPNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
			return
		case <-time.After(30 * time.Millisecond):
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer upstream.Close()
	var created TaskResponse
	app.request("POST", "/api/tasks", recoveryPayload(upstream.URL, "zip", 3), 201, &created)
	db := app.database()
	defer db.Close()
	var before string
	apiEventually(t, func() bool {
		return db.QueryRow(`SELECT owner_token FROM execution_queue WHERE plan_id=?`, created.Children[0].ID).Scan(&before) == nil && before != ""
	})
	rejectSecond()
	var after string
	if err := db.QueryRow(`SELECT owner_token FROM execution_queue WHERE plan_id=?`, created.Children[0].ID).Scan(&after); err != nil || before != after {
		t.Fatalf("second server changed worker ownership: %q -> %q, %v", before, after, err)
	}
	app.waitStatus(created.ID, "completed")
}

func TestInvalidPendingPublicationKeepsPreviousArtifactAndRawResume(t *testing.T) {
	for _, broken := range []string{"missing", "corrupt"} {
		t.Run(broken, func(t *testing.T) {
			useTestStore(t)
			root := setTestOutput(t)
			plan := storedBBoxTask(t, "broken-publish", "")
			task, err := buildTaskFromRecord(plan)
			if err != nil {
				t.Fatal(err)
			}
			defer task.cancel()
			rawDir := filepath.Join(root, "raw")
			task.File = rawDir
			if err := task.setupOutput(); err != nil {
				t.Fatal(err)
			}
			png := testPNG(t)
			if err := task.forEachExpected(func(job TileJob) error { return task.saveTile(Tile{T: job.Tile, C: png, URL: job.URL}) }); err != nil {
				t.Fatal(err)
			}
			old := &TaskRunRecord{ID: "old-ready", TaskRecordID: plan.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactReady, ArtifactPath: filepath.Join(root, "old.zip")}
			if err := zipDirectory(rawDir, old.ArtifactPath, nil); err != nil {
				t.Fatal(err)
			}
			if err := store.createRun(old); err != nil {
				t.Fatal(err)
			}
			current := &TaskRunRecord{ID: "broken-pending", TaskRecordID: plan.ID, UserID: 1, Status: TaskRunning, ArtifactStatus: ArtifactNone, OutputPath: rawDir}
			if err := store.createRun(current); err != nil {
				t.Fatal(err)
			}
			if err := store.markTaskRecordRunning(plan.ID, current.ID); err != nil {
				t.Fatal(err)
			}
			current.Status, current.ArtifactStatus, current.ArtifactPath = TaskCompleted, ArtifactReady, filepath.Join(root, "broken.zip")
			if broken == "corrupt" {
				if err := os.WriteFile(current.ArtifactPath, []byte("incomplete ZIP"), 0600); err != nil {
					t.Fatal(err)
				}
			}
			queuePublication := func(run *TaskRunRecord) {
				t.Helper()
				record, _ := json.Marshal(run)
				state, _ := json.Marshal(IntegrityState{Status: "complete", RunID: run.ID})
				if _, err := store.db.Exec(`INSERT INTO pending_publications VALUES(?,?,?)`, run.ID, string(record), string(state)); err != nil {
					t.Fatal(err)
				}
			}
			queuePublication(current)
			other := storedBBoxTask(t, "valid-publish", "")
			valid := &TaskRunRecord{ID: "valid-pending", TaskRecordID: other.ID, UserID: 1, Status: TaskRunning, ArtifactStatus: ArtifactNone, OutputPath: rawDir}
			if err := store.createRun(valid); err != nil {
				t.Fatal(err)
			}
			if err := store.markTaskRecordRunning(other.ID, valid.ID); err != nil {
				t.Fatal(err)
			}
			valid.Status, valid.ArtifactStatus, valid.ArtifactPath = TaskCompleted, ArtifactReady, old.ArtifactPath
			queuePublication(valid)
			if err := store.recoverInterruptedTaskRecords(); err != nil {
				t.Fatalf("one broken publication blocked service recovery: %v", err)
			}
			published, err := store.publishedRun(plan.ID)
			if err != nil || published.ID != old.ID {
				t.Fatalf("previous publication lost: %+v %v", published, err)
			}
			if err := validateArchive(published.ArtifactPath); err != nil {
				t.Fatal(err)
			}
			published, err = store.publishedRun(other.ID)
			if err != nil || published.ID != valid.ID {
				t.Fatalf("unrelated publication blocked: %+v %v", published, err)
			}
			var pending int
			if err := store.db.QueryRow(`SELECT COUNT(*) FROM pending_publications`).Scan(&pending); err != nil || pending != 0 {
				t.Fatalf("pending publications=%d %v", pending, err)
			}
			var trigger string
			if err := store.db.QueryRow(`SELECT trigger_mode FROM execution_queue WHERE plan_id=? AND state='queued'`, plan.ID).Scan(&trigger); err != nil || trigger != triggerResumeRun {
				t.Fatalf("raw recovery not queued: %q %v", trigger, err)
			}
			cleanup, err := prepareResumeJobs(plan, task, current)
			if err != nil {
				t.Fatal(err)
			}
			defer cleanup()
			requests := 0
			if err := task.retrySource(func(TileJob) error { requests++; return nil }); err != nil || requests != 0 {
				t.Fatalf("valid raw would be downloaded again: %d %v", requests, err)
			}
		})
	}
}
