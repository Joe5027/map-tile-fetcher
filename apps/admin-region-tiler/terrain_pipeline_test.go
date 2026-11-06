package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
	"tiler/internal/terrain"
)

func terrainTestPNG(t *testing.T, height float64) []byte {
	t.Helper()
	img := image.NewNRGBA(image.Rect(0, 0, 256, 256))
	value := int(math.Round((height + 32768) * 256))
	c := color.NRGBA{R: byte(value >> 16), G: byte(value >> 8), B: byte(value), A: 255}
	for y := 0; y < 256; y++ {
		for x := 0; x < 256; x++ {
			img.SetNRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func terrainTestRequest(url string) CreateTaskRequest {
	return CreateTaskRequest{Name: "local terrain", Mode: "bbox", Area: AreaRequest{BBox: &BBoxRequest{MinLon: 1, MinLat: 1, MaxLon: 2, MaxLat: 2}}, Zoom: &ZoomRangeRequest{Min: 2, Max: 2}, Output: OutputRequest{Format: "cesium"}, Workers: 2, Sources: []SourceRequest{{Name: "fixture", URL: url, Format: PNG, Schema: "xyz", Metadata: defaultTerrainSource()}}}
}

func storeTerrainTestPlan(t *testing.T, sourceURL string) *TaskRecord {
	t.Helper()
	parent, children, err := buildTaskRecordsFromRequest(1, terrainTestRequest(sourceURL))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.createTaskRecords(append([]*TaskRecord{parent}, children...)...); err != nil {
		t.Fatal(err)
	}
	return children[0]
}

func TestTerrainRequestMetadataAndCompatibility(t *testing.T) {
	req := terrainTestRequest(defaultTerrariumURL)
	req.Sources = nil
	_, children, err := buildTaskRecordsFromRequest(1, req)
	if err != nil {
		t.Fatal(err)
	}
	if children[0].URL != defaultTerrariumURL || children[0].Levels[0].Source.Encoding != "terrarium" {
		t.Fatal("default elevation source not configured")
	}
	req = terrainTestRequest(defaultTerrariumURL)
	req.Sources = append(req.Sources, req.Sources[0])
	parent, children, err := buildTaskRecordsFromRequest(1, req)
	if err != nil {
		t.Fatal(err)
	}
	children[0].Levels[0].Source.Encoding = "changed"
	children[0].Levels[0].BBox.MinLon = -100
	if children[1].Levels[0].Source.Encoding != "terrarium" || children[1].Levels[0].BBox.MinLon != 1 || parent.Levels[0].BBox.MinLon != 1 {
		t.Fatal("child source/area metadata shares mutable state")
	}
	for _, bad := range []SourceRequest{{URL: defaultTerrariumURL, Format: JPG, Schema: "xyz", Metadata: defaultTerrainSource()}, {URL: defaultTerrariumURL, Format: PNG, Schema: "tms", Metadata: defaultTerrainSource()}, {URL: "https://example.test/image/{z}/{x}/{y}.png", Format: PNG, Schema: "xyz"}} {
		req = terrainTestRequest(defaultTerrariumURL)
		req.Sources = []SourceRequest{bad}
		if _, _, err := buildTaskRecordsFromRequest(1, req); err == nil {
			t.Fatalf("accepted non-Terrarium source: %+v", bad)
		}
	}
	req = terrainTestRequest(defaultTerrariumURL)
	req.Zoom.Max = 16
	if _, _, err := buildTaskRecordsFromRequest(1, req); err == nil {
		t.Fatal("accepted unsupported DEM zoom")
	}
}

func TestTerrainLevelZeroRequestAndConfiguredSource(t *testing.T) {
	c, _ := gin.CreateTestContext(httptest.NewRecorder())
	c.Request = httptest.NewRequest(http.MethodPost, "/api/tasks", strings.NewReader(`{"name":"root terrain","levels":[{"minZoom":0,"maxZoom":0,"mode":"bbox","bbox":{"minLon":1,"minLat":1,"maxLon":2,"maxLat":2}}],"output":{"format":"cesium"}}`))
	c.Request.Header.Set("Content-Type", "application/json")
	var request CreateTaskRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		t.Fatalf("valid level zero rejected by binding: %v", err)
	}
	if _, _, err := buildTaskRecordsFromRequest(1, request); err != nil {
		t.Fatal(err)
	}
	config := viper.New()
	config.SetConfigFile("conf.toml")
	if err := config.ReadInConfig(); err != nil {
		t.Fatal(err)
	}
	var sources []struct {
		ID       int
		Metadata *SourceMetadata
	}
	if err := config.UnmarshalKey("tilemaps", &sources); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, source := range sources {
		if source.ID == 13 {
			found = true
			if source.Metadata == nil || source.Metadata.Projection != "EPSG:3857" || source.Metadata.Encoding != "terrarium" || source.Metadata.MaxZoom != 15 || source.Metadata.DefaultZoom != 14 {
				t.Fatalf("invalid configured terrain metadata: %+v", source.Metadata)
			}
		}
	}
	if !found {
		t.Fatal("configured public terrain source missing")
	}
}

func TestTerrainMixedLevelURLRejectedBeforePersistence(t *testing.T) {
	useFileTestStore(t)
	request := terrainTestRequest(defaultTerrariumURL)
	box := request.Area.BBox
	request.Mode = ""
	request.Area = AreaRequest{}
	request.Zoom = nil
	request.Levels = []LevelRequest{{MinZoom: 2, MaxZoom: 2, Mode: "bbox", BBox: box, URL: "https://another.example.test/{z}/{x}/{y}.png"}}
	response := taskCreationRequest(t, request)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "same source URL") {
		t.Fatalf("mixed terrain source request: %d %s", response.Code, response.Body.String())
	}
	for _, table := range []string{"plans", "tasks", "task_sources", "task_runs", "execution_queue"} {
		var count int
		if err := store.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("rejected terrain request left %d %s rows", count, table)
		}
	}
	request.Output.Format = "zip"
	if _, _, err := buildTaskRecordsFromRequest(1, request); err != nil {
		t.Fatalf("ordinary PNG level URL compatibility changed: %v", err)
	}
}

