package main

import (
	"context"
	"database/sql"
	"os"

	"github.com/paulmach/orb/maptile"
	"github.com/paulmach/orb/maptile/tilecover"
	"tiler/internal/area"
	"tiler/internal/downloader"
)

func walkLayer(ctx context.Context, layer Layer, visit func(maptile.Tile) error) error {
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
	// The library owns and closes its channel. Drain it on cancellation so its
	// producer cannot remain blocked after a consumer returns.
	tiles := make(chan maptile.Tile, 32)
	go tilecover.CollectionChannel(layer.Collection, maptile.Zoom(layer.Zoom), tiles)
	var result error
	for tile := range tiles {
		if result != nil {
			continue
		}
		if err := ctx.Err(); err != nil {
			result = err
			continue
		}
		result = visit(tile)
	}
	return result
}

func (task *Task) forEachExpected(visit func(TileJob) error) error {
	ctx := task.ctx
	if task.currentStatus() == TaskCompleted || task.currentStatus() == TaskPartialFailed {
		ctx = context.Background()
	}
	file, err := os.CreateTemp("", "tiler-enumeration-*.db")
	if err != nil {
		return err
	}
	path := file.Name()
	file.Close()
	defer os.Remove(path)
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return err
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA journal_mode=OFF; PRAGMA cache_size=-2048; CREATE TABLE jobs(z INTEGER,x INTEGER,y INTEGER,url TEXT,PRIMARY KEY(z,x,y,url)) WITHOUT ROWID`); err != nil {
		return err
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, layer := range task.Layers {
		if err := walkLayer(ctx, layer, func(tile maptile.Tile) error {
			_, err := tx.Exec(`INSERT OR IGNORE INTO jobs VALUES(?,?,?,?)`, tile.Z, tile.X, tile.Y, layer.URL)
			return err
		}); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rows, err := db.Query(`SELECT z,x,y,url FROM jobs ORDER BY z,x,y,url`)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
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
