package main

import (
	"compress/gzip"
	"container/list"
	"context"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paulmach/orb/maptile"
	"tiler/internal/terrain"
)

const defaultTerrariumURL = "https://s3.amazonaws.com/elevation-tiles-prod/terrarium/{z}/{x}/{y}.png"

// SourceMetadata is persisted with each child's levels so worker restarts do
// not depend on a later edit to the configured source catalogue.
type SourceMetadata struct {
	Kind          string `json:"kind" mapstructure:"kind"`
	Encoding      string `json:"encoding,omitempty" mapstructure:"encoding"`
	Projection    string `json:"projection,omitempty" mapstructure:"projection"`
	MinZoom       int    `json:"minZoom" mapstructure:"min_zoom"`
	MaxZoom       int    `json:"maxZoom" mapstructure:"max_zoom"`
	DefaultZoom   int    `json:"defaultZoom,omitempty" mapstructure:"default_zoom"`
	Attribution   string `json:"attribution,omitempty" mapstructure:"attribution"`
	VerticalDatum string `json:"verticalDatum,omitempty" mapstructure:"vertical_datum"`
}

func defaultTerrainSource() *SourceMetadata {
	return &SourceMetadata{Kind: "elevation", Encoding: "terrarium", Projection: "EPSG:3857", MinZoom: 0, MaxZoom: terrain.MaxZoom, DefaultZoom: 14,
		Attribution:   "Terrain Tiles: Mapzen / Tilezen; underlying datasets and attribution: https://github.com/tilezen/joerd/blob/master/docs/attribution.md",
		VerticalDatum: "Source-dependent vertical reference; no datum transformation. For visualization, not surveying or flight planning."}
}

func normalizedSourceMetadata(rawURL string, metadata *SourceMetadata) *SourceMetadata {
	if metadata == nil {
		if rawURL == defaultTerrariumURL {
			return defaultTerrainSource()
		}
		return nil
	}
	copy := *metadata
	copy.Kind, copy.Encoding = strings.ToLower(strings.TrimSpace(copy.Kind)), strings.ToLower(strings.TrimSpace(copy.Encoding))
	copy.Projection = strings.ToUpper(strings.TrimSpace(copy.Projection))
	if copy.Kind == "elevation" && copy.Encoding == "terrarium" && copy.Projection == "" {
		copy.Projection = "EPSG:3857"
	}
	return &copy
}

func cloneLevelsWithSource(levels []LevelConfig, source *SourceMetadata) []LevelConfig {
	result := append([]LevelConfig(nil), levels...)
	for i := range result {
		if source != nil {
			copy := *source
			result[i].Source = &copy
		}
		if result[i].BBox != nil {
			copy := *result[i].BBox
			result[i].BBox = &copy
		}
	}
	return result
}

func validateTerrainRequest(output string, sources []SourceRequest, levels []LevelConfig) error {
	if output != "cesium" {
		return nil
	}
	for _, source := range sources {
		if source.Format != PNG || source.Schema != "xyz" || source.Metadata == nil || source.Metadata.Kind != "elevation" || source.Metadata.Encoding != "terrarium" {
			return errors.New("Cesium terrain requires an XYZ Terrarium PNG elevation source")
		}
		if source.Metadata.Projection != "EPSG:3857" || source.Metadata.MinZoom != 0 {
			return errors.New("Cesium terrain requires EPSG:3857 source data including the level 0 root")
		}
		for _, level := range levels {
			if level.URL != "" && level.URL != source.URL {
				return errors.New("Cesium terrain levels must use the same source URL for ancestor and edge sampling")
			}
			if level.MinZoom < 0 || level.MaxZoom > terrain.MaxZoom || level.MinZoom < source.Metadata.MinZoom || level.MaxZoom > source.Metadata.MaxZoom {
				return errors.New("Terrarium terrain zoom must be within the source range (0–15)")
			}
		}
	}
	return nil
}