func TestTerrainExportDoesNotLeakSourceURLCredentials(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	const sourceURL = "https://source-user:source-password@terrain.example.test/{z}/{x}/{y}.png?token=source-secret#private-fragment"
	request := terrainTestRequest(sourceURL)
	request.Sources[0].Name = ""
	parent, children, err := buildTaskRecordsFromRequest(1, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.createTaskRecords(append([]*TaskRecord{parent}, children...)...); err != nil {
		t.Fatal(err)
	}
	plan := children[0]
	if strings.Contains(plan.SourceName, "source-secret") || strings.Contains(plan.SourceName, "source-password") {
		t.Fatal("default source label still contains URL credentials")
	}
	// Also cover already persisted legacy/custom names that are themselves URLs.
	plan.SourceName = sourceURL
	task, err := buildTaskFromRecord(plan)
	if err != nil {
		t.Fatal(err)
	}
	defer task.cancel()
	directory, err := managedRunDirectory(plan, "source-privacy")
	if err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "source-privacy", OutputPath: filepath.Join(directory, "tiles")}
	data := terrainTestPNG(t, 123.5)
	if err := task.forEachExpected(func(job TileJob) error {
		path := filepath.Join(run.OutputPath, fmt.Sprint(job.Tile.Z), fmt.Sprint(job.Tile.X), fmt.Sprintf("%d.png", job.Tile.Y))
		return atomicTerrainWrite(path, func(w io.Writer) error { _, err := w.Write(data); return err })
	}); err != nil {
		t.Fatal(err)
	}
	if err := deriveTerrainArtifacts(plan, task, run); err != nil {
		t.Fatal(err)
	}
	if plan.URL != sourceURL {
		t.Fatal("request credentials were removed from the download URL")
	}
	for _, name := range []string{"layer.json", "README.txt", "terrain-info.json"} {
		data, err := os.ReadFile(filepath.Join(task.artifactDirectory, name))
		if err != nil {
			t.Fatal(err)
		}
		for _, secret := range []string{"source-user", "source-password", "source-secret", "private-fragment"} {
			if bytes.Contains(data, []byte(secret)) {
				t.Errorf("%s exposes source URL credential %q", name, secret)
			}
		}
	}
	const descriptiveName = "Slope layer ?token=label & contrast"
	if publicSourceLabel(descriptiveName) != descriptiveName {
		t.Fatal("ordinary descriptive source name changed")
	}
}

