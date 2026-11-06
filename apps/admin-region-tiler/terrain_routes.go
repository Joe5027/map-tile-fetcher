package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"tiler/internal/terrain"
)

type terrainPreviewInfo struct {
	URL       string          `json:"url,omitempty"`
	Bounds    [4]float64      `json:"bounds"`
	Source    *SourceMetadata `json:"source"`
	Expected  int64           `json:"expected"`
	Available int64           `json:"available"`
}

func isTerrainPlan(plan *TaskRecord) bool {
	return plan != nil && len(plan.Levels) > 0 && plan.Levels[0].OutputFormat == "cesium"
}

func terrainDirectoryForRequest(c *gin.Context) (string, bool) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil || !isTerrainPlan(plan) {
		c.JSON(http.StatusNotFound, gin.H{"error": "terrain task not found"})
		return "", false
	}
	run, err := store.publishedRun(plan.ID)
	if err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "terrain artifact is not ready"})
		return "", false
	}
	directory, err := managedRunDirectory(plan, run.ID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "terrain artifact is unavailable"})
		return "", false
	}
	return filepath.Join(directory, "terrain"), true
}

func getTerrainInfo(c *gin.Context) {
	directory, ok := terrainDirectoryForRequest(c)
	if !ok {
		return
	}
	data, err := os.ReadFile(filepath.Join(directory, "terrain-info.json"))
	var info terrainPreviewInfo
	if err != nil || json.Unmarshal(data, &info) != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "terrain preview metadata is unavailable"})
		return
	}
	info.URL = "/api/tasks/" + c.Param("id") + "/terrain/"
	c.Header("Cache-Control", "private, no-store")
	c.JSON(http.StatusOK, info)
}

func terrainPublicPath(raw string) (string, bool) {
	path := strings.TrimPrefix(raw, "/")
	if path == "layer.json" {
		return path, true
	}
	parts := strings.Split(path, "/")
	if len(parts) != 3 || !strings.HasSuffix(parts[2], ".terrain") {
		return "", false
	}
	values := [3]int{}
	for i, text := range []string{parts[0], parts[1], strings.TrimSuffix(parts[2], ".terrain")} {
		value, err := strconv.Atoi(text)
		if err != nil || strconv.Itoa(value) != text {
			return "", false
		}
		values[i] = value
	}
	id := terrain.TileID{Z: values[0], X: values[1], Y: values[2]}
	if id.Validate() != nil {
		return "", false
	}
	return fmt.Sprintf("%d/%d/%d.terrain", values[0], values[1], values[2]), true
}

func getTerrainFile(c *gin.Context) {
	relative, ok := terrainPublicPath(c.Param("path"))
	if !ok {
		c.Status(http.StatusNotFound)
		return
	}
	directory, ok := terrainDirectoryForRequest(c)
	if !ok {
		return
	}
	path, err := ensurePathWithinRoot(filepath.Join(directory, filepath.FromSlash(relative)), directory, false)
	if err != nil {
		c.Status(http.StatusNotFound)
		return
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		c.Status(http.StatusNotFound)
		return
	}
	c.Header("Cache-Control", "private, no-cache")
	if strings.HasSuffix(relative, ".terrain") {
		c.Header("Content-Type", "application/vnd.quantized-mesh")
		c.Header("Content-Encoding", "gzip")
	} else {
		c.Header("Content-Type", "application/json")
	}
	c.File(path)
}
