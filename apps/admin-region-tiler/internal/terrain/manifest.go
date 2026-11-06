package terrain

import (
	"errors"
	"fmt"
	"sort"
)

// SourceMetadata describes the source data, not a guarantee of surveying
// accuracy or a conversion to a particular vertical reference system.
type SourceMetadata struct {
	Name          string `json:"name"`
	URL           string `json:"url,omitempty"`
	Encoding      string `json:"encoding"`
	VerticalDatum string `json:"verticalDatum"`
	RetrievedAt   string `json:"retrievedAt,omitempty"`
	Attribution   string `json:"attribution"`
}

type Availability struct {
	StartX int `json:"startX"`
	StartY int `json:"startY"`
	EndX   int `json:"endX"`
	EndY   int `json:"endY"`
}

type Manifest struct {
	TileJSON    string           `json:"tilejson"`
	Name        string           `json:"name"`
	Version     string           `json:"version"`
	Format      string           `json:"format"`
	Projection  string           `json:"projection"`
	Scheme      string           `json:"scheme"`
	MinZoom     int              `json:"minzoom"`
	MaxZoom     int              `json:"maxzoom"`
	Bounds      [4]float64       `json:"bounds"`
	Tiles       []string         `json:"tiles"`
	Available   [][]Availability `json:"available"`
	Attribution string           `json:"attribution"`
	Source      SourceMetadata   `json:"source"`
}

// BuildManifest only describes completed meshes. Missing parents are an error:
// publishing their descendants as available would prevent correct traversal.
func BuildManifest(name string, completed []TileID, source SourceMetadata) (Manifest, error) {
	var result Manifest
	set := make(map[TileID]bool, len(completed))
	maxZoom := 0
	for _, id := range completed {
		if err := id.Validate(); err != nil {
			return result, err
		}
		set[id] = true
		maxZoom = max(maxZoom, id.Z)
	}
	if !set[TileID{}] {
		return result, errors.New("completed terrain root is required")
	}
	for id := range set {
		if id.Z > 0 && !set[TileID{id.Z - 1, id.X / 2, id.Y / 2}] {
			return result, fmt.Errorf("terrain tile %+v has no completed parent", id)
		}
	}
	if name == "" {
		name = "Terrain"
	}
	result = Manifest{TileJSON: "2.1.0", Name: name, Version: "1.0.0", Format: "quantized-mesh-1.0", Projection: "EPSG:3857", Scheme: "tms", MaxZoom: maxZoom, Bounds: [4]float64{-180, -MaxLatitude, 180, MaxLatitude}, Tiles: []string{"{z}/{x}/{y}.terrain"}, Available: make([][]Availability, maxZoom+1), Attribution: source.Attribution, Source: source}
	for z := 0; z <= maxZoom; z++ {
		rows := make(map[int][]int)
		for id := range set {
			if id.Z == z {
				rows[id.TMSY()] = append(rows[id.TMSY()], id.X)
			}
		}
		ys := make([]int, 0, len(rows))
		for y := range rows {
			ys = append(ys, y)
		}
		sort.Ints(ys)
		// Merge horizontal runs, then adjoining rows with identical X intervals.
		active := make(map[[2]int]int)
		for _, y := range ys {
			xs := rows[y]
			sort.Ints(xs)
			current := make(map[[2]int]int)
			for i := 0; i < len(xs); {
				start, end := xs[i], xs[i]
				i++
				for i < len(xs) && xs[i] == end+1 {
					end = xs[i]
					i++
				}
				key := [2]int{start, end}
				if index, ok := active[key]; ok && result.Available[z][index].EndY == y-1 {
					result.Available[z][index].EndY = y
					current[key] = index
				} else {
					current[key] = len(result.Available[z])
					result.Available[z] = append(result.Available[z], Availability{start, y, end, y})
				}
			}
			active = current
		}
	}
	return result, nil
}
