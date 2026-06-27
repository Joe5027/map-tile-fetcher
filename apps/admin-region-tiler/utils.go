package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	log "github.com/sirupsen/logrus"
)

func validateTileResponse(body []byte, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case PNG, JPG, WEBP, "":
		if _, _, err := image.DecodeConfig(bytes.NewReader(body)); err != nil {
			return fmt.Errorf("tile image validation failed: %w", err)
		}
	}
	return nil
}

func normalizeTileData(body []byte, format string) ([]byte, error) {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "", PNG:
		if len(body) >= 4 && bytes.Equal(body[:4], []byte{0x89, 0x50, 0x4e, 0x47}) {
			return body, nil
		}

		img, _, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("convert tile to png failed: %w", err)
		}

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			return nil, fmt.Errorf("encode png failed: %w", err)
		}
		return buf.Bytes(), nil
	case JPG:
		if len(body) >= 2 && body[0] == 0xff && body[1] == 0xd8 {
			return body, nil
		}

		img, _, err := image.Decode(bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("convert tile to jpg failed: %w", err)
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
			return nil, fmt.Errorf("encode jpg failed: %w", err)
		}
		return buf.Bytes(), nil
	default:
		return body, nil
	}
}

func resolveGeoJSONPath(path string) (string, error) {
	cleaned := strings.TrimSpace(path)
	if cleaned == "" {
		return "", fmt.Errorf("geojson is required")
	}

	candidates := []string{
		filepath.Clean(cleaned),
		filepath.Clean(filepath.FromSlash(cleaned)),
	}

	if !filepath.IsAbs(cleaned) {
		if workingDir, err := os.Getwd(); err == nil {
			candidates = append(candidates,
				filepath.Join(workingDir, cleaned),
				filepath.Join(workingDir, filepath.FromSlash(cleaned)),
				filepath.Join(workingDir, "geojson", filepath.Base(cleaned)),
			)
		}
		if exePath, err := os.Executable(); err == nil {
			exeDir := filepath.Dir(exePath)
			candidates = append(candidates,
				filepath.Join(exeDir, cleaned),
				filepath.Join(exeDir, filepath.FromSlash(cleaned)),
				filepath.Join(exeDir, "geojson", filepath.Base(cleaned)),
			)
		}
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		info, err := os.Stat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		absolute, err := filepath.Abs(candidate)
		if err != nil {
			return candidate, nil
		}
		return absolute, nil
	}

	return "", fmt.Errorf("geojson file not found: %s", cleaned)
}

func saveToMBTile(tile Tile, db *sql.DB) error {
	_, err := db.Exec("insert or ignore into tiles (zoom_level, tile_column, tile_row, tile_data) values (?, ?, ?, ?);", tile.T.Z, tile.T.X, tile.flipY(), tile.C)
	if err != nil {
		return err
	}
	return nil
}

func saveToFiles(tile Tile, task *Task) error {
	dir := filepath.Join(task.File, fmt.Sprintf(`%d`, tile.T.Z), fmt.Sprintf(`%d`, tile.T.X))
	os.MkdirAll(dir, os.ModePerm)
	fileName := filepath.Join(dir, fmt.Sprintf(`%d.%s`, tile.T.Y, task.TileMap.Format))
	err := os.WriteFile(fileName, tile.C, os.ModePerm)
	if err != nil {
		return err
	}
	log.Println(fileName)
	return nil
}

func optimizeConnection(db *sql.DB) error {
	// _, err := db.Exec("PRAGMA synchronous=0")
	// if err != nil {
	// 	return err
	// }
	_, err := db.Exec("PRAGMA locking_mode=EXCLUSIVE")
	if err != nil {
		return err
	}
	_, err = db.Exec("PRAGMA journal_mode=DELETE")
	if err != nil {
		return err
	}
	return nil
}

func optimizeDatabase(db *sql.DB) error {
	_, err := db.Exec("ANALYZE;")
	if err != nil {
		return err
	}

	_, err = db.Exec("VACUUM;")
	if err != nil {
		return err
	}

	return nil
}

func loadFeature(path string) (*geojson.Feature, error) {
	resolvedPath, err := resolveGeoJSONPath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file: %w", err)
	}

	f, err := geojson.UnmarshalFeature(data)
	if err == nil {
		return f, nil
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err == nil {
		if len(fc.Features) != 1 {
			return nil, fmt.Errorf("must have 1 feature: %d", len(fc.Features))
		}
		return fc.Features[0], nil
	}

	g, err := geojson.UnmarshalGeometry(data)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal feature: %w", err)
	}

	return geojson.NewFeature(g.Geometry()), nil
}

func loadFeatureCollection(path string) (*geojson.FeatureCollection, error) {
	resolvedPath, err := resolveGeoJSONPath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file: %w", err)
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal feature collection: %w", err)
	}

	count := 0
	for i := range fc.Features {
		if fc.Features[i].Properties["name"] != "original" {
			fc.Features[count] = fc.Features[i]
			count++
		}
	}
	fc.Features = fc.Features[:count]

	return fc, nil
}

func loadCollection(path string) (orb.Collection, error) {
	resolvedPath, err := resolveGeoJSONPath(path)
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(resolvedPath)
	if err != nil {
		return nil, fmt.Errorf("unable to read file: %w", err)
	}

	fc, err := geojson.UnmarshalFeatureCollection(data)
	if err != nil {
		return nil, fmt.Errorf("unable to unmarshal feature collection: %w", err)
	}

	var collection orb.Collection
	for _, f := range fc.Features {
		collection = append(collection, f.Geometry)
	}

	return collection, nil
}

// output gets called if there is a test failure for debugging.
func output(name string, r *geojson.FeatureCollection) {
	f, err := loadFeature("./data/" + name + ".geojson")
	if err != nil {
		log.Fatalf("unable to load feature: %v", err)
	}
	if f.Properties == nil {
		f.Properties = make(geojson.Properties)
	}

	f.Properties["fill"] = "#FF0000"
	f.Properties["fill-opacity"] = "0.5"
	f.Properties["stroke"] = "#FF0000"
	f.Properties["name"] = "original"
	r.Append(f)

	data, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		log.Fatalf("error marshalling json: %v", err)
	}

	err = os.WriteFile("failure_"+name+".geojson", data, 0644)
	if err != nil {
		log.Fatalf("write file failure: %v", err)
	}
}

// output gets called if there is a test failure for debugging.
func output2(name string, r *geojson.FeatureCollection, wg *sync.WaitGroup) {
	defer wg.Done()
	data, err := json.MarshalIndent(r, "", " ")
	if err != nil {
		log.Fatalf("error marshalling json: %v", err)
	}

	err = os.WriteFile(name+".geojson", data, 0644)
	if err != nil {
		log.Fatalf("write file failure: %v", err)
	}
}

func getZoomCount(g orb.Geometry, minz int, maxz int) map[int]int64 {

	info := make(map[int]int64)
	for z := minz; z <= maxz; z++ {
		info[z] = tilecover.GeometryCount(g, maptile.Zoom(z))
	}
	return info
}
