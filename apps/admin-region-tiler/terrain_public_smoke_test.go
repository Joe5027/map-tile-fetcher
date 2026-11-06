package main

import (
	"bytes"
	"context"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"tiler/internal/terrain"
)

// Explicit, single-request public-service smoke. It is never part of the
// offline test suite and does not create a task or persist downloaded data.
func TestTerrainPublicRootSmoke(t *testing.T) {
	if os.Getenv("REAL_TERRARIUM_SMOKE") != "1" {
		t.Skip("set REAL_TERRARIUM_SMOKE=1 to request exactly one public root PNG")
	}
	client := &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	url := strings.NewReplacer("{z}", "0", "{x}", "0", "{y}", "0").Replace(defaultTerrariumURL)
	response, err := client.Get(url)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("root source HTTP status %d", response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, 8*1024*1024+1))
	if err != nil {
		t.Fatal(err)
	}
	if len(data) > 8*1024*1024 {
		t.Fatal("root PNG exceeds 8 MiB")
	}
	raster, err := terrain.DecodeTerrarium(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	minimum, maximum := math.Inf(1), math.Inf(-1)
	for _, height := range raster.Heights {
		minimum = min(minimum, height)
		maximum = max(maximum, height)
	}
	grid, err := terrain.SampleGrid(context.Background(), terrain.TileID{}, func(context.Context, terrain.TileID) (*terrain.Raster, error) { return raster, nil })
	if err != nil {
		t.Fatal(err)
	}
	var mesh bytes.Buffer
	stats, err := terrain.WriteQuantizedMesh(&mesh, grid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("one public GET: status=%d bytes=%d sourceHeight=[%.3f,%.3f] meshHeight=[%.3f,%.3f] gzipMeshBytes=%d horizonDisabled=%t", response.StatusCode, len(data), minimum, maximum, stats.MinimumHeight, stats.MaximumHeight, mesh.Len(), stats.HorizonDisabled)
}
