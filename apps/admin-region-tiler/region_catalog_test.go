package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestGetRegionCatalogGeoJSONBatchReturnsRequestedRegions(t *testing.T) {
	withTempWorkingDir(t)
	if err := os.MkdirAll("geojson", directoryPermissions); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile("geojson/regions.json", []byte(`[
		{"id":"one","name":"One","level":"province","parentId":"china","geojson":"./geojson/one.geojson"},
		{"id":"two","name":"Two","level":"province","parentId":"china","geojson":"./geojson/two.geojson"}
	]`), filePermissions); err != nil {
		t.Fatal(err)
	}
	for _, region := range []struct {
		Name string
		Data string
	}{
		{Name: "one.geojson", Data: `{"type":"Feature","properties":{"name":"One"},"geometry":{"type":"Point","coordinates":[0,0]}}`},
		{Name: "two.geojson", Data: `{"type":"Feature","properties":{"name":"Two"},"geometry":{"type":"Point","coordinates":[1,1]}}`},
	} {
		if err := os.WriteFile(filepath.Join("geojson", region.Name), []byte(region.Data), filePermissions); err != nil {
			t.Fatal(err)
		}
	}
	resetRegionCatalogCacheForTest(t)

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/config/region-catalog/geojson-batch?id=one&id=two", nil)
	getRegionCatalogGeoJSONBatch(context)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d: %s", http.StatusOK, recorder.Code, recorder.Body.String())
	}
	var response RegionGeoJSONBatchResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode batch response: %v", err)
	}
	if len(response.Regions) != 2 {
		t.Fatalf("expected 2 regions, got %#v", response.Regions)
	}
	for _, regionID := range []string{"one", "two"} {
		var geojson map[string]any
		if err := json.Unmarshal(response.Regions[regionID], &geojson); err != nil {
			t.Fatalf("decode %s geojson: %v", regionID, err)
		}
		if geojson["type"] != "Feature" {
			t.Fatalf("expected Feature geojson for %s, got %#v", regionID, geojson)
		}
	}
}

func resetRegionCatalogCacheForTest(t *testing.T) {
	t.Helper()
	regionCatalogCache.Lock()
	previousLoaded := regionCatalogCache.Loaded
	previousCheckedAt := regionCatalogCache.CheckedAt
	previousModTime := regionCatalogCache.ModTime
	previousSize := regionCatalogCache.Size
	previousResponse := regionCatalogCache.Response
	previousByID := regionCatalogCache.ByID
	previousGeoJSON := regionCatalogCache.GeoJSON
	regionCatalogCache.Loaded = false
	regionCatalogCache.CheckedAt = time.Time{}
	regionCatalogCache.ModTime = time.Time{}
	regionCatalogCache.Size = 0
	regionCatalogCache.Response = RegionCatalogResponse{}
	regionCatalogCache.ByID = make(map[string]RegionCatalogItem)
	regionCatalogCache.GeoJSON = make(map[string]cachedRegionGeoJSON)
	regionCatalogCache.Unlock()
	t.Cleanup(func() {
		regionCatalogCache.Lock()
		regionCatalogCache.Loaded = previousLoaded
		regionCatalogCache.CheckedAt = previousCheckedAt
		regionCatalogCache.ModTime = previousModTime
		regionCatalogCache.Size = previousSize
		regionCatalogCache.Response = previousResponse
		regionCatalogCache.ByID = previousByID
		regionCatalogCache.GeoJSON = previousGeoJSON
		regionCatalogCache.Unlock()
	})
}
