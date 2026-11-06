package terrain

import (
	"bytes"
	"compress/gzip"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"testing"
)

// decodeMesh is intentionally a reader of the public binary format, rather
// than a reuse of the encoder's vertex ordering and delta helpers.
type decodedMesh struct {
	center, sphere, horizon [3]float64
	minimum, maximum        float32
	radius                  float64
	coordinates             [3][]int
	indices                 []int
	edges                   [4][]uint16
}

func decodeMesh(t *testing.T, data []byte) decodedMesh {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := io.ReadAll(gz)
	if err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	r := bytes.NewReader(raw)
	read := func(value any) {
		t.Helper()
		if err := binary.Read(r, binary.LittleEndian, value); err != nil {
			t.Fatal(err)
		}
	}
	var mesh decodedMesh
	read(&mesh.center)
	read(&mesh.minimum)
	read(&mesh.maximum)
	read(&mesh.sphere)
	read(&mesh.radius)
	read(&mesh.horizon)
	var vertices uint32
	read(&vertices)
	if vertices != GridSize*GridSize {
		t.Fatalf("vertex count = %d", vertices)
	}
	for axis := range mesh.coordinates {
		mesh.coordinates[axis] = make([]int, vertices)
		previous := 0
		for i := range mesh.coordinates[axis] {
			var encoded uint16
			read(&encoded)
			delta := int(encoded >> 1)
			if encoded&1 != 0 {
				delta = -delta - 1
			}
			previous += delta
			if previous < 0 || previous > 32767 {
				t.Fatalf("quantized axis %d vertex %d out of bounds: %d", axis, i, previous)
			}
			mesh.coordinates[axis][i] = previous
		}
	}
	var triangles uint32
	read(&triangles)
	if triangles != 64*64*2 {
		t.Fatalf("triangle count = %d", triangles)
	}
	mesh.indices = make([]int, triangles*3)
	highest := 0
	for i := range mesh.indices {
		var code uint16
		read(&code)
		mesh.indices[i] = highest - int(code)
		if mesh.indices[i] < 0 || mesh.indices[i] >= int(vertices) {
			t.Fatalf("triangle index %d out of bounds: %d", i, mesh.indices[i])
		}
		if code == 0 {
			highest++
		}
	}
	if highest != int(vertices) {
		t.Fatalf("high-water index introduced %d vertices, want %d", highest, vertices)
	}
	for edge := range mesh.edges {
		var count uint32
		read(&count)
		if count != GridSize {
			t.Fatalf("edge %d count = %d", edge, count)
		}
		mesh.edges[edge] = make([]uint16, count)
		read(mesh.edges[edge])
	}
	if r.Len() != 0 {
		t.Fatalf("%d unexpected bytes after edge lists", r.Len())
	}
	return mesh
}

func meshFixture(id TileID) *Grid {
	grid := &Grid{Tile: id, Heights: make([]float64, GridSize*GridSize)}
	for y := 0; y < GridSize; y++ {
		for x := 0; x < GridSize; x++ {
			grid.Heights[y*GridSize+x] = float64(x)*2.125 + float64(y)*3.5 - 30.25
		}
	}
	return grid
}

