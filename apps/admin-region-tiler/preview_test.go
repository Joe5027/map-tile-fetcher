package main

import (
	"bytes"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPreviewRegistrationIncludesExpiry(t *testing.T) {
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user", &UserRecord{ID: 9876})
	c.Request = httptest.NewRequest("POST", "/api/tile-preview/tianditu-token", bytes.NewBufferString(`{"token":"fixture-token"}`))
	c.Request.Header.Set("Content-Type", "application/json")
	registerTiandituPreviewToken(c)
	var response struct {
		ID        string    `json:"id"`
		ExpiresAt time.Time `json:"expiresAt"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.ID == "" || time.Until(response.ExpiresAt) < 14*time.Minute {
		t.Fatalf("invalid token registration: %s", w.Body.String())
	}
	tiandituPreviewTokens.Lock()
	delete(tiandituPreviewTokens.values, response.ID)
	tiandituPreviewTokens.Unlock()
}

func TestTaskAreaReturnsFullCollectionAndLevels(t *testing.T) {
	withTempWorkingDir(t)
	useTestStore(t)
	dir := filepath.Join("data", "generated-areas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "areas.geojson")
	data := `{"type":"FeatureCollection","features":[{"type":"Feature","properties":{},"geometry":{"type":"Point","coordinates":[10,10]}},{"type":"Feature","properties":{},"geometry":{"type":"Point","coordinates":[11,11]}}]}`
	if err := os.WriteFile(path, []byte(data), 0644); err != nil {
		t.Fatal(err)
	}
	p := storedBBoxTask(t, "preview-levels", "")
	levels, _ := json.Marshal([]LevelConfig{{MinZoom: 1, MaxZoom: 2, Geojson: path}, {MinZoom: 3, MaxZoom: 4, Geojson: path}})
	if _, err := store.db.Exec(`UPDATE plans SET levels_json=? WHERE id=?`, string(levels), p.ID); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"", "?level=0"} {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("user", &UserRecord{ID: 1})
		c.Params = gin.Params{{Key: "id", Value: p.ID}}
		c.Request = httptest.NewRequest("GET", "/api/tasks/preview-levels/area"+query, nil)
		getTaskArea(c)
		var response struct {
			Features []json.RawMessage `json:"features"`
			Levels   []json.RawMessage `json:"levels"`
			Selected int               `json:"selectedLevel"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		want := 1
		if query != "" {
			want = 0
		}
		if w.Code != 200 || len(response.Features) != 2 || len(response.Levels) != 2 || response.Selected != want {
			t.Fatalf("incomplete task area: %s", w.Body.String())
		}
	}
}
