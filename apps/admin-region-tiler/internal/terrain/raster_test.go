package terrain

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/png"
	"math"
	"reflect"
	"testing"
)

func terrariumColor(height float64) color.NRGBA {
	v := int(math.Round((height + 32768) * 256))
	return color.NRGBA{R: byte(v >> 16), G: byte(v >> 8), B: byte(v), A: 255}
}

func pngBytes(t *testing.T, img image.Image) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestDecodeTerrariumPreservesSignedAndFractionalHeights(t *testing.T) {
	img := image.NewNRGBA(image.Rect(0, 0, RasterSize, RasterSize))
	values := []float64{-32768, -12.5, 0, 1234.25, 32767.99609375}
	for y := 0; y < RasterSize; y++ {
		for x := 0; x < RasterSize; x++ {
			img.SetNRGBA(x, y, terrariumColor(values[x%len(values)]))
		}
	}
	raster, err := DecodeTerrarium(bytes.NewReader(pngBytes(t, img)))
	if err != nil {
		t.Fatal(err)
	}
	for i, h := range raster.Heights {
		if want := values[(i%RasterSize)%len(values)]; h != want {
			t.Fatalf("height %d = %v, want %v", i, h, want)
		}
	}
}

func TestDecodeTerrariumRejectsMissingOrMalformedData(t *testing.T) {
	transparent := image.NewNRGBA(image.Rect(0, 0, RasterSize, RasterSize))
	wrongSize := image.NewNRGBA(image.Rect(0, 0, 1, 1))
	for name, data := range map[string][]byte{
		"transparent": pngBytes(t, transparent),
		"dimensions":  pngBytes(t, wrongSize),
		"not png":     []byte("not an elevation PNG"),
	} {
		t.Run(name, func(t *testing.T) {
			if raster, err := DecodeTerrarium(bytes.NewReader(data)); err == nil || raster != nil {
				t.Fatal("missing data must fail instead of creating zero elevations")
			}
		})
	}
}

func testLoader(calls map[TileID]int) Loader {
	return func(_ context.Context, id TileID) (*Raster, error) {
		if calls != nil {
			calls[id]++
		}
		raster := &Raster{Heights: make([]float64, RasterSize*RasterSize)}
		world := float64((int64(1) << id.Z) * RasterSize)
		for y := 0; y < RasterSize; y++ {
			for x := 0; x < RasterSize; x++ {
				gx, gy := float64(id.X*RasterSize+x)+0.5, float64(id.Y*RasterSize+y)+0.5
				raster.Heights[y*RasterSize+x] = 1000*math.Cos(gx/world*2*math.Pi) + gy/10 - 500
			}
		}
		return raster, nil
	}
}

func TestSampleGridSharedBordersAndOneLoadPerSource(t *testing.T) {
	cases := []struct{ left, right TileID }{
		{TileID{3, 3, 3}, TileID{3, 4, 3}},
		{TileID{3, 7, 3}, TileID{3, 0, 3}}, // longitude wrap
	}
	for _, pair := range cases {
		calls := make(map[TileID]int)
		left, err := SampleGrid(context.Background(), pair.left, testLoader(calls))
		if err != nil {
			t.Fatal(err)
		}
		if len(calls) != 9 {
			t.Fatalf("interior tile uses %d source rasters, want 9", len(calls))
		}
		for id, count := range calls {
			if count != 1 {
				t.Fatalf("source %+v loaded %d times", id, count)
			}
		}
		right, err := SampleGrid(context.Background(), pair.right, testLoader(nil))
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < GridSize; y++ {
			if left.Heights[y*GridSize+GridSize-1] != right.Heights[y*GridSize] {
				t.Fatalf("shared longitude edge differs on row %d", y)
			}
		}
	}
	upper, err := SampleGrid(context.Background(), TileID{3, 3, 3}, testLoader(nil))
	if err != nil {
		t.Fatal(err)
	}
	lower, err := SampleGrid(context.Background(), TileID{3, 3, 4}, testLoader(nil))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(upper.Heights[(GridSize-1)*GridSize:], lower.Heights[:GridSize]) {
		t.Fatal("shared latitude edge differs")
	}
}

func TestSampleGridUsesPixelCentresAndClampsActualPolarEdge(t *testing.T) {
	linear := func(_ context.Context, id TileID) (*Raster, error) {
		r := &Raster{Heights: make([]float64, RasterSize*RasterSize)}
		for y := 0; y < RasterSize; y++ {
			for x := 0; x < RasterSize; x++ {
				r.Heights[y*RasterSize+x] = float64(id.Y*RasterSize+y) + 0.5
			}
		}
		return r, nil
	}
	for _, id := range []TileID{{0, 0, 0}, {3, 2, 0}, {3, 2, 7}, {3, 2, 3}} {
		grid, err := SampleGrid(context.Background(), id, linear)
		if err != nil {
			t.Fatal(err)
		}
		for y := 0; y < GridSize; y++ {
			world := float64((int64(1) << id.Z) * RasterSize)
			want := math.Max(0.5, math.Min(world-0.5, float64(id.Y*RasterSize+y*4)))
			if grid.Heights[y*GridSize] != want {
				t.Fatalf("tile %+v row %d = %v, want %v", id, y, grid.Heights[y*GridSize], want)
			}
		}
	}
}

func TestSampleGridPropagatesMissingSourceAndCancellation(t *testing.T) {
	sentinel := errors.New("source unavailable")
	_, err := SampleGrid(context.Background(), TileID{3, 3, 3}, func(context.Context, TileID) (*Raster, error) { return nil, sentinel })
	if !errors.Is(err, sentinel) {
		t.Fatalf("source error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = SampleGrid(ctx, TileID{}, testLoader(nil))
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("cancel error = %v", err)
	}
	_, err = SampleGrid(context.Background(), TileID{}, func(context.Context, TileID) (*Raster, error) {
		r := &Raster{Heights: make([]float64, RasterSize*RasterSize)}
		r.Heights[0] = math.NaN()
		return r, nil
	})
	if err == nil {
		t.Fatal("missing pixel must fail")
	}
}
