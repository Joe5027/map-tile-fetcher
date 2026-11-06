package main

import (
	"database/sql"
	"time"
)

const mbtilesBatchSize = 256
const mbtilesBatchBytes = 8 << 20
const mbtilesCommitInterval = 250 * time.Millisecond

// Bound retained payload independently of tile count. A tile larger than the
// byte budget is still valid and is written in its own transaction.
type mbtilesBatch struct {
	tiles []Tile
	bytes int
}

// accepted is false only when flushing the previous batch fails before this
// tile enters it, so the saver can record that consumed tile's failure too.
func (batch *mbtilesBatch) add(tile Tile, write func([]Tile) error) (accepted bool, err error) {
	if len(batch.tiles) > 0 && batch.bytes+len(tile.C) > mbtilesBatchBytes {
		if err := batch.flush(write); err != nil {
			return false, err
		}
	}
	batch.tiles = append(batch.tiles, tile)
	batch.bytes += len(tile.C)
	if len(batch.tiles) >= mbtilesBatchSize || batch.bytes >= mbtilesBatchBytes {
		return true, batch.flush(write)
	}
	return true, nil
}

func (batch *mbtilesBatch) flush(write func([]Tile) error) error {
	if len(batch.tiles) == 0 {
		return nil
	}
	err := write(batch.tiles)
	batch.reset()
	return err
}

func (batch *mbtilesBatch) reset() {
	clear(batch.tiles)
	batch.tiles = batch.tiles[:0]
	batch.bytes = 0
}

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
	ticker := time.NewTicker(mbtilesCommitInterval)
	defer ticker.Stop()
	task.runMBTilesSaverTicks(ticker.C)
}

func (task *Task) runMBTilesSaverTicks(ticks <-chan time.Time) {
	defer task.saveWG.Done()
	batch := mbtilesBatch{tiles: make([]Tile, 0, mbtilesBatchSize)}
	write := func(tiles []Tile) error {
		err := writeMBTileBatch(task.db, tiles)
		for _, tile := range tiles {
			if err != nil {
				task.recordTileFailure(TileJob{Tile: tile.T, URL: tile.URL}, err, true)
			}
			task.markProcessed(err == nil, err)
		}
		if err != nil {
			task.setError(err)
			task.cancel()
		}
		return err
	}
	defer batch.flush(write)
	for {
		select {
		case tile, ok := <-task.savingpipe:
			if !ok {
				return
			}
			if err := task.waitIfPaused(); err != nil {
				return
			}
			if accepted, err := batch.add(tile, write); err != nil {
				if !accepted {
					task.recordTileFailure(TileJob{Tile: tile.T, URL: tile.URL}, err, true)
					task.markProcessed(false, err)
				}
				return
			}
		case <-ticks:
			if err := batch.flush(write); err != nil {
				return
			}
		case <-task.ctx.Done():
			return
		}
	}
}
