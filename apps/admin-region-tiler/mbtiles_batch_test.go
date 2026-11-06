package main

import (
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/paulmach/orb/maptile"
)

func TestMBTilesBatchRollbackAndTMS(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "batch.mbtiles"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	_, err = db.Exec(`CREATE TABLE tiles(zoom_level INTEGER,tile_column INTEGER CHECK(tile_column<2),tile_row INTEGER,tile_data BLOB,PRIMARY KEY(zoom_level,tile_column,tile_row))`)
	if err != nil {
		t.Fatal(err)
	}
	batch := []Tile{{T: maptile.New(0, 0, 2), C: []byte("first")}, {T: maptile.New(2, 0, 2), C: []byte("invalid")}}
	if err := writeMBTileBatch(db, batch); err == nil {
		t.Fatal("expected failed transaction")
	}
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM tiles`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("partial commit: %d, %v", count, err)
	}
	batch[1].T = maptile.New(1, 0, 2)
	if err := writeMBTileBatch(db, batch); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(`SELECT count(*) FROM tiles WHERE tile_row=3`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("TMS mismatch: %d, %v", count, err)
	}
}

func BenchmarkMBTilesWrites(b *testing.B) {
	for _, batched := range []bool{false, true} {
		name := "autocommit"
		if batched {
			name = "batch256"
		}
		b.Run(name, func(b *testing.B) {
			db, err := sql.Open("sqlite", filepath.Join(b.TempDir(), "benchmark.mbtiles"))
			if err != nil {
				b.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec(`CREATE TABLE tiles(zoom_level INTEGER,tile_column INTEGER,tile_row INTEGER,tile_data BLOB,PRIMARY KEY(zoom_level,tile_column,tile_row))`); err != nil {
				b.Fatal(err)
			}
			tiles := make([]Tile, 256)
			for i := range tiles {
				tiles[i] = Tile{T: maptile.New(uint32(i), 0, 8), C: make([]byte, 1024)}
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if batched {
					err = writeMBTileBatch(db, tiles)
				} else {
					for _, tile := range tiles {
						if err = saveToMBTile(tile, db); err != nil {
							break
						}
					}
				}
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
