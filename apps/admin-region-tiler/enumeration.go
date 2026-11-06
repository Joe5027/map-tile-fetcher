package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	"tiler/internal/area"
	"tiler/internal/downloader"
)

func walkLayer(ctx context.Context, layer Layer, visit func(maptile.Tile) error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if layer.BBox != nil {
		b := layer.BBox
		return downloader.WalkBBoxTiles(area.BBox{MinLon: b.MinLon, MinLat: b.MinLat, MaxLon: b.MaxLon, MaxLat: b.MaxLat}, area.ZoomRange{Min: layer.Zoom, Max: layer.Zoom}, func(id downloader.TileID) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			return visit(maptile.New(uint32(id.X), uint32(id.Y), maptile.Zoom(id.Z)))
		})
	}
	if len(layer.Tiles) > 0 {
		for _, tile := range layer.Tiles {
			if err := ctx.Err(); err != nil {
				return err
			}
			if err := visit(tile); err != nil {
				return err
			}
		}
		return nil
	}
	// The library owns and closes its channel. On cancellation drain it in the
	// background so an isolated worker can stop promptly without a blocked send.
	tiles := make(chan maptile.Tile, 32)
	go tilecover.CollectionChannel(layer.Collection, maptile.Zoom(layer.Zoom), tiles)
	drain := func() {
		go func() {
			for range tiles {
			}
		}()
	}
	for {
		if err := ctx.Err(); err != nil {
			drain()
			return err
		}
		select {
		case <-ctx.Done():
			drain()
			return ctx.Err()
		case tile, ok := <-tiles:
			if !ok {
				return nil
			}
			if err := visit(tile); err != nil {
				drain()
				return err
			}
		}
	}
}

