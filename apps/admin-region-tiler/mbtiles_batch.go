package main

import (
	"database/sql"
	"time"
)

const mbtilesBatchSize = 256
const mbtilesCommitInterval = 250 * time.Millisecond

// A batch is one durability boundary: callers must only count success after Commit.
func writeMBTileBatch(db *sql.DB, tiles []Tile) error {
	if len(tiles) == 0 {
		return nil
	}
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.Prepare(`INSERT OR REPLACE INTO tiles(zoom_level,tile_column,tile_row,tile_data) VALUES(?,?,?,?)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for _, tile := range tiles {
		if _, err := statement.Exec(tile.T.Z, tile.T.X, tile.flipY(), tile.C); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (task *Task) runMBTilesSaver() {
	defer task.saveWG.Done()
	ticker := time.NewTicker(mbtilesCommitInterval)
	defer ticker.Stop()
	batch := make([]Tile, 0, mbtilesBatchSize)
	flush := func() bool {
		if len(batch) == 0 {
			return true
		}
		err := writeMBTileBatch(task.db, batch)
		for _, tile := range batch {
			if err != nil {
				task.recordTileFailure(TileJob{Tile: tile.T, URL: tile.URL}, err, true)
			}
			task.markProcessed(err == nil, err)
		}
		batch = batch[:0]
		if err != nil {
			task.setError(err)
			task.cancel()
			return false
		}
		return true
	}
	defer flush()
	for {
		select {
		case tile, ok := <-task.savingpipe:
			if !ok {
				return
			}
			if err := task.waitIfPaused(); err != nil {
				return
			}
			batch = append(batch, tile)
			if len(batch) >= mbtilesBatchSize && !flush() {
				return
			}
		case <-ticker.C:
			if !flush() {
				return
			}
		case <-task.ctx.Done():
			return
		}
	}
}
