// Package terrain converts Terrarium elevation rasters into Cesium quantized
// meshes. Coordinates accepted by this package are XYZ; exported paths use TMS.
package terrain

import (
	"fmt"
	"math"
	"sort"
)

const (
	GridSize    = 65
	RasterSize  = 256
	MaxZoom     = 15
	MaxLatitude = 85.0511287798066
)

type TileID struct{ Z, X, Y int }

func (id TileID) Validate() error {
	if id.Z < 0 || id.Z > MaxZoom || id.X < 0 || id.Y < 0 || id.X >= 1<<id.Z || id.Y >= 1<<id.Z {
		return fmt.Errorf("invalid terrain XYZ tile: %+v", id)
	}
	return nil
}

func (id TileID) TMSY() int    { return (1 << id.Z) - 1 - id.Y }
func (id TileID) Path() string { return fmt.Sprintf("%d/%d/%d.terrain", id.Z, id.X, id.TMSY()) }

// Bounds returns west, south, east, north in degrees.
func (id TileID) Bounds() [4]float64 {
	n := float64(int64(1) << id.Z)
	return [4]float64{float64(id.X)/n*360 - 180, latitude(float64(id.Y+1) / n), float64(id.X+1)/n*360 - 180, latitude(float64(id.Y) / n)}
}

func latitude(y float64) float64 { return math.Atan(math.Sinh(math.Pi*(1-2*y))) * 180 / math.Pi }

func sortedIDs(set map[TileID]bool) []TileID {
	ids := make([]TileID, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := ids[i], ids[j]
		if a.Z != b.Z {
			return a.Z < b.Z
		}
		if a.Y != b.Y {
			return a.Y < b.Y
		}
		return a.X < b.X
	})
	return ids
}

// Ancestors adds the complete path to the single Web Mercator root, deduplicated.
func Ancestors(ids []TileID) ([]TileID, error) {
	set := make(map[TileID]bool)
	for _, id := range ids {
		if err := id.Validate(); err != nil {
			return nil, err
		}
		for {
			set[id] = true
			if id.Z == 0 {
				break
			}
			id = TileID{id.Z - 1, id.X / 2, id.Y / 2}
		}
	}
	return sortedIDs(set), nil
}

// SourceTiles includes the one-pixel halo needed for pixel-centred bilinear
// sampling. Longitude wraps; latitude stops at the actual Mercator data edge.
func SourceTiles(id TileID) ([]TileID, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}
	n := 1 << id.Z
	set := make(map[TileID]bool)
	for dy := -1; dy <= 1; dy++ {
		y := min(n-1, max(0, id.Y+dy))
		for dx := -1; dx <= 1; dx++ {
			set[TileID{id.Z, (id.X + dx + n) % n, y}] = true
		}
	}
	return sortedIDs(set), nil
}
