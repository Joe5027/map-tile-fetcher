package main

import (
	"encoding/base64"
	"encoding/json"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
)

// This fixture also compiles against the pre-upgrade revision. Opt in because
// timing and filesystem sampling are evidence, not stable CI assertions.
func TestDownloadPerformanceFixture(t *testing.T) {
	if os.Getenv("TILER_PERFORMANCE_FIXTURE") != "1" {
		t.Skip("set TILER_PERFORMANCE_FIXTURE=1 for local performance evidence")
	}
	png, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9Y9ZlS8AAAAASUVORK5CYII=")
	for _, format := range []string{"zip", "mbtiles"} {
		t.Run(format, func(t *testing.T) {
			var connections, requests atomic.Int64
			upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				requests.Add(1)
				w.Header().Set("Content-Type", "image/png")
				_, _ = w.Write(png)
			}))
			upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
				if state == http.StateNew {
					connections.Add(1)
				}
			}
			upstream.Start()
			defer upstream.Close()
			tiles := make([]maptile.Tile, 0, 256)
			for x := uint32(0); x < 16; x++ {
				for y := uint32(0); y < 16; y++ {
					tiles = append(tiles, maptile.New(x, y, 4))
				}
			}
			directory := t.TempDir()
			task := NewTask([]Layer{{Tiles: tiles, Zoom: 4}}, TileMap{Name: "fixture", Format: PNG, Schema: "xyz", URL: upstream.URL + "/{z}/{x}/{y}", Min: 4, Max: 4}, TaskOptions{WorkerCount: 4, SavePipeSize: 2, BufferSize: 32, OutputFormat: format, RequestTTL: 5 * time.Second})
			task.File = filepath.Join(directory, "tiles")
			if format == "mbtiles" {
				task.File += ".mbtiles"
			}
			runtime.GC()
			started := time.Now()
			done := make(chan struct{})
			go func() { task.Run(); close(done) }()
			var first time.Duration
			var peakHeap, peakDisk uint64
			sample := func() {
				if first == 0 && task.currentCounts().Success > 0 {
					first = time.Since(started)
				}
				var stats runtime.MemStats
				runtime.ReadMemStats(&stats)
				if stats.HeapAlloc > peakHeap {
					peakHeap = stats.HeapAlloc
				}
				var disk uint64
				_ = filepath.WalkDir(directory, func(path string, entry fs.DirEntry, err error) error {
					if err == nil && !entry.IsDir() {
						if info, err := entry.Info(); err == nil {
							disk += uint64(info.Size())
						}
					}
					return nil
				})
				if disk > peakDisk {
					peakDisk = disk
				}
			}
			ticker := time.NewTicker(10 * time.Millisecond)
			defer ticker.Stop()
		loop:
			for {
				select {
				case <-ticker.C:
					sample()
				case <-done:
					sample()
					break loop
				}
			}
			elapsed := time.Since(started)
			if task.currentStatus() != TaskCompleted || task.currentCounts().Success != 256 || requests.Load() != 256 {
				t.Fatalf("fixture incomplete: %+v, requests=%d", task.snapshot(), requests.Load())
			}
			result := map[string]any{"format": format, "tiles": 256, "first_committed_ms": float64(first.Microseconds()) / 1000, "elapsed_ms": float64(elapsed.Microseconds()) / 1000, "connections": connections.Load(), "requests": requests.Load(), "sampled_peak_heap_bytes": peakHeap, "sampled_peak_output_bytes": peakDisk, "sample_interval_ms": 10}
			encoded, _ := json.Marshal(result)
			t.Log("PERFORMANCE " + string(encoded))
		})
	}
}
