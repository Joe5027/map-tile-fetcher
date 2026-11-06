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

type recoveryTestApp struct {
	t              *testing.T
	dir, exe, base string
	cmd            *exec.Cmd
	log            *os.File
	client         *http.Client
}

func newRecoveryTestApp(t *testing.T, exe string) *recoveryTestApp {
	t.Helper()
	dir := t.TempDir()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	config := fmt.Sprintf("[app]\nport=%d\ndatabase='tasks.db'\n[auth]\nenabled=false\n[output]\ndirectory='output'\n[task]\nmax_active=1\nmax_tiles=10000\nworkers=1\ntime_jitter_ms=0\nmax_retries=0\nretry_passes=0\nrequest_timeout_seconds=5\n", port)
	if err := os.WriteFile(filepath.Join(dir, "conf.toml"), []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	log, err := os.Create(filepath.Join(dir, "app.log"))
	if err != nil {
		t.Fatal(err)
	}
	app := &recoveryTestApp{t: t, dir: dir, exe: exe, base: fmt.Sprintf("http://127.0.0.1:%d", port), log: log, client: &http.Client{Timeout: 5 * time.Second}}
	t.Cleanup(func() {
		app.stop()
		log.Close()
		if t.Failed() {
			data, _ := os.ReadFile(filepath.Join(dir, "app.log"))
			t.Log(string(data))
		}
	})
	return app
}

func (a *recoveryTestApp) start() {
	a.t.Helper()
	a.cmd = exec.Command(a.exe, "-c", filepath.Join(a.dir, "conf.toml"))
	a.cmd.Dir = a.dir
	a.cmd.Stdout = a.log
	a.cmd.Stderr = a.log
	for _, entry := range os.Environ() {
		key := strings.ToUpper(strings.SplitN(entry, "=", 2)[0])
		if !strings.HasPrefix(key, "APP_") && !strings.HasPrefix(key, "AUTH_") && !strings.HasPrefix(key, "OUTPUT_") && !strings.HasPrefix(key, "TASK_") && !strings.HasPrefix(key, "TILER_WORKER_") {
			a.cmd.Env = append(a.cmd.Env, entry)
		}
	}
	if err := a.cmd.Start(); err != nil {
		a.t.Fatal(err)
	}
	apiEventually(a.t, func() bool {
		r, err := a.client.Get(a.base + "/api/tasks")
		if err != nil {
			return false
		}
		defer r.Body.Close()
		return r.StatusCode == 200
	})
}

func (a *recoveryTestApp) stop() {
	if a.cmd != nil {
		_ = a.cmd.Process.Kill()
		_ = a.cmd.Wait()
		a.cmd = nil
	}
}

func (a *recoveryTestApp) request(method, path string, payload any, code int, target any) {
	a.t.Helper()
	var body io.Reader
	if payload != nil {
		raw, err := json.Marshal(payload)
		if err != nil {
			a.t.Fatal(err)
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, a.base+path, body)
	if err != nil {
		a.t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := a.client.Do(req)
	if err != nil {
		a.t.Fatal(err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(res.Body)
	if err != nil {
		a.t.Fatal(err)
	}
	if res.StatusCode != code {
		a.t.Fatalf("%s %s status=%d want=%d: %s", method, path, res.StatusCode, code, raw)
	}
	if target != nil {
		if err := json.Unmarshal(raw, target); err != nil {
			a.t.Fatal(err)
		}
	}
}

func (a *recoveryTestApp) task(id string) TaskResponse {
	var task TaskResponse
	a.request("GET", "/api/tasks/"+id, nil, 200, &task)
	return task
}
func (a *recoveryTestApp) waitStatus(id, status string) TaskResponse {
	a.t.Helper()
	var task TaskResponse
	apiEventually(a.t, func() bool {
		task = a.task(id)
		if status == "completed" && task.Status == "failed" {
			a.t.Fatalf("task failed while awaiting completion: %+v", task)
		}
		if task.Status != status {
			return false
		}
		if status == "completed" {
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

func (a *recoveryTestApp) database() *sql.DB {
	a.t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(a.dir, "data", "tasks.db")+"?_pragma=busy_timeout(5000)")
	if err != nil {
		a.t.Fatal(err)
	}
	return db
}

func recoveryPayload(url, format string, zoom int) CreateTaskRequest {
	return CreateTaskRequest{Name: "recovery", Mode: "bbox", Area: AreaRequest{BBox: &BBoxRequest{MinLon: -179, MaxLon: 179, MinLat: -80, MaxLat: 80}}, Zoom: &ZoomRangeRequest{Min: zoom, Max: zoom}, Workers: 1, Sources: []SourceRequest{{Name: "local", URL: url + "/{z}/{x}/{y}", Format: PNG, Schema: "xyz"}}, Output: OutputRequest{Format: format}}
}

func TestProcessRestartResumesCommittedTilesAndPausedQueue(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "tiler")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if out, err := exec.CommandContext(ctx, "go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	t.Run("duplicate_server_does_not_claim_database", func(t *testing.T) { testDuplicateServerOwnership(t, exe) })
	for _, format := range []string{"zip", "mbtiles"} {
		t.Run(format, func(t *testing.T) {
			var slow atomic.Bool
			slow.Store(true)
			var mu sync.Mutex
			counts := map[string]int{}
			png := testPNG(t)
			upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				counts[r.URL.Path]++
				mu.Unlock()
				if slow.Load() {
					select {
					case <-r.Context().Done():
						return
					case <-time.After(100 * time.Millisecond):
					}
				}
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(png)
			}))
			defer upstream.Close()
			app := newRecoveryTestApp(t, exe)
			app.start()
			var created TaskResponse
			app.request("POST", "/api/tasks", recoveryPayload(upstream.URL, format, 3), 201, &created)
			if len(created.Children) != 1 {
				t.Fatalf("children: %+v", created)
			}
			child := created.Children[0].ID
			db := app.database()
			defer db.Close()
			var runID, output string
			apiEventually(t, func() bool {
				var successes int64
				err := db.QueryRow(`SELECT r.id,r.output_path,r.success_count FROM plans p JOIN task_runs r ON r.id=p.last_run_id WHERE p.id=?`, child).Scan(&runID, &output, &successes)
				return err == nil && successes >= 3 && successes < 64
			})
			app.stop()
			if !filepath.IsAbs(output) {
				output = filepath.Join(app.dir, output)
			}
			lockFile, err := os.OpenFile(filepath.Join(filepath.Dir(output), "worker.lock"), os.O_RDWR, 0600)
			if err != nil {
				t.Fatal(err)
			}
			apiEventually(t, func() bool { return lockMaintenanceFile(lockFile, true) == nil })
			saved := map[string]int{}
			if format == "zip" {
				err = filepath.Walk(output, func(path string, info os.FileInfo, err error) error {
					if err != nil {
						return err
					}
					if info.IsDir() || filepath.Ext(path) != ".png" {
						return nil
					}
					relative, err := filepath.Rel(output, path)
					if err != nil {
						return err
					}
					key := "/" + strings.TrimSuffix(filepath.ToSlash(relative), ".png")
					mu.Lock()
					saved[key] = counts[key]
					mu.Unlock()
					return nil
				})
			} else {
				var tiles *sql.DB
				tiles, err = sql.Open("sqlite", output+"?mode=ro")
				if err == nil {
					var rows *sql.Rows
					rows, err = tiles.Query(`SELECT zoom_level,tile_column,tile_row FROM tiles`)
					if err == nil {
						for rows.Next() {
							var z, x, y int
							if err = rows.Scan(&z, &x, &y); err != nil {
								break
							}
							key := fmt.Sprintf("/%d/%d/%d", z, x, (1<<z)-1-y)
							mu.Lock()
							saved[key] = counts[key]
							mu.Unlock()
						}
						if err == nil {
							err = rows.Err()
						}
						rows.Close()
					}
					tiles.Close()
				}
			}
			lockFile.Close()
			if err != nil {
				t.Fatal(err)
			}
			if len(saved) == 0 || len(saved) >= 64 {
				t.Fatalf("need partial committed output, have %d tiles", len(saved))
			}
			slow.Store(false)
			app.start()
			complete := app.waitStatus(created.ID, "completed")
			var resumedID string
			if err := db.QueryRow(`SELECT last_run_id FROM plans WHERE id=?`, child).Scan(&resumedID); err != nil {
				t.Fatal(err)
			}
			if resumedID != runID {
				t.Fatalf("run replaced: %s -> %s", runID, resumedID)
			}
			if len(complete.Children) != 1 || complete.Children[0].Integrity.Available != 64 {
				t.Fatalf("incomplete recovered output: %+v", complete)
			}
			mu.Lock()
			for key, before := range saved {
				if counts[key] != before {
					t.Errorf("saved tile %s requested again: %d -> %d", key, before, counts[key])
				}
			}
			mu.Unlock()
			app.request("DELETE", "/api/tasks/"+created.ID+"/purge", nil, 200, nil)
			for _, table := range []string{"task_budgets", "run_phases"} {
				var count int
				if err := db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
					t.Fatalf("purge %s: %d %v", table, count, err)
				}
			}
		})
	}
	t.Run("packing_restart_excludes_partial_files", func(t *testing.T) {
		png := testPNG(t)
		var requests atomic.Int64
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			requests.Add(1)
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		}))
		defer upstream.Close()
		app := newRecoveryTestApp(t, exe)
		app.start()
		db := app.database()
		defer db.Close()
		if _, err := db.Exec(`CREATE TABLE packing_write_count(n INTEGER); INSERT INTO packing_write_count VALUES(0); CREATE TRIGGER count_packing_updates AFTER UPDATE ON run_phases WHEN NEW.phase='packing' BEGIN UPDATE packing_write_count SET n=n+1; END`); err != nil {
			t.Fatal(err)
		}
		payload := recoveryPayload(upstream.URL, "zip", 5)
		payload.Workers = 4
		var created TaskResponse
		app.request("POST", "/api/tasks", payload, 201, &created)
		child := created.Children[0].ID
		var runID, output string
		var expected int64
		deadline := time.Now().Add(90 * time.Second)
		packing := false
		for time.Now().Before(deadline) {
			var current, total int64
			err := db.QueryRow(`SELECT r.id,r.output_path,r.total,ph.current,ph.total FROM plans p JOIN task_runs r ON r.id=p.last_run_id JOIN run_phases ph ON ph.run_id=r.id WHERE p.id=? AND ph.phase='packing'`, child).Scan(&runID, &output, &expected, &current, &total)
			if err == nil && total > 0 && current < total {
				outputPath := output
				if !filepath.IsAbs(outputPath) {
					outputPath = filepath.Join(app.dir, outputPath)
				}
				if info, statErr := os.Stat(filepath.Join(filepath.Dir(outputPath), "artifact.zip.partial")); statErr == nil && info.Size() > 0 {
					packing = true
					break
				}
			}
			time.Sleep(time.Millisecond)
		}
		if !packing {
			t.Fatal("did not observe actual unfinished ZIP packaging")
		}
		app.stop()
		if !filepath.IsAbs(output) {
			output = filepath.Join(app.dir, output)
		}
		lock, err := os.OpenFile(filepath.Join(filepath.Dir(output), "worker.lock"), os.O_RDWR, 0600)
		if err != nil {
			t.Fatal(err)
		}
		apiEventually(t, func() bool { return lockMaintenanceFile(lock, true) == nil })
		for _, name := range []string{"0.png.partial", "mesh.terrain.partial"} {
			if err := os.WriteFile(filepath.Join(output, name), []byte("unfinished atomic write"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		before := requests.Load()
		lock.Close()
		recoveryStarted := time.Now()
		app.start()
		app.waitStatus(created.ID, "completed")
		var packingWrites int
		if err := db.QueryRow(`SELECT n FROM packing_write_count`).Scan(&packingWrites); err != nil {
			t.Fatal(err)
		}
		if int64(packingWrites) >= expected/4 {
			t.Fatalf("packaging persists per-file progress: %d writes for %d files", packingWrites, expected)
		}
		t.Logf("recovered %d already-downloaded files and repacked in %s; %d packing progress updates across both workers", expected, time.Since(recoveryStarted), packingWrites)
		var resumed string
		if err := db.QueryRow(`SELECT last_run_id FROM plans WHERE id=?`, child).Scan(&resumed); err != nil {
			t.Fatal(err)
		}
		if resumed != runID || requests.Load() != before {
			t.Fatalf("pack recovery repeated download/run: %s/%s requests=%d/%d", runID, resumed, before, requests.Load())
		}
		response, err := app.client.Get(app.base + "/api/tasks/" + child + "/download")
		if err != nil {
			t.Fatal(err)
		}
		raw, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil {
			t.Fatal(err)
		}
		if response.StatusCode != 200 {
			t.Fatalf("download %d: %s", response.StatusCode, raw)
		}
		archive, err := zip.NewReader(bytes.NewReader(raw), int64(len(raw)))
		if err != nil {
			t.Fatal(err)
		}
		if int64(len(archive.File)) != expected {
			t.Fatalf("archive contains %d files; expected %d", len(archive.File), expected)
		}
		for _, file := range archive.File {
			if strings.HasSuffix(file.Name, ".partial") {
				t.Fatalf("published temporary output %s", file.Name)
			}
		}
		app.request("DELETE", "/api/tasks/"+created.ID+"/purge", nil, 200, nil)
	})
	t.Run("paused_releases_slot_and_survives_restart", func(t *testing.T) {
		var slow, hold atomic.Bool
		slow.Store(true)
		png := testPNG(t)
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			for hold.Load() && strings.HasPrefix(r.URL.Path, "/blocking/") {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(10 * time.Millisecond):
				}
			}
			if slow.Load() && strings.HasPrefix(r.URL.Path, "/paused/") {
				select {
				case <-r.Context().Done():
					return
				case <-time.After(100 * time.Millisecond):
				}
			}
			w.Header().Set("Content-Type", "image/png")
			_, _ = w.Write(png)
		}))
		defer upstream.Close()
		defer hold.Store(false)
		app := newRecoveryTestApp(t, exe)
		app.start()
		var first TaskResponse
		app.request("POST", "/api/tasks", recoveryPayload(upstream.URL+"/paused", "zip", 3), 201, &first)
		child := first.Children[0].ID
		db := app.database()
		defer db.Close()
		apiEventually(t, func() bool {
			var n int64
			return db.QueryRow(`SELECT r.success_count FROM plans p JOIN task_runs r ON r.id=p.last_run_id WHERE p.id=?`, child).Scan(&n) == nil && n >= 2
		})
		app.request("PUT", "/api/tasks/"+child+"/pause", nil, 200, nil)
		apiEventually(t, func() bool {
			var status string
			return db.QueryRow(`SELECT r.status FROM plans p JOIN task_runs r ON r.id=p.last_run_id WHERE p.id=?`, child).Scan(&status) == nil && status == "paused"
		})
		var second TaskResponse
		app.request("POST", "/api/tasks", recoveryPayload(upstream.URL+"/other", "zip", 1), 201, &second)
		app.waitStatus(second.ID, "completed")
		app.stop()
		app.start()
		paused := app.task(child)
		if paused.Status != "paused" || paused.Queue.State != "paused" {
			t.Fatalf("paused intent lost: %+v", paused)
		}
		hold.Store(true)
		var blocker TaskResponse
		app.request("POST", "/api/tasks", recoveryPayload(upstream.URL+"/blocking", "zip", 1), 201, &blocker)
		app.waitStatus(blocker.ID, "running")
		app.request("PUT", "/api/tasks/"+child+"/resume", nil, 200, nil)
		waiting := app.task(child)
		if waiting.Status != "paused" || waiting.Queue.State != "queued" || waiting.Queue.Position < 1 {
			t.Fatalf("resume exceeded capacity: %+v", waiting)
		}
		slow.Store(false)
		hold.Store(false)
		app.waitStatus(blocker.ID, "completed")
		app.waitStatus(first.ID, "completed")
		for _, id := range []string{first.ID, second.ID, blocker.ID} {
			app.request("DELETE", "/api/tasks/"+id+"/purge", nil, 200, nil)
		}
	})
}