func (task *Task) forEachExpected(visit func(TileJob) error) error {
	ctx := task.ctx
	if !task.workerManaged && (task.currentStatus() == TaskCompleted || task.currentStatus() == TaskPartialFailed) {
		ctx = context.Background()
	}
	if task.canStreamExpected() {
		for _, layer := range task.Layers {
			if err := walkLayer(ctx, layer, func(tile maptile.Tile) error {
				if task.workerManaged {
					if err := task.waitIfPaused(); err != nil {
						return err
					}
				}
				return visit(TileJob{Tile: tile, URL: layer.URL})
			}); err != nil {
				return err
			}
		}
		return nil
	}
	db, cleanup, err := task.openExpectedCatalogue(ctx)
	if err != nil {
		return err
	}
	defer cleanup()
	rows, err := db.Query(`SELECT z,x,y,url FROM jobs ORDER BY z,x,y,url`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if task.workerManaged {
			if err := task.waitIfPaused(); err != nil {
				return err
			}
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		var z, x, y int
		var url string
		if err := rows.Scan(&z, &x, &y, &url); err != nil {
			return err
		}
		if err := visit(TileJob{Tile: maptile.New(uint32(x), uint32(y), maptile.Zoom(z)), URL: url}); err != nil {
			return err
		}
	}
	return rows.Err()
}

// A single bbox per zoom/source already emits unique coordinates. Avoid a
// temporary database on this common path, while retaining deduplication for
// overlapping regions and providers that expand jobs (for example DEM halos).
func (task *Task) canStreamExpected() bool {
	if task.expandExpected != nil {
		return false
	}
	seen := make(map[string]bool)
	for _, layer := range task.Layers {
		if layer.BBox == nil || len(layer.Tiles) != 0 {
			return false
		}
		key := fmt.Sprintf("%d:%s", layer.Zoom, layer.URL)
		if seen[key] {
			return false
		}
		seen[key] = true
	}
	return true
}

func (task *Task) expectedCount() (int64, error) {
	if task.canStreamExpected() {
		var total int64
		for _, layer := range task.Layers {
			total += layer.Count
		}
		if task.expectedLimit > 0 && total > task.expectedLimit {
			return 0, &TileBudgetError{Estimated: total, Limit: task.expectedLimit, Actual: true}
		}
		return total, nil
	}
	db, cleanup, err := task.openExpectedCatalogue(task.ctx)
	if err != nil {
		return 0, err
	}
	defer cleanup()
	var count int64
	err = db.QueryRow(`SELECT COUNT(*) FROM jobs`).Scan(&count)
	if err == nil && task.expectedLimit > 0 && count > task.expectedLimit {
		return 0, &TileBudgetError{Estimated: count, Limit: task.expectedLimit, Actual: true}
	}
	return count, err
}

func (task *Task) openExpectedCatalogue(ctx context.Context) (*sql.DB, func(), error) {
	if task.expectedPath != "" {
		if _, err := os.Stat(task.expectedPath); err == nil {
			db, err := sql.Open("sqlite", filepath.ToSlash(task.expectedPath)+"?mode=ro")
			if err != nil {
				return nil, nil, err
			}
			var ready int
			if err := db.QueryRow(`SELECT complete FROM manifest`).Scan(&ready); err == nil && ready == 1 {
				return db, func() { db.Close() }, nil
			}
			db.Close()
			if err := os.Remove(task.expectedPath); err != nil {
				return nil, nil, err
			}
		}
	}
	directory := ""
	if task.expectedPath != "" {
		directory = filepath.Dir(task.expectedPath)
		if err := os.MkdirAll(directory, directoryPermissions); err != nil {
			return nil, nil, err
		}
	}
	file, err := os.CreateTemp(directory, "tiler-enumeration-*.db")
	if err != nil {
		return nil, nil, err
	}
	path := file.Name()
	file.Close()
	remove := true
	defer func() {
		if remove {
			os.Remove(path)
		}
	}()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, nil, err
	}
	closed := false
	defer func() {
		if !closed {
			db.Close()
		}
	}()
	if _, err := db.Exec(`PRAGMA journal_mode=OFF; PRAGMA cache_size=-2048; CREATE TABLE jobs(z INTEGER,x INTEGER,y INTEGER,url TEXT,PRIMARY KEY(z,x,y,url)) WITHOUT ROWID; CREATE TABLE manifest(complete INTEGER NOT NULL)`); err != nil {
		return nil, nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		return nil, nil, err
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT OR IGNORE INTO jobs VALUES(?,?,?,?)`)
	if err != nil {
		return nil, nil, err
	}
	defer insert.Close()
	var count int64
	emit := func(job TileJob) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		result, err := insert.Exec(job.Tile.Z, job.Tile.X, job.Tile.Y, job.URL)
		if err != nil {
			return err
		}
		added, err := result.RowsAffected()
		if err != nil {
			return err
		}
		count += added
		if task.expectedLimit > 0 && count > task.expectedLimit {
			return &TileBudgetError{Estimated: count, Limit: task.expectedLimit, Actual: true}
		}
		return nil
	}
	for _, layer := range task.Layers {
		if err := walkLayer(ctx, layer, func(tile maptile.Tile) error {
			if task.workerManaged {
				if err := task.waitIfPaused(); err != nil {
					return err
				}
			}
			job := TileJob{Tile: tile, URL: layer.URL}
			if task.expandExpected != nil {
				return task.expandExpected(job, emit)
			}
			return emit(job)
		}); err != nil {
			return nil, nil, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO manifest VALUES(1)`); err != nil {
		return nil, nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	if err := db.Close(); err != nil {
		return nil, nil, err
	}
	closed = true
	if task.expectedPath != "" {
		if err := os.Rename(path, task.expectedPath); err != nil {
			return nil, nil, err
		}
		path = task.expectedPath
		remove = false
	}
	db, err = sql.Open("sqlite", filepath.ToSlash(path)+"?mode=ro")
	if err != nil {
		return nil, nil, err
	}
	remove = false
	return db, func() {
		db.Close()
		if task.expectedPath == "" {
			os.Remove(path)
		}
	}, nil
}
