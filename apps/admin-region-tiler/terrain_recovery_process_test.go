package main

import (
	"archive/zip"
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestProcessRestartDuringTerrainConversion(t *testing.T) {
	exe := filepath.Join(t.TempDir(), "tiler")
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if output, err := exec.CommandContext(ctx, "go", "build", "-o", exe, ".").CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, output)
	}
	// Opaque, signed, varying Terrarium elevations. No timing hooks or sleeps
	// are added to production conversion to make the fault window observable.
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			value := 32768 + x*2 + y*3 - 350
			img.SetNRGBA(x, y, color.NRGBA{R: byte(value >> 8), G: byte(value), A: 255})
		}
	}
	var fixture bytes.Buffer
	if err := png.Encode(&fixture, img); err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int64
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(fixture.Bytes())
	}))
	defer upstream.Close()
	app := newRecoveryTestApp(t, exe)
	app.start()
	payload := recoveryPayload(upstream.URL, "cesium", 4)
	payload.Area.BBox = &BBoxRequest{MinLon: -179.99, MaxLon: 179.99, MinLat: -85, MaxLat: 85}
	payload.Sources[0].Metadata = defaultTerrainSource()
	payload.Workers = 4
	var created TaskResponse
	app.request("POST", "/api/tasks", payload, 201, &created)
	if len(created.Children) != 1 {
		t.Fatalf("children: %+v", created)
	}
	child := created.Children[0].ID
	db := app.database()
	defer db.Close()
	var runID, rawOutput string
	converting := false
	deadline := time.Now().Add(45 * time.Second)
	for time.Now().Before(deadline) {
		var current, total int64
		err := db.QueryRow(`SELECT r.id,r.output_path,ph.current,ph.total FROM plans p JOIN task_runs r ON r.id=p.last_run_id JOIN run_phases ph ON ph.run_id=r.id WHERE p.id=? AND ph.phase='converting'`, child).Scan(&runID, &rawOutput, &current, &total)
		if err == nil && total == 341 && current > 0 && current < total/2 {
			converting = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !converting {
		t.Fatal("did not observe first half of actual 341-mesh conversion")
	}
	app.stop() // Kill the real server, letting its existing parent watcher stop the worker.
	if !filepath.IsAbs(rawOutput) {
		rawOutput = filepath.Join(app.dir, rawOutput)
	}
	lock, err := os.OpenFile(filepath.Join(filepath.Dir(rawOutput), "worker.lock"), os.O_RDWR, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	apiEventually(t, func() bool { return lockMaintenanceFile(lock, true) == nil })
	// Exclusive acquisition proves the old worker has released its run before
	// restart. Snapshot its completed meshes while no writer can race us.
	meshes := make(map[string]time.Time)
	meshDirectory := filepath.Join(filepath.Dir(rawOutput), "terrain")
	if err := filepath.Walk(meshDirectory, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() && strings.HasSuffix(path, ".terrain") && validTerrainMesh(path) {
			meshes[path] = info.ModTime()
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if len(meshes) == 0 || len(meshes) >= 341 {
		t.Fatalf("need partial converted output at fault, got %d meshes", len(meshes))
	}
	before := requests.Load()
	if before != 341 {
		t.Fatalf("conversion began without the expected raw catalogue: %d requests", before)
	}
	if err := lock.Close(); err != nil {
		t.Fatal(err)
	}
	app.start()
	completed := app.waitStatus(created.ID, "completed")
	var resumed string
	if err := db.QueryRow(`SELECT last_run_id FROM plans WHERE id=?`, child).Scan(&resumed); err != nil {
		t.Fatal(err)
	}
	if resumed != runID || requests.Load() != before {
		t.Fatalf("conversion recovery replaced run/downloaded raw again: %s/%s requests=%d/%d", runID, resumed, before, requests.Load())
	}
	if len(completed.Children) != 1 || completed.Children[0].Integrity.Available != 341 || completed.Children[0].Integrity.Missing != 0 {
		t.Fatalf("recovered terrain coverage: %+v", completed)
	}
	for path, modified := range meshes {
		info, err := os.Stat(path)
		if err != nil || !info.ModTime().Equal(modified) {
			t.Fatalf("completed mesh was rewritten after restart: %s %v", path, err)
		}
	}
	response, err := app.client.Get(app.base + "/api/tasks/" + child + "/download")
	if err != nil {
		t.Fatal(err)
	}
	data, err := io.ReadAll(response.Body)
	response.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("download status %d: %s", response.StatusCode, data)
	}
	archive, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, entry := range archive.File {
		if strings.HasSuffix(entry.Name, ".terrain") {
			count++
		}
		if strings.HasSuffix(entry.Name, ".png") || strings.HasSuffix(entry.Name, ".partial") {
			t.Fatalf("raw/temporary file published: %s", entry.Name)
		}
	}
	if count != 341 {
		t.Fatalf("published %d meshes, want 341", count)
	}
	t.Logf("killed during conversion; reused %d completed meshes, preserved run and all %d raw requests", len(meshes), before)
}
