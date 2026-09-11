package main

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// This launches the actual application binary, including its scheduler and
// ordinary child-process launcher. All tile traffic stays on a loopback server.
func TestAPILifecycle(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "tiler")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	base := fmt.Sprintf("http://127.0.0.1:%d", port)
	config := fmt.Sprintf("[app]\nport=%d\ndatabase='tasks.db'\n[auth]\nenabled=false\n[output]\ndirectory='output'\n[task]\nworkers=3\nmax_active=3\nmax_tiles=1000000\ntimedelay=0\ntime_jitter_ms=0\nmax_retries=0\nretry_passes=0\nrequest_timeout_seconds=5\n", port)
	configPath := filepath.Join(dir, "conf.toml")
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	logFile, err := os.Create(filepath.Join(dir, "server.log"))
	if err != nil {
		t.Fatal(err)
	}
	defer logFile.Close()
	client := &http.Client{Timeout: 10 * time.Second}
	var cmd *exec.Cmd
	stop := func() {
		if cmd != nil {
			_ = cmd.Process.Kill()
			_ = cmd.Wait()
			cmd = nil
		}
	}
	t.Cleanup(func() {
		// Stop queued work and signal workers before terminating their parent.
		db, err := sql.Open("sqlite", filepath.Join(dir, "data", "tasks.db"))
		if err == nil {
			_, _ = db.Exec(`DELETE FROM execution_queue; UPDATE plans SET status='cancelled' WHERE status IN ('running','paused','queued')`)
			deadline := time.Now().Add(8 * time.Second)
			for time.Now().Before(deadline) {
				var count int
				if db.QueryRow(`SELECT COUNT(*) FROM task_runs WHERE status IN ('running','paused','pending')`).Scan(&count) != nil || count == 0 {
					break
				}
				time.Sleep(100 * time.Millisecond)
			}
			db.Close()
		}
		stop()
		if t.Failed() {
			if out, err := os.ReadFile(filepath.Join(dir, "server.log")); err == nil {
				t.Logf("server log:\n%s", out)
			}
		}
	})
	start := func() {
		cmd = exec.Command(exe, "-c", configPath)
		cmd.Dir = dir
		cmd.Stdout, cmd.Stderr = logFile, logFile
		// Prevent the caller's app/auth/output settings from escaping the fixture.
		for _, entry := range os.Environ() {
			key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
			if !strings.HasPrefix(key, "APP_") && !strings.HasPrefix(key, "AUTH_") && !strings.HasPrefix(key, "OUTPUT_") && !strings.HasPrefix(key, "TASK_") {
				cmd.Env = append(cmd.Env, entry)
			}
		}
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		apiEventually(t, func() bool {
			r, err := client.Get(base + "/api/tasks")
			if err != nil {
				return false
			}
			defer r.Body.Close()
			return r.StatusCode == 200
		})
	}
	request := func(method, path string, payload any, code int, target any) []byte {
		t.Helper()
		var body io.Reader
		if payload != nil {
			raw, err := json.Marshal(payload)
			if err != nil {
				t.Fatal(err)
			}
			body = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, base+path, body)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", "application/json")
		res, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != code {
			t.Fatalf("%s %s: status %d, want %d: %s", method, path, res.StatusCode, code, raw)
		}
		if target != nil {
			if err := json.Unmarshal(raw, target); err != nil {
				t.Fatal(err)
			}
		}
		return raw
	}
	get := func(id string) TaskResponse {
		var task TaskResponse
		request("GET", "/api/tasks/"+id, nil, 200, &task)
		return task
	}
	waitStatus := func(id, status string) TaskResponse {
		var task TaskResponse
		apiEventually(t, func() bool {
			task = get(id)
			if task.Status != status {
				return false
			}
			if status == "completed" || status == "partial_failed" || status == "failed" || status == "cancelled" {
				if task.Queue.State != "" {
					return false
				}
				for _, child := range task.Children {
					if child.Queue.State != "" {
						return false
					}
				}
			}
			return true
		})
		return task
	}
	var hold, failing, failAll atomic.Bool
	var mu sync.Mutex
	counts := map[string]int{}
	png := testPNG(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counts[r.URL.Path]++
		mu.Unlock()
		for hold.Load() {
			select {
			case <-r.Context().Done():
				return
			case <-time.After(10 * time.Millisecond):
			}
		}
		if failAll.Load() || (failing.Load() && strings.HasSuffix(r.URL.Path, "/1/1/1")) {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(png)
	}))
	defer upstream.Close()
	t.Cleanup(func() { hold.Store(false) })
	payload := func(format string, sources int) CreateTaskRequest {
		req := CreateTaskRequest{Name: "same name", Mode: "bbox", Area: AreaRequest{BBox: &BBoxRequest{MinLon: -10, MaxLon: 10, MinLat: -10, MaxLat: 10}}, Zoom: &ZoomRangeRequest{Min: 1, Max: 1}, Workers: 1, Output: OutputRequest{Format: format}}
		for i := 0; i < sources; i++ {
			req.Sources = append(req.Sources, SourceRequest{Name: fmt.Sprintf("source-%d", i), URL: fmt.Sprintf("%s/s%d/{z}/{x}/{y}", upstream.URL, i), Format: "png", Schema: "xyz"})
		}
		return req
	}
	start()
	t.Run("queue_pause_resume_cancel_delete", func(t *testing.T) {
		hold.Store(true)
		var group TaskResponse
		request("POST", "/api/tasks", payload("zip", 4), 201, &group)
		if len(group.Children) != 4 {
			t.Fatalf("children: %+v", group)
		}
		apiEventually(t, func() bool { mu.Lock(); defer mu.Unlock(); return len(counts) == 3 })
		group = get(group.ID)
		var paused, queued string
		for _, child := range group.Children {
			if child.Queue.State == "queued" {
				queued = child.ID
			} else if paused == "" {
				paused = child.ID
			}
		}
		if queued == "" || paused == "" {
			t.Fatalf("expected 3 slots and 1 queued: %+v", group)
		}
		request("PUT", "/api/tasks/"+paused+"/pause", nil, 200, nil)
		waitStatus(paused, "paused")
		// The HTTP acknowledgement precedes the worker's persisted pause.
		db, err := sql.Open("sqlite", filepath.Join(dir, "data", "tasks.db"))
		if err != nil {
			t.Fatal(err)
		}
		apiEventually(t, func() bool {
			var status string
			return db.QueryRow(`SELECT r.status FROM task_runs r JOIN plans p ON p.last_run_id=r.id WHERE p.id=?`, paused).Scan(&status) == nil && status == "paused"
		})
		db.Close()
		request("DELETE", "/api/tasks/"+paused+"/purge", nil, 409, nil)
		if get(queued).Queue.State != "queued" {
			t.Fatal("pause released a slot")
		}
		hold.Store(false)
		waitStatus(queued, "completed")
		if get(paused).Status != "paused" {
			t.Fatal("paused task ran to completion")
		}
		request("PUT", "/api/tasks/"+paused+"/resume", nil, 200, nil)
		waitStatus(group.ID, "completed")
		for _, child := range get(group.ID).Children {
			raw := request("GET", "/api/tasks/"+child.ID+"/download", nil, 200, nil)
			archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
			if err != nil || len(archive.File) != 4 {
				t.Fatalf("artifact: %v", err)
			}
		}
		// Cancel a real worker blocked on the local upstream and then purge it.
		hold.Store(true)
		var cancelled TaskResponse
		request("POST", "/api/tasks", payload("zip", 1), 201, &cancelled)
		request("DELETE", "/api/tasks/"+cancelled.ID, nil, 200, nil)
		waitStatus(cancelled.ID, "cancelled")
		hold.Store(false)
		// Worker monitor releases the slot after persisting terminal status.
		time.Sleep(200 * time.Millisecond)
		request("DELETE", "/api/tasks/"+cancelled.ID+"/purge", nil, 200, nil)
		request("DELETE", "/api/tasks/"+group.ID+"/purge", nil, 200, nil)
		request("GET", "/api/tasks/"+group.ID, nil, 404, nil)
	})
	t.Run("retry_and_missing_baseline", func(t *testing.T) {
		for _, format := range []string{"zip", "mbtiles"} {
			failing.Store(true)
			var created TaskResponse
			request("POST", "/api/tasks", payload(format, 1), 201, &created)
			waitStatus(created.ID, "partial_failed")
			childID := created.Children[0].ID
			before := request("GET", "/api/tasks/"+childID+"/download", nil, 200, nil)
			request("POST", "/api/tasks/"+created.ID+"/retry-failures", nil, 202, nil)
			waitStatus(created.ID, "partial_failed")
			failing.Store(false)
			request("POST", "/api/tasks/"+created.ID+"/retry-failures", nil, 202, nil)
			complete := waitStatus(created.ID, "completed")
			if complete.Integrity.Available != 4 || complete.FailureCount != 0 {
				t.Fatalf("incomplete retry: %+v", complete)
			}
			after := request("GET", "/api/tasks/"+childID+"/download", nil, 200, nil)
			if bytes.Equal(before, after) {
				t.Fatal("retry did not replace published artifact")
			}
			request("POST", "/api/tasks/"+created.ID+"/retry-failures", nil, 409, nil)
			request("POST", "/api/tasks/"+created.ID+"/reconcile", nil, 202, nil)
			apiEventually(t, func() bool { return get(created.ID).Integrity.Status == "complete" })
			request("DELETE", "/api/tasks/"+created.ID+"/purge", nil, 200, nil)
		}
		failing.Store(true)
		var missing TaskResponse
		request("POST", "/api/tasks", payload("zip", 1), 201, &missing)
		waitStatus(missing.ID, "partial_failed")
		db, err := sql.Open("sqlite", filepath.Join(dir, "data", "tasks.db"))
		if err != nil {
			t.Fatal(err)
		}
		var output, artifact string
		err = db.QueryRow(`SELECT output_path,artifact_path FROM task_runs WHERE plan_id=?`, missing.Children[0].ID).Scan(&output, &artifact)
		db.Close()
		if err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{output, artifact} {
			if !filepath.IsAbs(path) {
				path = filepath.Join(dir, path)
			}
			if err := os.RemoveAll(path); err != nil {
				t.Fatal(err)
			}
		}
		request("POST", "/api/tasks/"+missing.ID+"/retry-failures", nil, 409, nil)
		failing.Store(false)
		var recreated TaskResponse
		request("POST", "/api/tasks/"+missing.ID+"/recreate", nil, 201, &recreated)
		waitStatus(recreated.ID, "completed")
		request("DELETE", "/api/tasks/"+missing.ID+"/purge", nil, 200, nil)
		request("DELETE", "/api/tasks/"+recreated.ID+"/purge", nil, 200, nil)
	})
	t.Run("scheduled_restart", func(t *testing.T) {
		req := payload("zip", 1)
		req.ScheduleMode = ScheduleOnce
		req.RunAt = time.Now().Add(4 * time.Second).Format(time.RFC3339)
		var task TaskResponse
		request("POST", "/api/tasks", req, 201, &task)
		if task.Children[0].Queue.State != "queued" {
			t.Fatalf("not queued: %+v", task)
		}
		stop()
		start()
		waitStatus(task.ID, "completed")
		request("DELETE", "/api/tasks/"+task.ID+"/purge", nil, 200, nil)
	})
	t.Run("all_failed_can_retry_without_old_successes", func(t *testing.T) {
		failAll.Store(true)
		var task TaskResponse
		request("POST", "/api/tasks", payload("zip", 1), 201, &task)
		waitStatus(task.ID, "failed")
		failAll.Store(false)
		request("POST", "/api/tasks/"+task.ID+"/retry-failures", nil, 202, nil)
		complete := waitStatus(task.ID, "completed")
		if complete.Integrity.Available != 4 {
			t.Fatalf("all-failed retry: %+v", complete)
		}
		request("DELETE", "/api/tasks/"+task.ID+"/purge", nil, 200, nil)
	})
	var tasks []TaskResponse
	request("GET", "/api/tasks", nil, 200, &tasks)
	if len(tasks) != 0 {
		t.Fatalf("remaining tasks: %+v", tasks)
	}
	db, err := sql.Open("sqlite", filepath.Join(dir, "data", "tasks.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"plans", "tasks", "task_runs", "task_sources", "failures", "artifacts", "run_coverage", "task_integrity", "pending_publications", "execution_queue", "task_deletions", "deletion_paths"} {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("cleanup %s: %d %v", table, count, err)
		}
	}
}

func apiEventually(t *testing.T, predicate func() bool) {
	t.Helper()
	deadline := time.Now().Add(25 * time.Second)
	for time.Now().Before(deadline) {
		if predicate() {
			return
		}
		time.Sleep(80 * time.Millisecond)
	}
	t.Fatal("condition not met within 25 seconds")
}