func configureTerrainTask(task *Task, req CreateTaskRequest) error {
	if outputFormatFromRequest(req) != "cesium" {
		return nil
	}
	metadata := normalizedSourceMetadata(req.URL, nil)
	for _, level := range req.Levels {
		if level.Source != nil {
			metadata = normalizedSourceMetadata(req.URL, level.Source)
			break
		}
	}
	source := SourceRequest{URL: req.URL, Format: req.Format, Schema: req.Schema, Metadata: metadata}
	levels := make([]LevelConfig, len(req.Levels))
	for i, level := range req.Levels {
		levels[i] = LevelConfig(level)
	}
	if err := validateTerrainRequest("cesium", []SourceRequest{source}, levels); err != nil {
		return err
	}
	task.TileMap.Metadata = metadata
	task.expandExpected = func(job TileJob, emit func(TileJob) error) error {
		ancestors, err := terrain.Ancestors([]terrain.TileID{{Z: int(job.Tile.Z), X: int(job.Tile.X), Y: int(job.Tile.Y)}})
		if err != nil {
			return err
		}
		for _, ancestor := range ancestors {
			halo, err := terrain.SourceTiles(ancestor)
			if err != nil {
				return err
			}
			for _, id := range halo {
				if err := emit(TileJob{Tile: maptile.New(uint32(id.X), uint32(id.Y), maptile.Zoom(id.Z)), URL: job.URL}); err != nil {
					return err
				}
			}
		}
		return nil
	}
	task.deriveArtifacts = deriveTerrainArtifacts
	return nil
}