func TestTerrainWorkerPublishesMeshesAndRetriesCumulativeRawBaseline(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	var missing atomic.Bool
	missing.Store(true)
	var requests atomic.Int32
	data := terrainTestPNG(t, 650.25)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if missing.Load() && r.URL.Path == "/2/2/1.png" {
			w.WriteHeader(503)
			return
		}
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(data)
	}))
	defer upstream.Close()
	plan := storeTerrainTestPlan(t, upstream.URL+"/{z}/{x}/{y}.png")
	first := executeStoredWorker(t, plan, "terrain-first", "immediate")
	if first.ArtifactStatus != ArtifactReady || first.Status != TaskPartialFailed {
		t.Fatalf("first run: %+v", first)
	}
	state := store.integrityState(plan.ID)
	if state.Expected != 3 || state.Available != 2 || state.Missing != 1 {
		t.Fatalf("mesh coverage counted source halo: %+v", state)
	}
	missing.Store(false)
	requests.Store(0)
	second := executeStoredWorker(t, plan, "terrain-retry", triggerRetryFailures)
	if second.Status != TaskCompleted || second.ArtifactStatus != ArtifactReady {
		t.Fatalf("retry: %+v", second)
	}
	if requests.Load() != 1 {
		t.Fatalf("retry downloaded %d rasters instead of one failure", requests.Load())
	}
	state = store.integrityState(plan.ID)
	if state.Expected != 3 || state.Available != 3 || state.Missing != 0 || state.Status != "complete" {
		t.Fatalf("retry mesh coverage: %+v", state)
	}
	reader, err := zip.OpenReader(second.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	files := make(map[string]*zip.File)
	for _, entry := range reader.File {
		files[entry.Name] = entry
		if strings.HasSuffix(entry.Name, ".png") || strings.HasSuffix(entry.Name, ".sqlite") || strings.HasSuffix(entry.Name, ".partial") {
			t.Fatalf("source/cache leaked into terrain artifact: %s", entry.Name)
		}
	}
	for _, name := range []string{"layer.json", "README.txt", "0/0/0.terrain", "1/1/1.terrain", "2/2/2.terrain"} {
		if files[name] == nil {
			t.Fatalf("missing %s", name)
		}
	}
	layer, err := files["layer.json"].Open()
	if err != nil {
		t.Fatal(err)
	}
	var manifest terrain.Manifest
	err = json.NewDecoder(layer).Decode(&manifest)
	layer.Close()
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Projection != "EPSG:3857" || manifest.Scheme != "tms" || manifest.Available[2][0].StartY != 2 {
		t.Fatalf("manifest: %+v", manifest)
	}
	// Persistent raw baseline remains available independently of the terrain ZIP.
	if _, err := os.Stat(filepath.Join(second.OutputPath, "0", "0", "0.png")); err != nil {
		t.Fatal(err)
	}
	fresh, err := store.getTaskRecordByID(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	response := taskResponseFromRecord(fresh)
	if response.Artifact.Format != "cesium" || response.Sources[0].Metadata.Encoding != "terrarium" {
		t.Fatalf("terrain API metadata missing: %+v", response.Artifact)
	}
}

func TestTerrainConversionReusesCompletedMeshesAndRejectsMissingRoot(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	plan := storeTerrainTestPlan(t, "http://fixture.test/{z}/{x}/{y}.png")
	task, err := buildTaskFromRecord(plan)
	if err != nil {
		t.Fatal(err)
	}
	defer task.cancel()
	directory, err := managedRunDirectory(plan, "resume")
	if err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "resume", OutputPath: filepath.Join(directory, "tiles")}
	data := terrainTestPNG(t, -12.5)
	if err := task.forEachExpected(func(job TileJob) error {
		path := filepath.Join(run.OutputPath, fmt.Sprint(job.Tile.Z), fmt.Sprint(job.Tile.X), fmt.Sprintf("%d.png", job.Tile.Y))
		return atomicTerrainWrite(path, func(w io.Writer) error { _, err := w.Write(data); return err })
	}); err != nil {
		t.Fatal(err)
	}
	if err := deriveTerrainArtifacts(plan, task, run); err != nil {
		t.Fatal(err)
	}
	meshPath := filepath.Join(directory, "terrain", "0", "0", "0.terrain")
	original, err := os.Stat(meshPath)
	if err != nil {
		t.Fatal(err)
	}
	partial := filepath.Join(filepath.Dir(meshPath), ".terrain-killed.partial")
	if err := os.WriteFile(partial, []byte("incomplete gzip"), filePermissions); err != nil {
		t.Fatal(err)
	}
	// A restart should reuse CRC-valid output even if the raw root is no longer
	// needed for conversion; no fallback elevation is manufactured.
	if err := os.Remove(filepath.Join(run.OutputPath, "0", "0", "0.png")); err != nil {
		t.Fatal(err)
	}
	if err := deriveTerrainArtifacts(plan, task, run); err != nil {
		t.Fatal(err)
	}
	after, err := os.Stat(meshPath)
	if err != nil {
		t.Fatal(err)
	}
	if !after.ModTime().Equal(original.ModTime()) {
		t.Fatal("resume rewrote valid mesh")
	}
	if _, err := os.Stat(partial); !os.IsNotExist(err) {
		t.Fatalf("resume kept interrupted conversion temporary: %v", err)
	}
	if err := os.WriteFile(meshPath, []byte("interrupted mesh"), filePermissions); err != nil {
		t.Fatal(err)
	}
	if err := deriveTerrainArtifacts(plan, task, run); err == nil {
		t.Fatal("published terrain with absent root source and corrupt root mesh")
	}
}

