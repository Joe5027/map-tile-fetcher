package area

import "testing"

func TestNormalizeBBox(t *testing.T) {
	spec, err := Normalize(Spec{
		Mode: ModeBBox,
		BBox: &BBox{MinLon: 100, MinLat: 20, MaxLon: 101, MaxLat: 21},
	}, "")
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if spec.BBox == nil || spec.Mode != ModeBBox {
		t.Fatalf("expected normalized bbox mode, got %#v", spec)
	}
}

func TestNormalizeRejectsInvalidBBox(t *testing.T) {
	_, err := Normalize(Spec{
		Mode: ModeBBox,
		BBox: &BBox{MinLon: 101, MinLat: 20, MaxLon: 100, MaxLat: 21},
	}, "")
	if err == nil {
		t.Fatal("expected invalid bbox error")
	}
}

func TestNormalizeRegionAcceptsGeoJSON(t *testing.T) {
	spec, err := Normalize(Spec{Mode: ModeRegion, GeoJSON: " ./geojson/china.geojson "}, "")
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}
	if spec.GeoJSON != "./geojson/china.geojson" || spec.BBox != nil {
		t.Fatalf("unexpected normalized region: %#v", spec)
	}
}

func TestZoomRangeValidate(t *testing.T) {
	if err := (ZoomRange{Min: 0, Max: 20}).Validate(); err != nil {
		t.Fatalf("valid zoom range rejected: %v", err)
	}
	if err := (ZoomRange{Min: 10, Max: 9}).Validate(); err == nil {
		t.Fatal("expected invalid zoom range error")
	}
}