// Keep targets separate from the raw download catalogue: halo rasters are not
// output meshes. SQLite keeps enumeration and availability bounded in memory.
func openTerrainTargets(task *Task, path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", filepath.ToSlash(path))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err = db.Exec(`PRAGMA journal_mode=WAL; PRAGMA cache_size=-2048; CREATE TABLE IF NOT EXISTS targets(z INTEGER,x INTEGER,y INTEGER,available INTEGER NOT NULL DEFAULT 0,PRIMARY KEY(z,x,y)) WITHOUT ROWID; CREATE TABLE IF NOT EXISTS manifest(complete INTEGER NOT NULL)`); err != nil {
		db.Close()
		return nil, err
	}
	var ready int
	if err = db.QueryRow(`SELECT complete FROM manifest`).Scan(&ready); err == nil && ready == 1 {
		_, err = db.Exec(`UPDATE targets SET available=0`)
		if err != nil {
			db.Close()
			return nil, err
		}
		return db, err
	}
	tx, err := db.Begin()
	if err != nil {
		db.Close()
		return nil, err
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT OR IGNORE INTO targets(z,x,y) VALUES(?,?,?)`)
	if err == nil {
		defer insert.Close()
		for _, layer := range task.Layers {
			err = walkLayer(task.ctx, layer, func(tile maptile.Tile) error {
				ids, err := terrain.Ancestors([]terrain.TileID{{Z: int(tile.Z), X: int(tile.X), Y: int(tile.Y)}})
				if err != nil {
					return err
				}
				for _, id := range ids {
					if _, err := insert.Exec(id.Z, id.X, id.Y); err != nil {
						return err
					}
				}
				return nil
			})
			if err != nil {
				break
			}
		}
	}
	if err == nil {
		_, err = tx.Exec(`DELETE FROM manifest; INSERT INTO manifest VALUES(1)`)
	}
	if err == nil {
		err = tx.Commit()
	}
	if err != nil {
		tx.Rollback()
		db.Close()
		return nil, err
	}
	return db, nil
}

type terrainRasterEntry struct {
	id     terrain.TileID
	raster *terrain.Raster
}

func terrainRasterLoader(task *Task, run *TaskRunRecord) terrain.Loader {
	cache := make(map[terrain.TileID]*list.Element)
	lru := list.New()
	return func(ctx context.Context, id terrain.TileID) (*terrain.Raster, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if item := cache[id]; item != nil {
			lru.MoveToFront(item)
			return item.Value.(terrainRasterEntry).raster, nil
		}
		path := filepath.Join(run.OutputPath, fmt.Sprint(id.Z), fmt.Sprint(id.X), fmt.Sprintf("%d.png", id.Y))
		file, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		raster, decodeErr := terrain.DecodeTerrarium(file)
		closeErr := file.Close()
		if decodeErr != nil {
			// An image may pass the raster downloader's PNG check but contain no
			// valid elevation. Remove only that coordinate from raw coverage so
			// publication cannot incorrectly resolve its retryable source failure.
			if store != nil {
				result, err := store.db.Exec(`DELETE FROM run_coverage WHERE run_id=? AND z=? AND x=? AND y=?`, run.ID, id.Z, id.X, id.Y)
				if err != nil {
					return nil, err
				}
				if count, _ := result.RowsAffected(); count > 0 {
					task.recordTileFailure(TileJob{Tile: maptile.New(uint32(id.X), uint32(id.Y), maptile.Zoom(id.Z)), URL: task.TileMap.URL}, decodeErr, true)
				}
			}
			return nil, decodeErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		cache[id] = lru.PushFront(terrainRasterEntry{id, raster})
		// 12 x 256² Float64 rasters, about 6 MiB per converting child.
		if lru.Len() > 12 {
			last := lru.Back()
			delete(cache, last.Value.(terrainRasterEntry).id)
			lru.Remove(last)
		}
		return raster, nil
	}
}

func atomicTerrainWrite(path string, write func(io.Writer) error) error {
	if err := os.MkdirAll(filepath.Dir(path), directoryPermissions); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".terrain-*.partial")
	if err != nil {
		return err
	}
	temporary := f.Name()
	defer os.Remove(temporary)
	err = write(f)
	closeErr := f.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// Only complete CRC-valid meshes generated by this fixed-grid encoder are
// reusable. A killed writer leaves a .partial file, never a valid mesh path.
func validTerrainMesh(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return false
	}
	defer gz.Close()
	data, err := io.ReadAll(io.LimitReader(gz, 100000))
	const vertices, triangles = terrain.GridSize * terrain.GridSize, (terrain.GridSize - 1) * (terrain.GridSize - 1) * 2
	const size = 88 + 4 + vertices*6 + 4 + triangles*6 + 4*(4+terrain.GridSize*2)
	if err != nil || len(data) != size || binary.LittleEndian.Uint32(data[88:92]) != vertices {
		return false
	}
	minHeight, maxHeight := math.Float32frombits(binary.LittleEndian.Uint32(data[24:28])), math.Float32frombits(binary.LittleEndian.Uint32(data[28:32]))
	if math.IsNaN(float64(minHeight)) || math.IsNaN(float64(maxHeight)) || minHeight > maxHeight {
		return false
	}
	pos := 92 + vertices*6
	if binary.LittleEndian.Uint32(data[pos:]) != triangles {
		return false
	}
	pos += 4 + triangles*6
	for edge := 0; edge < 4; edge++ {
		if binary.LittleEndian.Uint32(data[pos:]) != terrain.GridSize {
			return false
		}
		pos += 4 + terrain.GridSize*2
	}
	return true
}

func deriveTerrainArtifacts(plan *TaskRecord, task *Task, run *TaskRunRecord) error {
	directory, err := managedRunDirectory(plan, run.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(directory, directoryPermissions); err != nil {
		return err
	}
	db, err := openTerrainTargets(task, filepath.Join(directory, "terrain-targets.sqlite"))
	if err != nil {
		return err
	}
	defer db.Close()
	output := filepath.Join(directory, "terrain")
	if err := cleanTerrainPartials(output); err != nil {
		return err
	}
	state := &IntegrityState{Status: "incomplete", RunID: run.ID}
	task.derivedIntegrity = state
	if err := db.QueryRow(`SELECT COUNT(*) FROM targets`).Scan(&state.Expected); err != nil {
		return err
	}
	state.Missing = state.Expected
	load := terrainRasterLoader(task, run)
	var current int64
	var firstError error
	var last terrain.TileID
	first := true
	for {
		// Keyset batches allow updates on the same single-connection SQLite DB.
		query := `SELECT z,x,y FROM targets WHERE (z,x,y)>(?,?,?) ORDER BY z,x,y LIMIT 128`
		start := last
		if first {
			start.Z = -1
			first = false
		}
		rows, err := db.Query(query, start.Z, start.X, start.Y)
		if err != nil {
			return err
		}
		batch := make([]terrain.TileID, 0, 128)
		for rows.Next() {
			var id terrain.TileID
			if err := rows.Scan(&id.Z, &id.X, &id.Y); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, id)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if len(batch) == 0 {
			break
		}
		for _, id := range batch {
			if err := task.waitIfPaused(); err != nil {
				return err
			}
			current++
			last = id
			if store != nil && (current == 1 || current%32 == 0 || current == state.Expected) {
				if err := store.setRunPhase(run.ID, "converting", current-1, state.Expected); err != nil {
					return err
				}
			}
			if id.Z > 0 {
				var parentAvailable int
				if err := db.QueryRow(`SELECT available FROM targets WHERE z=? AND x=? AND y=?`, id.Z-1, id.X/2, id.Y/2).Scan(&parentAvailable); err != nil {
					return err
				}
				if parentAvailable == 0 {
					continue
				}
			}
			path := filepath.Join(output, filepath.FromSlash(id.Path()))
			if !validTerrainMesh(path) {
				grid, err := terrain.SampleGrid(task.ctx, id, load)
				if err != nil {
					if errors.Is(err, context.Canceled) {
						return err
					}
					if firstError == nil {
						firstError = fmt.Errorf("terrain %d/%d/%d: %w", id.Z, id.X, id.Y, err)
					}
					continue
				}
				// This encoder's 65x65 mesh is 75,134 bytes before gzip. Check
				// only one temporary mesh's conservative gzip upper bound.
				if err := requireOutputSpace(directory, "terrain conversion", 76*1024); err != nil {
					return err
				}
				if err := atomicTerrainWrite(path, func(w io.Writer) error { _, err := terrain.WriteQuantizedMesh(w, grid); return err }); err != nil {
					return err
				}
			}
			if _, err := db.Exec(`UPDATE targets SET available=1 WHERE z=? AND x=? AND y=?`, id.Z, id.X, id.Y); err != nil {
				return err
			}
			state.Available++
		}
	}
	state.Missing = state.Expected - state.Available
	if firstError != nil {
		state.Error = firstError.Error()
	}
	var root int
	if err := db.QueryRow(`SELECT available FROM targets WHERE z=0 AND x=0 AND y=0`).Scan(&root); err != nil {
		return err
	}
	if root == 0 {
		return fmt.Errorf("terrain root unavailable; previous artifact preserved: %w", firstError)
	}
	if state.Missing == 0 {
		state.Status = "complete"
	}
	manifest, bounds, err := terrainManifestFromCatalogue(plan, task, db)
	if err != nil {
		return err
	}
	info := terrainPreviewInfo{Bounds: bounds, Source: task.TileMap.Metadata, Expected: state.Expected, Available: state.Available}
	for name, value := range map[string]any{"layer.json": manifest, "terrain-info.json": info} {
		if err := atomicTerrainWrite(filepath.Join(output, name), func(w io.Writer) error { enc := json.NewEncoder(w); enc.SetIndent("", "  "); return enc.Encode(value) }); err != nil {
			return err
		}
	}
	readme := terrainPackageReadme + "\nAttribution: " + manifest.Attribution + "\nVertical reference: " + manifest.Source.VerticalDatum + "\n"
	if err := atomicTerrainWrite(filepath.Join(output, "README.txt"), func(w io.Writer) error { _, err := io.WriteString(w, readme); return err }); err != nil {
		return err
	}
	task.artifactDirectory = output
	if state.Missing > 0 {
		run.ErrorMessage = fmt.Sprintf("%d/%d terrain meshes available; %s", state.Available, state.Expected, state.Error)
	}
	if store != nil {
		return store.setRunPhase(run.ID, "converting", current, state.Expected)
	}
	return nil
}

// A terminated process can leave an unfinished atomic-write file. Only this
// converter's temporary names inside its managed output tree are discarded.
func cleanTerrainPartials(directory string) error {
	return filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
		if os.IsNotExist(err) && path == directory {
			return nil
		}
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), ".terrain-") && strings.HasSuffix(entry.Name(), ".partial") {
			return os.Remove(path)
		}
		return nil
	})
}

func terrainManifestFromCatalogue(plan *TaskRecord, task *Task, db *sql.DB) (terrain.Manifest, [4]float64, error) {
	metadata := task.TileMap.Metadata
	publicURL := task.TileMap.URL
	if parsed, err := url.Parse(publicURL); err == nil {
		parsed.RawQuery = ""
		parsed.User = nil
		publicURL = parsed.String()
	}
	source := terrain.SourceMetadata{Name: plan.SourceName, URL: publicURL, Encoding: "Terrarium", RetrievedAt: time.Now().UTC().Format(time.RFC3339), Attribution: metadata.Attribution, VerticalDatum: metadata.VerticalDatum}
	manifest, err := terrain.BuildManifest(plan.Name, []terrain.TileID{{}}, source)
	var bounds [4]float64
	if err != nil {
		return manifest, bounds, err
	}
	var maxZoom, minX, maxX, minY, maxY int
	if err := db.QueryRow(`SELECT MAX(z) FROM targets WHERE available=1`).Scan(&maxZoom); err != nil {
		return manifest, bounds, err
	}
	if err := db.QueryRow(`SELECT MIN(x),MAX(x),MIN(y),MAX(y) FROM targets WHERE z=?`, maxZoom).Scan(&minX, &maxX, &minY, &maxY); err != nil {
		return manifest, bounds, err
	}
	nw, se := (terrain.TileID{Z: maxZoom, X: minX, Y: minY}).Bounds(), (terrain.TileID{Z: maxZoom, X: maxX, Y: maxY}).Bounds()
	bounds = [4]float64{nw[0], se[1], se[2], nw[3]}
	manifest.MaxZoom = maxZoom
	manifest.Available = make([][]terrain.Availability, maxZoom+1)
	rows, err := db.Query(`SELECT z,x,(1 << z)-1-y FROM targets WHERE available=1 ORDER BY z,3,x`)
	if err != nil {
		return manifest, bounds, err
	}
	defer rows.Close()
	zNow, yNow, start, end := -1, -1, -1, -1
	active := make(map[[2]int]int)
	current := make(map[[2]int]int)
	flush := func() {
		if start < 0 {
			return
		}
		key := [2]int{start, end}
		if index, ok := active[key]; ok && manifest.Available[zNow][index].EndY == yNow-1 {
			manifest.Available[zNow][index].EndY = yNow
			current[key] = index
		} else {
			current[key] = len(manifest.Available[zNow])
			manifest.Available[zNow] = append(manifest.Available[zNow], terrain.Availability{StartX: start, StartY: yNow, EndX: end, EndY: yNow})
		}
	}
	for rows.Next() {
		var z, x, y int
		if err := rows.Scan(&z, &x, &y); err != nil {
			return manifest, bounds, err
		}
		if z != zNow || y != yNow {
			flush()
			active = current
			current = make(map[[2]int]int)
			if z != zNow {
				active = make(map[[2]int]int)
			}
			zNow, yNow, start, end = z, y, x, x
		} else if x == end+1 {
			end = x
		} else {
			flush()
			start, end = x, x
		}
	}
	flush()
	return manifest, bounds, rows.Err()
}

const terrainPackageReadme = `Cesium offline terrain / Cesium 离线地形

This package contains real elevation quantized-mesh-1.0, EPSG:3857, TMS.
65x65 vertices sample the Terrarium raster every 4 pixels; this is visualization
terrain, not a lossless DEM or a surveying / flight-planning product.
Only completed tiles with completed ancestors are advertised in layer.json.
Missing source data is never replaced with artificial sea-level elevation.
The globe-spanning coarse meshes use a CesiumJS-specific NaN horizon-culling
sentinel when no finite point exists. Target runtime: CesiumJS 1.147.0.

Serve this directory over HTTP, not file://. The .terrain files are gzip members:
Content-Type: application/vnd.quantized-mesh
Content-Encoding: gzip
Do not gzip them a second time. JSON files use application/json without gzip.
For cross-origin hosting, also allow the viewer's origin through CORS.

Nginx example (a dedicated terrain location):
location ~ ^/terrain/.*\.terrain$ {
    root /srv;
    default_type application/vnd.quantized-mesh;
    add_header Content-Encoding gzip;
    gzip off;
}

Use the application's locally bundled /static/vendor/cesium resources, or copy
that complete Cesium directory to your offline viewer. This ZIP does not bundle
Cesium again, and requires no Cesium ion account, token, CDN or online basemap.

const provider = await Cesium.CesiumTerrainProvider.fromUrl('/terrain/');
const viewer = new Cesium.Viewer('viewer', {
  terrainProvider: provider, baseLayer: false, baseLayerPicker: false,
  geocoder: false, animation: false, timeline: false
});
viewer.scene.globe.baseColor = Cesium.Color.fromCssColorString('#738c76');

The Mercator source stops at latitude ±85.0511287798066 degrees.
Source heights are preserved without vertical-datum conversion.
`
