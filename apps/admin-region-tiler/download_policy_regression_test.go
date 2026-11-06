package main

import (
	"encoding/base64"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func TestFetchReusesConnectionsAndHonorsSourceOverride(t *testing.T) {
	for _, closeConnection := range []bool{false, true} {
		var connections atomic.Int32
		upstream := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte("tile")) }))
		upstream.Config.ConnState = func(_ net.Conn, state http.ConnState) {
			if state == http.StateNew {
				connections.Add(1)
			}
		}
		upstream.Start()
		policy := defaultFetchPolicy(upstream.URL, "local")
		if closeConnection {
			policy.Headers = map[string]string{"Connection": "close"}
		}
		task := NewTask([]Layer{{Tiles: []maptile.Tile{maptile.New(0, 0, 0)}}}, TileMap{}, TaskOptions{Policy: policy, RequestTTL: time.Second})
		for i := 0; i < 3; i++ {
			if _, err := task.fetchTile(maptile.New(0, 0, 0), upstream.URL); err != nil {
				t.Fatal(err)
			}
		}
		upstream.Close()
		task.cancel()
		want := int32(1)
		if closeConnection {
			want = 3
		}
		if connections.Load() != want {
			t.Errorf("close=%v: got %d TCP connections, want %d", closeConnection, connections.Load(), want)
		}
	}
}

func TestConfiguredRetryStatusControlsAttempts(t *testing.T) {
	for _, status := range []int{404, 503} {
		var calls atomic.Int32
		upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { calls.Add(1); w.WriteHeader(status) }))
		task := NewTask([]Layer{{Tiles: []maptile.Tile{maptile.New(0, 0, 0)}}}, TileMap{}, TaskOptions{MaxRetries: 2, RequestTTL: time.Second, RetryBackoff: time.Millisecond, Policy: FetchPolicy{RetryStatuses: map[int]struct{}{404: {}}}})
		_, _ = task.fetchTile(maptile.New(0, 0, 0), upstream.URL)
		upstream.Close()
		task.cancel()
		want := int32(1)
		if status == 404 {
			want = 3
		}
		if calls.Load() != want {
			t.Errorf("status %d: got %d attempts, want %d", status, calls.Load(), want)
		}
	}
}

func TestExplicitZeroGlobalRetries(t *testing.T) {
	previous := viper.Get("task.max_retries")
	viper.Set("task.max_retries", 0)
	t.Cleanup(func() { viper.Set("task.max_retries", previous) })
	task, err := buildTaskFromRequest(CreateTaskRequest{Name: "no retries", URL: "http://example.test/{z}/{x}/{y}", Format: PNG, Levels: []LevelRequest{{MinZoom: 0, MaxZoom: 0, Mode: "bbox", BBox: &BBoxRequest{MinLon: 0, MaxLon: 1, MinLat: 0, MaxLat: 1}}}})
	if err != nil {
		t.Fatal(err)
	}
	defer task.cancel()
	if task.maxRetries != 0 {
		t.Fatalf("explicit zero became %d retries", task.maxRetries)
	}
}

func TestActualWebPResponse(t *testing.T) {
	data, err := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
	if err != nil {
		t.Fatal(err)
	}
	if err := validateTileResponse(data, WEBP); err != nil {
		t.Fatal(err)
	}
	result, err := normalizeTileData(data, WEBP)
	if err != nil || string(result) != string(data) {
		t.Fatal("WebP bytes changed", err)
	}
}
