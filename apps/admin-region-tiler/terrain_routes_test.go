package main

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"tiler/internal/terrain"
)

func TestTerrainRoutesAuthorizeOwnershipAndServeGzip(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	plan := storeTerrainTestPlan(t, "https://example.test/{z}/{x}/{y}.png")
	directory, err := managedRunDirectory(plan, "published")
	if err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(directory, "terrain")
	grid := &terrain.Grid{Tile: terrain.TileID{}, Heights: make([]float64, terrain.GridSize*terrain.GridSize)}
	for i := range grid.Heights {
		grid.Heights[i] = 1000
	}
	if err := atomicTerrainWrite(filepath.Join(output, "0", "0", "0.terrain"), func(w io.Writer) error { _, err := terrain.WriteQuantizedMesh(w, grid); return err }); err != nil {
		t.Fatal(err)
	}
	manifest, err := terrain.BuildManifest("fixture", []terrain.TileID{{}}, terrain.SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	info := terrainPreviewInfo{Bounds: [4]float64{1, 1, 2, 2}, Source: defaultTerrainSource(), Expected: 1, Available: 1}
	for name, value := range map[string]any{"layer.json": manifest, "terrain-info.json": info} {
		data, _ := json.Marshal(value)
		if err := os.WriteFile(filepath.Join(output, name), data, filePermissions); err != nil {
			t.Fatal(err)
		}
	}
	run := &TaskRunRecord{ID: "published", TaskRecordID: plan.ID, UserID: 1, Status: TaskCompleted, ArtifactStatus: ArtifactReady, ArtifactPath: filepath.Join(directory, "artifact.zip")}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	// A later failed run must not hide the last published local preview.
	if err := store.createRun(&TaskRunRecord{ID: "failed", TaskRecordID: plan.ID, UserID: 1, Status: TaskFailed}); err != nil {
		t.Fatal(err)
	}
	gin.SetMode(gin.TestMode)
	router := func(user int64) *gin.Engine {
		r := gin.New()
		r.Use(func(c *gin.Context) { c.Set("user", &UserRecord{ID: user}) })
		r.GET("/api/tasks/:id/terrain-info", getTerrainInfo)
		r.GET("/api/tasks/:id/terrain/*path", getTerrainFile)
		return r
	}
	owner := router(1)
	for _, path := range []string{"/terrain-info", "/terrain/layer.json", "/terrain/0/0/0.terrain"} {
		request := httptest.NewRequest(http.MethodGet, "/api/tasks/"+plan.ID+path, nil)
		response := httptest.NewRecorder()
		owner.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("%s: %d %s", path, response.Code, response.Body.String())
		}
		if path == "/terrain/0/0/0.terrain" {
			if response.Header().Get("Content-Encoding") != "gzip" || response.Header().Get("Content-Type") != "application/vnd.quantized-mesh" {
				t.Fatalf("mesh headers: %+v", response.Header())
			}
			gz, err := gzip.NewReader(bytes.NewReader(response.Body.Bytes()))
			if err != nil {
				t.Fatal(err)
			}
			if _, err := io.Copy(io.Discard, gz); err != nil {
				t.Fatal(err)
			}
			gz.Close()
		}
		response = httptest.NewRecorder()
		router(999).ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("cross-user terrain access returned %d", response.Code)
		}
	}
	previous := viper.Get("auth.enabled")
	viper.Set("auth.enabled", true)
	t.Cleanup(func() { viper.Set("auth.enabled", previous) })
	protected := gin.New()
	protected.GET("/api/tasks/:id/terrain-info", authMiddleware(), getTerrainInfo)
	response := httptest.NewRecorder()
	protected.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/tasks/"+plan.ID+"/terrain-info", nil))
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("anonymous terrain access: %d", response.Code)
	}
}

func TestTerrainPublicPathExcludesRawCacheAndTraversal(t *testing.T) {
	for _, path := range []string{"/../terrain-info.json", "/../../tiles/0/0/0.png", "/0/0/0.png", "/0/0/1.terrain", "/16/0/0.terrain", "/0/0/00.terrain", "/layer.json/../terrain-info.json", `/0\0\0.terrain`} {
		if _, ok := terrainPublicPath(path); ok {
			t.Fatalf("accepted nonpublic terrain path %q", path)
		}
	}
	for _, path := range []string{"/layer.json", "/0/0/0.terrain", "/15/32767/32767.terrain"} {
		if _, ok := terrainPublicPath(path); !ok {
			t.Fatalf("rejected %q", path)
		}
	}
}
