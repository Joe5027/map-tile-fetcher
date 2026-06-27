package main

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestBuildPlansFromRequestBBoxCreatesDirectLevel(t *testing.T) {
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
		t.Fatalf("expected 1 bbox level, got %d", len(parent.Levels))
	}

	level := parent.Levels[0]
	if level.MinZoom != 1 || level.MaxZoom != 2 {
		t.Fatalf("unexpected zoom range: %#v", level)
	}
	if level.Mode != "bbox" || level.BBox == nil {
		t.Fatalf("expected direct bbox level, got %#v", level)
	}
	if level.Geojson != "" {
		t.Fatalf("bbox level should not depend on generated geojson, got %s", level.Geojson)
	}
	if children[0].SourceName != "img" || children[1].SourceName != "cia" {
		t.Fatalf("unexpected child source names: %q %q", children[0].SourceName, children[1].SourceName)
	}
	if _, statErr := os.Stat("data"); !errors.Is(statErr, os.ErrNotExist) {
		t.Fatalf("bbox task should not write generated data, stat error: %v", statErr)
	}

	task, err := buildTaskFromPlan(children[0])
	if err != nil {
		t.Fatalf("direct bbox child task should build: %v", err)
	}
	if task.Total == 0 {
		t.Fatal("direct bbox task should enumerate tiles")
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

func TestPlanResponseIncludesUnifiedTaskContractFields(t *testing.T) {
	previousStore := store
	store = newSQLiteTestStore(t)
	t.Cleanup(func() {
		store = previousStore
	})

	runAt := time.Unix(2000, 0)
	parent := &PlanRecord{
		ID:           "task-contract",
		UserID:       42,
		Kind:         PlanKindGroup,
		Name:         "bbox contract",
		URL:          "https://example.test/img/{z}/{x}/{y}.png",
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels: []LevelConfig{{
			MinZoom: 3,
			MaxZoom: 4,
			Mode:    "bbox",
			BBox:    &BBoxRequest{MinLon: 100, MinLat: 20, MaxLon: 101, MaxLat: 21},
		}},
	}
	child := &PlanRecord{
		ID:           "task-contract-img",
		UserID:       42,
		ParentID:     parent.ID,
		Kind:         PlanKindChild,
		Name:         parent.Name,
		SourceName:   "img",
		URL:          parent.URL,
		Format:       PNG,
		Schema:       "xyz",
		ScheduleMode: ScheduleImmediate,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels:       parent.Levels,
	}
	parent.Children = []*PlanRecord{child}
	if err := store.createPlan(parent); err != nil {
		t.Fatalf("create parent failed: %v", err)
	}
	if err := store.createPlan(child); err != nil {
		t.Fatalf("create child failed: %v", err)
	}

	response := planResponseFromPlan(parent)
	if response.Mode != "bbox" || response.Area.BBox == nil {
		t.Fatalf("expected bbox response area, got %#v", response.Area)
	}
	if response.Zoom == nil || response.Zoom.Min != 3 || response.Zoom.Max != 4 {
		t.Fatalf("unexpected response zoom: %#v", response.Zoom)
	}
	if len(response.Sources) != 1 || response.Sources[0].Layer != "img" {
		t.Fatalf("unexpected response sources: %#v", response.Sources)
	}
	if response.Progress.Total != response.Total || response.Artifact.Status != response.ArtifactStatus {
		t.Fatalf("unified response mirrors are inconsistent: %#v", response)
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
