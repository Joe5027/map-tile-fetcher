package main

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildPlansFromRequestBBoxCreatesGeneratedLevel(t *testing.T) {
	withTempWorkingDir(t)

	parent, children, err := buildPlansFromRequest(42, CreateTaskRequest{
		Name: "bbox task",
		Mode: "bbox",
		Area: AreaRequest{BBox: &BBoxRequest{
			MinLon: 100,
			MinLat: 20,
			MaxLon: 101,
			MaxLat: 21,
		}},
		Zoom: &ZoomRangeRequest{Min: 1, Max: 2},
		Sources: []SourceRequest{
			{Name: "img", URL: "https://example.test/img/{z}/{x}/{y}.png", Format: PNG, Schema: "xyz"},
			{Name: "cia", URL: "https://example.test/cia/{z}/{x}/{y}.png", Format: PNG, Schema: "xyz"},
		},
	})
	if err != nil {
		t.Fatalf("buildPlansFromRequest returned error: %v", err)
	}
	if parent.Kind != PlanKindGroup {
		t.Fatalf("expected group plan, got %s", parent.Kind)
	}
	if len(children) != 2 {
		t.Fatalf("expected 2 child plans, got %d", len(children))
	}
	if len(parent.Levels) != 1 {
		t.Fatalf("expected 1 generated level, got %d", len(parent.Levels))
	}

	level := parent.Levels[0]
	if level.MinZoom != 1 || level.MaxZoom != 2 {
		t.Fatalf("unexpected zoom range: %#v", level)
	}
	if _, err := os.Stat(level.Geojson); err != nil {
		t.Fatalf("generated geojson was not written: %v", err)
	}
	if filepath.Base(filepath.Dir(level.Geojson)) != "generated-areas" {
		t.Fatalf("generated bbox should live under data/generated-areas, got %s", level.Geojson)
	}
	collection, err := loadCollection(level.Geojson)
	if err != nil {
		t.Fatalf("generated geojson should be loadable: %v", err)
	}
	if len(collection) != 1 {
		t.Fatalf("expected one generated bbox geometry, got %d", len(collection))
	}
	if children[0].SourceName != "img" || children[1].SourceName != "cia" {
		t.Fatalf("unexpected child source names: %q %q", children[0].SourceName, children[1].SourceName)
	}
}

func TestBuildPlansFromRequestRejectsInvalidBBoxWithoutWritingData(t *testing.T) {
	withTempWorkingDir(t)

	_, _, err := buildPlansFromRequest(42, CreateTaskRequest{
		Name: "bad bbox",
		Mode: "bbox",
		Area: AreaRequest{BBox: &BBoxRequest{
			MinLon: 101,
			MinLat: 20,
			MaxLon: 100,
			MaxLat: 21,
		}},
		Zoom: &ZoomRangeRequest{Min: 1, Max: 2},
		Sources: []SourceRequest{{
			Name:   "img",
			URL:    "https://example.test/img/{z}/{x}/{y}.png",
			Format: PNG,
			Schema: "xyz",
		}},
	})
	if err == nil {
		t.Fatal("expected invalid bbox error")
	}
	if _, statErr := os.Stat("data"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("invalid bbox should not write generated data, stat error: %v", statErr)
	}
}

func withTempWorkingDir(t *testing.T) {
	t.Helper()
	previous, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd failed: %v", err)
	}
	temp := t.TempDir()
	if err := os.Chdir(temp); err != nil {
		t.Fatalf("chdir temp failed: %v", err)
	}
	t.Cleanup(func() {
		if err := os.Chdir(previous); err != nil {
			t.Fatalf("restore cwd failed: %v", err)
		}
	})
}
