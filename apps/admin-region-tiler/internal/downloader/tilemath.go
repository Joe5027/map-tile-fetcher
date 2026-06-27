package downloader

import (
	"math"

	"tiler/internal/area"
)

type TileID struct {
	Z int
	X int
	Y int
}

func CalculateBBoxTiles(box area.BBox, zoom area.ZoomRange) ([]TileID, error) {
	if err := box.Validate(); err != nil {
		return nil, err
	}
	if err := zoom.Validate(); err != nil {
		return nil, err
	}

	tiles := make([]TileID, 0)
	for z := zoom.Min; z <= zoom.Max; z++ {
		leftTop := tileForLonLat(box.MinLon, box.MaxLat, z)
		rightBottom := tileForLonLat(box.MaxLon, box.MinLat, z)
		for x := leftTop.X; x <= rightBottom.X; x++ {
			for y := leftTop.Y; y <= rightBottom.Y; y++ {
				tiles = append(tiles, TileID{Z: z, X: x, Y: y})
			}
		}
	}
	return tiles, nil
}

func CountBBoxTiles(box area.BBox, zoom area.ZoomRange) (int64, error) {
	if err := box.Validate(); err != nil {
		return 0, err
	}
	if err := zoom.Validate(); err != nil {
		return 0, err
	}
	var count int64
	for z := zoom.Min; z <= zoom.Max; z++ {
		leftTop := tileForLonLat(box.MinLon, box.MaxLat, z)
		rightBottom := tileForLonLat(box.MaxLon, box.MinLat, z)
		count += int64(rightBottom.X-leftTop.X+1) * int64(rightBottom.Y-leftTop.Y+1)
	}
	return count, nil
}

func tileForLonLat(lon, lat float64, z int) TileID {
	n := math.Pow(2, float64(z))
	x := int(math.Floor((lon + 180.0) / 360.0 * n))
	latRad := lat * math.Pi / 180.0
	y := int(math.Floor((1.0 - math.Log(math.Tan(latRad)+1.0/math.Cos(latRad))/math.Pi) / 2.0 * n))
	max := int(n) - 1
	return TileID{
		Z: z,
		X: clampInt(x, 0, max),
		Y: clampInt(y, 0, max),
	}
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}
