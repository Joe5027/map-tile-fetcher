package terrain

import (
	"context"
	"errors"
	"fmt"
	"image/color"
	"image/png"
	"io"
	"math"
)

// Raster is a 256-square Terrarium raster in north-to-south row order.
type Raster struct{ Heights []float64 }
type Loader func(context.Context, TileID) (*Raster, error)
type Grid struct {
	Tile    TileID
	Heights []float64
}

// DecodeTerrarium preserves all three colour channels; alpha/invalid input is
// missing elevation, never an instruction to invent sea-level terrain.
func DecodeTerrarium(r io.Reader) (*Raster, error) {
	img, err := png.Decode(r)
	if err != nil {
		return nil, fmt.Errorf("decode Terrarium PNG: %w", err)
	}
	b := img.Bounds()
	if b.Dx() != RasterSize || b.Dy() != RasterSize {
		return nil, fmt.Errorf("Terrarium raster must be %dx%d", RasterSize, RasterSize)
	}
	result := &Raster{Heights: make([]float64, RasterSize*RasterSize)}
	for y := 0; y < RasterSize; y++ {
		for x := 0; x < RasterSize; x++ {
			c := color.NRGBAModel.Convert(img.At(x+b.Min.X, y+b.Min.Y)).(color.NRGBA)
			if c.A != 255 {
				return nil, fmt.Errorf("missing Terrarium elevation at %d,%d", x, y)
			}
			result.Heights[y*RasterSize+x] = float64(c.R)*256 + float64(c.G) + float64(c.B)/256 - 32768
		}
	}
	return result, nil
}

func SampleGrid(ctx context.Context, id TileID, load Loader) (*Grid, error) {
	if err := id.Validate(); err != nil {
		return nil, err
	}
	if load == nil {
		return nil, errors.New("terrain source loader is required")
	}
	cache := make(map[TileID]*Raster)
	world := (1 << id.Z) * RasterSize
	pixel := func(x, y int) (float64, error) {
		x = (x%world + world) % world
		y = min(world-1, max(0, y))
		sourceID := TileID{id.Z, x / RasterSize, y / RasterSize}
		raster := cache[sourceID]
		if raster == nil {
			var err error
			raster, err = load(ctx, sourceID)
			if err != nil {
				return 0, fmt.Errorf("source %d/%d/%d: %w", sourceID.Z, sourceID.X, sourceID.Y, err)
			}
			if raster == nil || len(raster.Heights) != RasterSize*RasterSize {
				return 0, errors.New("invalid terrain source raster")
			}
			cache[sourceID] = raster
		}
		h := raster.Heights[(y%RasterSize)*RasterSize+x%RasterSize]
		if math.IsNaN(h) || math.IsInf(h, 0) {
			return 0, errors.New("missing terrain source elevation")
		}
		return h, nil
	}
	grid := &Grid{Tile: id, Heights: make([]float64, GridSize*GridSize)}
	for y := 0; y < GridSize; y++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		for x := 0; x < GridSize; x++ {
			// All adjacent tiles use the same global sample position. Raster values
			// represent pixel centres, hence the half-pixel shift at tile borders.
			gx := float64(id.X*RasterSize+x*RasterSize/(GridSize-1)) - 0.5
			gy := float64(id.Y*RasterSize+y*RasterSize/(GridSize-1)) - 0.5
			x0, y0 := int(math.Floor(gx)), int(math.Floor(gy))
			fx, fy := gx-float64(x0), gy-float64(y0)
			var h [4]float64
			for i, p := range [4][2]int{{x0, y0}, {x0 + 1, y0}, {x0, y0 + 1}, {x0 + 1, y0 + 1}} {
				var err error
				h[i], err = pixel(p[0], p[1])
				if err != nil {
					return nil, err
				}
			}
			grid.Heights[y*GridSize+x] = (h[0]*(1-fx)+h[1]*fx)*(1-fy) + (h[2]*(1-fx)+h[3]*fx)*fy
		}
	}
	return grid, nil
}
