package downloader

import (
	"testing"

	"tiler/internal/area"
)

func TestCalculateBBoxTilesWorldAtZoomZero(t *testing.T) {
	tiles, err := CalculateBBoxTiles(
		area.BBox{MinLon: -180, MinLat: -85, MaxLon: 180, MaxLat: 85},
		area.ZoomRange{Min: 0, Max: 0},
	)
	if err != nil {
		t.Fatalf("CalculateBBoxTiles returned error: %v", err)
	}
	if len(tiles) != 1 {
		t.Fatalf("expected 1 tile, got %d", len(tiles))
	}
	if tiles[0] != (TileID{Z: 0, X: 0, Y: 0}) {
		t.Fatalf("unexpected tile: %#v", tiles[0])
	}
}

func TestCalculateBBoxTilesPreservesInclusiveLegacyBounds(t *testing.T) {
	tiles, err := CalculateBBoxTiles(
		area.BBox{MinLon: -180, MinLat: 0, MaxLon: 0, MaxLat: 85},
		area.ZoomRange{Min: 2, Max: 2},
	)
	if err != nil {
		t.Fatalf("CalculateBBoxTiles returned error: %v", err)
	}
	if len(tiles) != 9 {
		t.Fatalf("expected inclusive 3x3 tile coverage, got %d", len(tiles))
	}
	first := tiles[0]
	last := tiles[len(tiles)-1]
	if first != (TileID{Z: 2, X: 0, Y: 0}) || last != (TileID{Z: 2, X: 2, Y: 2}) {
		t.Fatalf("unexpected tile bounds: first=%#v last=%#v", first, last)
	}
}

func TestCalculateBBoxTilesRejectsInvalidBBox(t *testing.T) {
	_, err := CalculateBBoxTiles(
		area.BBox{MinLon: 10, MinLat: 0, MaxLon: 9, MaxLat: 1},
		area.ZoomRange{Min: 1, Max: 1},
	)
	if err == nil {
		t.Fatal("expected invalid bbox error")
	}
}
