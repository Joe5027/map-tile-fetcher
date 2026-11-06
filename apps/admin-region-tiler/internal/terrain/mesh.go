package terrain

import (
	"compress/gzip"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math"
)

const earthA = 6378137.0
const earthB = 6356752.3142451793
const quantMax = 32767

type Stats struct {
	MinimumHeight   float64
	MaximumHeight   float64
	Vertices        int
	Triangles       int
	HorizonDisabled bool
}

type vector struct{ X, Y, Z float64 }

func (a vector) add(b vector) vector    { return vector{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func (a vector) sub(b vector) vector    { return vector{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func (a vector) scale(s float64) vector { return vector{a.X * s, a.Y * s, a.Z * s} }
func (a vector) dot(b vector) float64   { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func (a vector) length() float64        { return math.Sqrt(a.dot(a)) }

func ecef(lon, lat, height float64) vector {
	lon *= math.Pi / 180
	lat *= math.Pi / 180
	sinLat, cosLat := math.Sincos(lat)
	sinLon, cosLon := math.Sincos(lon)
	e2 := 1 - earthB*earthB/(earthA*earthA)
	n := earthA / math.Sqrt(1-e2*sinLat*sinLat)
	return vector{(n + height) * cosLat * cosLon, (n + height) * cosLat * sinLon, (n*(1-e2) + height) * sinLat}
}

// horizonPoint follows Cesium's ellipsoid-scaled horizon construction. Shrink
// the ellipsoid when heights are negative, matching Cesium's under-ellipsoid
// visibility test. A globe-spanning root cannot have a finite culling point.
// Algorithm: CesiumJS Core/EllipsoidalOccluder.js, computeMagnitude.
func horizonPoint(points []vector, direction vector, minimumHeight float64) (vector, bool) {
	a, b := earthA, earthB
	if minimumHeight < 0 {
		a += minimumHeight
		b += minimumHeight
	}
	scale := func(v vector) vector { return vector{v.X / a, v.Y / a, v.Z / b} }
	d := scale(direction)
	if d.length() < 1e-12 {
		return vector{}, false
	}
	d = d.scale(1 / d.length())
	maximum := 0.0
	for _, p := range points {
		p = scale(p)
		m := p.length()
		if m == 0 {
			return vector{}, false
		}
		unit := p.scale(1 / m)
		cosAlpha := max(-1.0, min(1.0, unit.dot(d)))
		sinAlpha := math.Sqrt(max(0, 1-cosAlpha*cosAlpha))
		m = max(1, m)
		denom := (cosAlpha - sinAlpha*math.Sqrt(m*m-1)) / m
		if denom <= 0 {
			return vector{}, false
		}
		maximum = max(maximum, 1/denom)
	}
	if maximum == 0 || math.IsInf(maximum, 0) || math.IsNaN(maximum) {
		return vector{}, false
	}
	return d.scale(maximum), true
}

// WriteQuantizedMesh writes one deterministic gzip member. Most tiles have a
// finite standard horizon point. For globe-spanning coarse tiles (levels 0/1)
// where no such point exists, IEEE NaN disables only CesiumJS horizon culling;
// positions, heights and bounding spheres remain finite. This compatibility
// behaviour must be verified against the bundled CesiumJS and is not a claim
// that other quantized-mesh engines support this sentinel.
//
// A root covers both sides of the globe, so its scaled vertices do not fit in
// the positive hemisphere required by the horizon construction. Zero is not a
// safe substitute: Cesium would interpret it as an occluded point. CesiumJS
// 1.147's EllipsoidalOccluder compares the supplied point with its horizon using
// floating-point inequalities; a NaN point makes the occlusion result false.
// This is intentionally restricted to coarse meshes whose calculation fails.
func WriteQuantizedMesh(w io.Writer, grid *Grid) (Stats, error) {
	var stats Stats
	if grid == nil || len(grid.Heights) != GridSize*GridSize {
		return stats, errors.New("terrain grid must be 65x65")
	}
	if err := grid.Tile.Validate(); err != nil {
		return stats, err
	}
	low, high := math.Inf(1), math.Inf(-1)
	for _, h := range grid.Heights {
		if math.IsNaN(h) || math.IsInf(h, 0) || h <= -earthB {
			return stats, errors.New("terrain grid has invalid elevation")
		}
		low = min(low, h)
		high = max(high, h)
	}
	minimum, maximum := float32(low), float32(high)
	if math.IsInf(float64(minimum), 0) || math.IsInf(float64(maximum), 0) {
		return stats, errors.New("terrain height exceeds Float32 range")
	}
	if float64(minimum) > low {
		minimum = math.Nextafter32(minimum, float32(math.Inf(-1)))
	}
	if float64(maximum) < high {
		maximum = math.Nextafter32(maximum, float32(math.Inf(1)))
	}
	low, high = float64(minimum), float64(maximum)
	stats = Stats{MinimumHeight: low, MaximumHeight: high, Vertices: GridSize * GridSize, Triangles: (GridSize - 1) * (GridSize - 1) * 2}
	// High-water-mark indices require first appearances to introduce vertices in
	// increasing order. Remap the regular grid in triangle traversal order.
	remap := make([]int, stats.Vertices)
	for i := range remap {
		remap[i] = -1
	}
	order := make([]int, 0, stats.Vertices)
	indices := make([]uint16, 0, stats.Triangles*3)
	appendVertex := func(old int) {
		if remap[old] < 0 {
			remap[old] = len(order)
			order = append(order, old)
		}
		indices = append(indices, uint16(remap[old]))
	}
	for y := 0; y < GridSize-1; y++ {
		for x := 0; x < GridSize-1; x++ {
			nw := y*GridSize + x
			ne := nw + 1
			sw := nw + GridSize
			se := sw + 1
			for _, v := range []int{nw, sw, ne, ne, sw, se} {
				appendVertex(v)
			}
		}
	}
	bounds := grid.Tile.Bounds()
	world := float64(int64(1) << grid.Tile.Z)
	coordinates := [3][]uint16{make([]uint16, stats.Vertices), make([]uint16, stats.Vertices), make([]uint16, stats.Vertices)}
	points := make([]vector, stats.Vertices)
	minimumPoint := vector{math.Inf(1), math.Inf(1), math.Inf(1)}
	maximumPoint := vector{math.Inf(-1), math.Inf(-1), math.Inf(-1)}
	for index, old := range order {
		x, y := old%GridSize, old/GridSize
		u := float64(x) / (GridSize - 1)
		lat := latitude((float64(grid.Tile.Y) + float64(y)/(GridSize-1)) / world)
		v := (lat - bounds[1]) / (bounds[3] - bounds[1])
		h := 0.0
		if high > low {
			h = (grid.Heights[old] - low) / (high - low)
		}
		coordinates[0][index] = uint16(math.Round(u * quantMax))
		coordinates[1][index] = uint16(math.Round(max(0, min(1, v)) * quantMax))
		coordinates[2][index] = uint16(math.Round(max(0, min(1, h)) * quantMax))
		// Bounds must contain the decoded vertices, including quantization error.
		lon := bounds[0] + float64(coordinates[0][index])/quantMax*(bounds[2]-bounds[0])
		lat = bounds[1] + float64(coordinates[1][index])/quantMax*(bounds[3]-bounds[1])
		height := low + float64(coordinates[2][index])/quantMax*(high-low)
		p := ecef(lon, lat, height)
		points[index] = p
		minimumPoint = vector{min(minimumPoint.X, p.X), min(minimumPoint.Y, p.Y), min(minimumPoint.Z, p.Z)}
		maximumPoint = vector{max(maximumPoint.X, p.X), max(maximumPoint.Y, p.Y), max(maximumPoint.Z, p.Z)}
	}
	center := minimumPoint.add(maximumPoint).scale(0.5)
	radius := 0.0
	for _, p := range points {
		radius = max(radius, p.sub(center).length())
	}
	radius += 0.01
	direction := ecef((bounds[0]+bounds[2])/2, (bounds[1]+bounds[3])/2, 0)
	horizon, valid := horizonPoint(points, direction, low)
	if !valid {
		if grid.Tile.Z > 1 {
			return stats, fmt.Errorf("cannot calculate terrain horizon point for %+v", grid.Tile)
		}
		horizon = vector{math.NaN(), math.NaN(), math.NaN()}
		stats.HorizonDisabled = true
	}
	// Append fixed-width fields directly: binary.Write per scalar creates tens
	// of thousands of tiny allocations for every mesh.
	payload := make([]byte, 0, 88+4+stats.Vertices*6+4+len(indices)*2+4*(4+GridSize*2))
	writeVector := func(v vector) {
		payload = binary.LittleEndian.AppendUint64(payload, math.Float64bits(v.X))
		payload = binary.LittleEndian.AppendUint64(payload, math.Float64bits(v.Y))
		payload = binary.LittleEndian.AppendUint64(payload, math.Float64bits(v.Z))
	}
	writeVector(center)
	payload = binary.LittleEndian.AppendUint32(payload, math.Float32bits(minimum))
	payload = binary.LittleEndian.AppendUint32(payload, math.Float32bits(maximum))
	writeVector(center)
	payload = binary.LittleEndian.AppendUint64(payload, math.Float64bits(radius))
	writeVector(horizon)
	payload = binary.LittleEndian.AppendUint32(payload, uint32(stats.Vertices))
	for _, values := range coordinates {
		previous := int32(0)
		for _, v := range values {
			delta := int32(v) - previous
			payload = binary.LittleEndian.AppendUint16(payload, uint16((delta<<1)^(delta>>31)))
			previous = int32(v)
		}
	}
	payload = binary.LittleEndian.AppendUint32(payload, uint32(stats.Triangles))
	highest := uint16(0)
	for _, index := range indices {
		payload = binary.LittleEndian.AppendUint16(payload, highest-index)
		if index == highest {
			highest++
		}
	}
	for edge := 0; edge < 4; edge++ {
		payload = binary.LittleEndian.AppendUint32(payload, uint32(GridSize))
		for i := 0; i < GridSize; i++ {
			var old int
			switch edge {
			case 0:
				old = i * GridSize
			case 1:
				old = (GridSize-1)*GridSize + i
			case 2:
				old = i*GridSize + GridSize - 1
			case 3:
				old = i
			}
			payload = binary.LittleEndian.AppendUint16(payload, uint16(remap[old]))
		}
	}
	gz, err := gzip.NewWriterLevel(w, gzip.BestSpeed)
	if err != nil {
		return stats, err
	}
	if _, err = gz.Write(payload); err != nil {
		_ = gz.Close()
		return stats, err
	}
	if err = gz.Close(); err != nil {
		return stats, err
	}
	return stats, nil
}
