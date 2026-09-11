package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"github.com/paulmach/orb"
	"github.com/paulmach/orb/geojson"
)

const (
	directoryPermissions = 0o755
	filePermissions      = 0o644
	maxTilePixels        = 16_777_216
)

func validateTileResponse(body []byte, format string) error {
	switch strings.ToLower(strings.TrimSpace(format)) {
	case PNG, JPG, WEBP, "":
		config, _, err := image.DecodeConfig(bytes.NewReader(body))
		if err != nil {
			return fmt.Errorf("tile image validation failed: %w", err)
		}
		if config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > maxTilePixels {
			return fmt.Errorf("tile image dimensions exceed %d pixels", maxTilePixels)
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

// resolveManagedTaskGeoJSONPath only permits GeoJSON files that belong to the
// application. Task records are persistent and must not turn an arbitrary local
// path into a later API response or download input.
func resolveManagedTaskGeoJSONPath(path string) (string, error) {
	resolved, err := resolveGeoJSONPath(path)
	if err != nil {
		return "", err
	}
	for _, root := range []string{"geojson", filepath.Join(defaultDataDir, "generated-areas")} {
		if _, err := ensurePathWithinRoot(resolved, root, false); err == nil {
			return resolved, nil
		}
	}
	return "", errors.New("task geojson must be located in geojson or data/generated-areas")
}

func saveToMBTile(tile Tile, db *sql.DB) error {
	_, err := db.Exec("insert or replace into tiles (zoom_level, tile_column, tile_row, tile_data) values (?, ?, ?, ?);", tile.T.Z, tile.T.X, tile.flipY(), tile.C)
	if err != nil {
		return err
	}
	return nil
}

func saveToFiles(tile Tile, task *Task) error {
	dir := filepath.Join(task.File, fmt.Sprintf(`%d`, tile.T.Z), fmt.Sprintf(`%d`, tile.T.X))
	if err := os.MkdirAll(dir, directoryPermissions); err != nil {
		return err
	}
	y := tile.T.Y
	if strings.EqualFold(task.TileMap.Schema, "tms") {
		y = tile.flipY()
	}
	fileName := filepath.Join(dir, fmt.Sprintf(`%d.%s`, y, task.TileMap.Format))
	err := os.WriteFile(fileName, tile.C, filePermissions)
	if err != nil {
		return err
	}
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