func TestQuantizedMeshBinaryGeometryAndMercatorV(t *testing.T) {
	grid := meshFixture(TileID{5, 19, 6})
	var buf bytes.Buffer
	stats, err := WriteQuantizedMesh(&buf, grid)
	if err != nil {
		t.Fatal(err)
	}
	if stats.HorizonDisabled || stats.Vertices != 4225 || stats.Triangles != 8192 {
		t.Fatalf("unexpected mesh stats: %+v", stats)
	}
	mesh := decodeMesh(t, buf.Bytes())
	bounds := grid.Tile.Bounds()
	seen := make(map[[2]int]bool)
	for i, u := range mesh.coordinates[0] {
		v, h := mesh.coordinates[1][i], mesh.coordinates[2][i]
		x := int(math.Round(float64(u) / 32767 * 64))
		lat := bounds[1] + float64(v)/32767*(bounds[3]-bounds[1])
		mercatorY := (1 - math.Asinh(math.Tan(lat*math.Pi/180))/math.Pi) / 2
		y := int(math.Round((mercatorY*32 - float64(grid.Tile.Y)) * 64))
		if x < 0 || x >= GridSize || y < 0 || y >= GridSize || seen[[2]int{x, y}] {
			t.Fatalf("vertex %d does not uniquely map to grid: %d,%d", i, x, y)
		}
		seen[[2]int{x, y}] = true
		actualLat := math.Atan(math.Sinh(math.Pi*(1-2*(float64(grid.Tile.Y)+float64(y)/64)/32))) * 180 / math.Pi
		if math.Abs(actualLat-lat) > (bounds[3]-bounds[1])/32767/2+1e-10 {
			t.Fatalf("Mercator sample row %d interpreted as linear latitude", y)
		}
		height := float64(mesh.minimum) + float64(h)/32767*(float64(mesh.maximum)-float64(mesh.minimum))
		if math.Abs(height-grid.Heights[y*GridSize+x]) > (float64(mesh.maximum)-float64(mesh.minimum))/32767/2+1e-8 {
			t.Fatalf("height mismatch at %d,%d: %v", x, y, height)
		}
		lon := bounds[0] + float64(u)/32767*(bounds[2]-bounds[0])
		p := ecef(lon, lat, height)
		dx, dy, dz := p.X-mesh.sphere[0], p.Y-mesh.sphere[1], p.Z-mesh.sphere[2]
		if distance := math.Sqrt(dx*dx + dy*dy + dz*dz); distance > mesh.radius+1e-8 {
			t.Fatalf("vertex outside bounding sphere: %v > %v", distance, mesh.radius)
		}
	}
	for i := 0; i < len(mesh.indices); i += 3 {
		a, b, c := mesh.indices[i], mesh.indices[i+1], mesh.indices[i+2]
		ux, uy := mesh.coordinates[0][b]-mesh.coordinates[0][a], mesh.coordinates[1][b]-mesh.coordinates[1][a]
		vx, vy := mesh.coordinates[0][c]-mesh.coordinates[0][a], mesh.coordinates[1][c]-mesh.coordinates[1][a]
		if ux*vy-uy*vx <= 0 {
			t.Fatalf("triangle %d not counterclockwise", i/3)
		}
	}
	for edge, indices := range mesh.edges {
		axis, want := 0, 0
		switch edge {
		case 1:
			axis = 1
		case 2:
			want = 32767
		case 3:
			axis, want = 1, 32767
		}
		for _, index := range indices {
			if int(index) >= len(mesh.coordinates[axis]) || mesh.coordinates[axis][index] != want {
				t.Fatalf("edge %d contains non-boundary vertex %d", edge, index)
			}
		}
	}
	for _, v := range mesh.horizon {
		if math.IsNaN(v) || math.IsInf(v, 0) {
			t.Fatal("ordinary tile horizon must be finite")
		}
	}
	var repeated bytes.Buffer
	if _, err := WriteQuantizedMesh(&repeated, grid); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf.Bytes(), repeated.Bytes()) {
		t.Fatal("mesh gzip output is not deterministic")
	}
}

func TestRootAndCoarseMeshHorizonIsDisabledOnlyWhenNecessary(t *testing.T) {
	for z := 0; z <= 2; z++ {
		for y := 0; y < 1<<z; y++ {
			for x := 0; x < 1<<z; x++ {
				grid := meshFixture(TileID{z, x, y})
				var buf bytes.Buffer
				stats, err := WriteQuantizedMesh(&buf, grid)
				if err != nil {
					t.Fatalf("tile %+v: %v", grid.Tile, err)
				}
				mesh := decodeMesh(t, buf.Bytes())
				if z == 0 && !stats.HorizonDisabled {
					t.Fatal("whole globe root cannot have a finite horizon culling point")
				}
				if z > 1 && stats.HorizonDisabled {
					t.Fatalf("horizon disabled outside coarse levels: %+v", grid.Tile)
				}
				for _, v := range mesh.horizon {
					if math.IsNaN(v) != stats.HorizonDisabled || math.IsInf(v, 0) {
						t.Fatalf("horizon sentinel mismatch: %+v", mesh.horizon)
					}
				}
				for _, values := range [][3]float64{mesh.center, mesh.sphere} {
					for _, v := range values {
						if math.IsNaN(v) || math.IsInf(v, 0) {
							t.Fatal("mesh position/sphere must always remain finite")
						}
					}
				}
			}
		}
	}
}

func TestFlatMeshPreservesRealZeroAndNegativeHeight(t *testing.T) {
	for _, height := range []float64{0, -430.125} {
		grid := meshFixture(TileID{5, 19, 6})
		for i := range grid.Heights {
			grid.Heights[i] = height
		}
		var buf bytes.Buffer
		if _, err := WriteQuantizedMesh(&buf, grid); err != nil {
			t.Fatal(err)
		}
		mesh := decodeMesh(t, buf.Bytes())
		if float64(mesh.minimum) != height || float64(mesh.maximum) != height {
			t.Fatalf("flat height %v became [%v,%v]", height, mesh.minimum, mesh.maximum)
		}
		for _, h := range mesh.coordinates[2] {
			if h != 0 {
				t.Fatal("flat height quantization must remain zero")
			}
		}
	}
}

type failedWriter struct{ err error }

func (w failedWriter) Write([]byte) (int, error) { return 0, w.err }

func TestMeshRejectsMissingHeightsAndPropagatesWriteFailure(t *testing.T) {
	grid := meshFixture(TileID{5, 19, 6})
	sentinel := errors.New("disk unavailable")
	if _, err := WriteQuantizedMesh(failedWriter{sentinel}, grid); !errors.Is(err, sentinel) {
		t.Fatalf("writer error = %v", err)
	}
	grid.Heights[123] = math.NaN()
	if _, err := WriteQuantizedMesh(io.Discard, grid); err == nil {
		t.Fatal("missing height accepted")
	}
}

func BenchmarkWriteQuantizedMesh(b *testing.B) {
	grid := meshFixture(TileID{12, 3300, 1600})
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if _, err := WriteQuantizedMesh(io.Discard, grid); err != nil {
			b.Fatal(err)
		}
	}
}