func TestTerrainMissingFirstRootCanRetryFromRawCache(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	var rootMissing atomic.Bool
	rootMissing.Store(true)
	var requests atomic.Int32
	data := terrainTestPNG(t, 222.5)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if rootMissing.Load() && r.URL.Path == "/0/0/0.png" {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(data)
	}))
	defer upstream.Close()
	plan := storeTerrainTestPlan(t, upstream.URL+"/{z}/{x}/{y}.png")
	first := executeStoredWorker(t, plan, "missing-root-first", "immediate")
	if first.Status != TaskFailed || first.ArtifactStatus == ArtifactReady {
		t.Fatalf("missing root was published: %+v", first)
	}
	rootMissing.Store(false)
	requests.Store(0)
	second := executeStoredWorker(t, plan, "missing-root-retry", triggerRetryFailures)
	if second.Status != TaskCompleted || second.ArtifactStatus != ArtifactReady {
		t.Fatalf("root retry failed: %+v", second)
	}
	if requests.Load() != 1 {
		t.Fatalf("root retry made %d requests; raw successful baseline was not reused", requests.Load())
	}
}

func TestTerrainTransparentRootStaysRetryableUntilValidElevation(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	var invalid atomic.Bool
	invalid.Store(true)
	var requests atomic.Int32
	var transparent bytes.Buffer
	if err := png.Encode(&transparent, image.NewNRGBA(image.Rect(0, 0, 256, 256))); err != nil {
		t.Fatal(err)
	}
	data := terrainTestPNG(t, -4.75)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if invalid.Load() && r.URL.Path == "/0/0/0.png" {
			_, _ = w.Write(transparent.Bytes())
			return
		}
		_, _ = w.Write(data)
	}))
	defer upstream.Close()
	plan := storeTerrainTestPlan(t, upstream.URL+"/{z}/{x}/{y}.png")
	first := executeStoredWorker(t, plan, "transparent-first", "immediate")
	if first.Status != TaskFailed || first.ArtifactStatus == ArtifactReady {
		t.Fatalf("transparent source published as elevation: %+v", first)
	}
	summary, err := store.failureSummary(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Retryable != 1 {
		t.Fatalf("NoData failure not retained: %+v", summary)
	}
	var covered int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM run_coverage WHERE run_id=? AND z=0`, first.ID).Scan(&covered); err != nil {
		t.Fatal(err)
	}
	if covered != 0 {
		t.Fatal("invalid root was retained as successful elevation coverage")
	}
	invalid.Store(false)
	requests.Store(0)
	second := executeStoredWorker(t, plan, "transparent-retry", triggerRetryFailures)
	if second.Status != TaskCompleted || requests.Load() != 1 {
		t.Fatalf("NoData retry failed or redownloaded successful data: %+v, requests=%d", second, requests.Load())
	}
	summary, err = store.failureSummary(plan.ID)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Retryable != 0 {
		t.Fatalf("valid elevation did not resolve previous NoData failure: %+v", summary)
	}
}

// Explicit local fixture export for the Cesium browser smoke. Normal go test
// never writes outside t.TempDir and never downloads terrain from the network.
func TestTerrainBrowserFixture(t *testing.T) {
	directory := os.Getenv("TILER_TERRAIN_FIXTURE_DIR")
	if directory == "" {
		t.Skip("set TILER_TERRAIN_FIXTURE_DIR to export local Cesium fixture")
	}
	if !filepath.IsAbs(directory) {
		t.Fatal("fixture directory must be absolute")
	}
	ids := make([]terrain.TileID, 0, 85)
	for z := 0; z <= 3; z++ {
		for y := 0; y < 1<<z; y++ {
			for x := 0; x < 1<<z; x++ {
				id := terrain.TileID{Z: z, X: x, Y: y}
				grid := &terrain.Grid{Tile: id, Heights: make([]float64, terrain.GridSize*terrain.GridSize)}
				n := float64(int64(1) << z)
				for row := 0; row < terrain.GridSize; row++ {
					for col := 0; col < terrain.GridSize; col++ {
						lonDegrees := (float64(x)+float64(col)/64)/n*360 - 180
						lon := lonDegrees * math.Pi / 180
						lat := math.Atan(math.Sinh(math.Pi * (1 - 2*(float64(y)+float64(row)/64)/n)))
						latDegrees := lat * 180 / math.Pi
						wave := math.Sin((lonDegrees - 112) * math.Pi / 2)
						peak := 2000 * math.Exp(-((lonDegrees-115)*(lonDegrees-115)+(latDegrees-35)*(latDegrees-35))/3) * wave * wave
						grid.Heights[row*terrain.GridSize+col] = 600 + 400*math.Sin(lon)*math.Cos(lat) + peak
						if height := grid.Heights[row*terrain.GridSize+col]; math.IsNaN(height) || math.IsInf(height, 0) || height < 0 || height > 5000 {
							t.Fatalf("invalid synthetic fixture height: %v", height)
						}
					}
				}
				if err := atomicTerrainWrite(filepath.Join(directory, filepath.FromSlash(id.Path())), func(w io.Writer) error { _, err := terrain.WriteQuantizedMesh(w, grid); return err }); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, id)
			}
		}
	}
	manifest, err := terrain.BuildManifest("本地合成高程夹具", ids, terrain.SourceMetadata{Name: "本地合成高程夹具", Encoding: "Terrarium", Attribution: "本地合成高程夹具，无外部数据服务", VerticalDatum: "Test fixture"})
	if err != nil {
		t.Fatal(err)
	}
	info := terrainPreviewInfo{URL: "/fixture/", Bounds: [4]float64{112, 32, 118, 38}, Source: defaultTerrainSource(), Expected: int64(len(ids)), Available: int64(len(ids))}
	info.Source.Attribution = "本地合成高程夹具：600 + 400 sin(经度) cos(纬度)，叠加115°E/35°N附近最高2000米的连续局部峰；非实测地形。"
	for name, value := range map[string]any{"layer.json": manifest, "terrain-info.json": info} {
		if err := atomicTerrainWrite(filepath.Join(directory, name), func(w io.Writer) error { return json.NewEncoder(w).Encode(value) }); err != nil {
			t.Fatal(err)
		}
	}
	t.Logf("exported %d real quantized meshes to %s at %s", len(ids), directory, time.Now().Format(time.RFC3339))
}
