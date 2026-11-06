package terrain

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func TestTileAncestorsPathsAndHalo(t *testing.T) {
	id := TileID{3, 5, 6}
	got, err := Ancestors([]TileID{id, id})
	want := []TileID{{0, 0, 0}, {1, 1, 1}, {2, 2, 3}, id}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("Ancestors = %v, %v; want %v", got, err, want)
	}
	if id.Path() != "3/5/1.terrain" {
		t.Fatalf("path = %s", id.Path())
	}
	for _, tc := range []struct {
		id    TileID
		count int
	}{{TileID{}, 1}, {TileID{3, 3, 3}, 9}, {TileID{3, 0, 0}, 6}} {
		halo, err := SourceTiles(tc.id)
		if err != nil || len(halo) != tc.count {
			t.Fatalf("SourceTiles(%+v) = %v, %v", tc.id, halo, err)
		}
		for _, source := range halo {
			if err := source.Validate(); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, invalid := range []TileID{{-1, 0, 0}, {16, 0, 0}, {1, 2, 0}, {1, 0, -1}} {
		if _, err := Ancestors([]TileID{invalid}); err == nil {
			t.Fatalf("accepted %+v", invalid)
		}
	}
}

func TestManifestRequiresCompletedAncestorsAndUsesTMS(t *testing.T) {
	for _, incomplete := range [][]TileID{nil, {{1, 0, 0}}, {{0, 0, 0}, {2, 0, 0}}} {
		if _, err := BuildManifest("test", incomplete, SourceMetadata{}); err == nil {
			t.Fatalf("accepted incomplete hierarchy %v", incomplete)
		}
	}
	ids, err := Ancestors([]TileID{{3, 5, 6}})
	if err != nil {
		t.Fatal(err)
	}
	source := SourceMetadata{Name: "fixture", Encoding: "Terrarium", VerticalDatum: "source-dependent", Attribution: "fixture attribution"}
	manifest, err := BuildManifest("sample", ids, source)
	if err != nil {
		t.Fatal(err)
	}
	if manifest.Projection != "EPSG:3857" || manifest.Scheme != "tms" || manifest.MaxZoom != 3 || manifest.MinZoom != 0 {
		t.Fatalf("wrong tile scheme: %+v", manifest)
	}
	if got := manifest.Available[3]; !reflect.DeepEqual(got, []Availability{{5, 1, 5, 1}}) {
		t.Fatalf("available uses wrong Y: %+v", got)
	}
	data, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{`"format":"quantized-mesh-1.0"`, `"tiles":["{z}/{x}/{y}.terrain"]`, `"attribution":"fixture attribution"`, `"verticalDatum":"source-dependent"`} {
		if !strings.Contains(string(data), text) {
			t.Fatalf("manifest missing %s: %s", text, data)
		}
	}
}

func TestManifestCoalescesOnlyCompletedRectangles(t *testing.T) {
	ids := []TileID{{0, 0, 0}}
	for z := 1; z <= 2; z++ {
		for y := 0; y < 1<<z; y++ {
			for x := 0; x < 1<<z; x++ {
				ids = append(ids, TileID{z, x, y})
			}
		}
	}
	manifest, err := BuildManifest("", ids, SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	for z := 0; z <= 2; z++ {
		want := []Availability{{0, 0, (1 << z) - 1, (1 << z) - 1}}
		if !reflect.DeepEqual(manifest.Available[z], want) {
			t.Fatalf("level %d rectangles: %+v", z, manifest.Available[z])
		}
	}
	// Remove one corner: compression must not claim this missing child exists.
	ids = ids[:len(ids)-1]
	manifest, err = BuildManifest("", ids, SourceMetadata{})
	if err != nil {
		t.Fatal(err)
	}
	covered := make(map[[2]int]bool)
	for _, rect := range manifest.Available[2] {
		for y := rect.StartY; y <= rect.EndY; y++ {
			for x := rect.StartX; x <= rect.EndX; x++ {
				covered[[2]int{x, y}] = true
			}
		}
	}
	if len(covered) != 15 || covered[[2]int{3, 0}] {
		t.Fatalf("availability overclaims missing tile: %+v", manifest.Available[2])
	}
}
