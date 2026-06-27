package planner

import (
	"testing"

	"tiler/internal/area"
)

func TestNormalizeRequestBBox(t *testing.T) {
	definition, err := NormalizeRequest(Request{
		Name: " Range task ",
		Mode: area.ModeBBox,
		Area: area.Spec{BBox: &area.BBox{MinLon: 100, MinLat: 20, MaxLon: 101, MaxLat: 21}},
		Zoom: area.ZoomRange{Min: 1, Max: 3},
		Sources: []Source{{
			URL:    "https://example.test/{z}/{x}/{y}.png",
			Layer:  "img",
			Format: "PNG",
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeRequest returned error: %v", err)
	}
	if definition.Name != "Range task" {
		t.Fatalf("unexpected name: %q", definition.Name)
	}
	if definition.Mode != area.ModeBBox || definition.Area.BBox == nil {
		t.Fatalf("unexpected area: %#v", definition.Area)
	}
	if definition.Sources[0].Schema != "xyz" || definition.Sources[0].Format != "png" {
		t.Fatalf("unexpected source normalization: %#v", definition.Sources[0])
	}
	if definition.Schedule.Mode != ScheduleImmediate {
		t.Fatalf("unexpected schedule: %#v", definition.Schedule)
	}
	if definition.Output.Format != OutputMBTiles || definition.Output.Package != PackageZIP {
		t.Fatalf("unexpected output: %#v", definition.Output)
	}
}

func TestNormalizeRequestRegion(t *testing.T) {
	definition, err := NormalizeRequest(Request{
		Name: "Region task",
		Mode: area.ModeRegion,
		Area: area.Spec{RegionID: "china"},
		Zoom: area.ZoomRange{Min: 6, Max: 8},
		Sources: []Source{{
			Name:   "vec",
			URL:    "https://example.test/{z}/{x}/{y}.png",
			Format: "png",
			Schema: "tms",
		}},
	})
	if err != nil {
		t.Fatalf("NormalizeRequest returned error: %v", err)
	}
	if definition.Mode != area.ModeRegion || definition.Area.RegionID != "china" {
		t.Fatalf("unexpected region definition: %#v", definition)
	}
}

func TestNormalizeRequestRejectsMissingSource(t *testing.T) {
	_, err := NormalizeRequest(Request{
		Name: "missing source",
		Mode: area.ModeRegion,
		Area: area.Spec{GeoJSON: "./geojson/china.geojson"},
		Zoom: area.ZoomRange{Min: 1, Max: 2},
	})
	if err == nil {
		t.Fatal("expected missing source error")
	}
}

func TestNormalizeRequestRejectsScheduledWithoutRunAt(t *testing.T) {
	_, err := NormalizeRequest(Request{
		Name: "scheduled",
		Mode: area.ModeRegion,
		Area: area.Spec{GeoJSON: "./geojson/china.geojson"},
		Zoom: area.ZoomRange{Min: 1, Max: 2},
		Sources: []Source{{
			URL:    "https://example.test/{z}/{x}/{y}.png",
			Format: "png",
		}},
		Schedule: Schedule{Mode: ScheduleOnce},
	})
	if err == nil {
		t.Fatal("expected runAt error")
	}
}
