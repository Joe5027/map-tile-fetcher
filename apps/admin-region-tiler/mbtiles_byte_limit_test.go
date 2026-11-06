package main

import (
	"bytes"
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

// With no timer ticks or channel close, only the payload bound can commit.
func TestMBTilesSaverCommitsLargePayloadBeforeCountLimit(t *testing.T) {
	task := NewTask([]Layer{{Zoom: 4, Tiles: []maptile.Tile{maptile.New(0, 0, 4)}}}, TileMap{Format: PBF}, TaskOptions{OutputFormat: "mbtiles"})
	task.File = filepath.Join(t.TempDir(), "saver.mbtiles")
	if err := task.setupOutput(); err != nil {
		t.Fatal(err)
	}
	task.saveWG.Add(1)
	go task.runMBTilesSaverTicks(nil)
	t.Cleanup(func() { task.cancel(); task.saveWG.Wait(); task.closeOutput() })
	for i := 0; i < 3; i++ {
		task.savingpipe <- Tile{T: maptile.New(uint32(i), 0, 4), C: bytes.Repeat([]byte{byte(i)}, 3<<20)}
	}
	waitCommitted := func(want int64) {
		t.Helper()
		until := time.Now().Add(3 * time.Second)
		for task.currentCounts().Success < want && time.Now().Before(until) {
			time.Sleep(time.Millisecond)
		}
		var rows int64
		if err := task.db.QueryRow(`SELECT count(*) FROM tiles WHERE tile_row=15`).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if counts := task.currentCounts(); counts.Success != want || counts.Failure != 0 || rows != want {
			t.Fatalf("payload boundary did not commit independently of timer/count: counts=%+v durable TMS rows=%d want=%d", counts, rows, want)
		}
	}
	waitCommitted(2) // 6 MiB commits before the next 3 MiB is appended.
	task.savingpipe <- Tile{T: maptile.New(3, 0, 4), C: bytes.Repeat([]byte{3}, 9<<20)}
	waitCommitted(4) // The preceding 3 MiB and oversized 9 MiB commit separately.
	var size int
	if err := task.db.QueryRow(`SELECT length(tile_data) FROM tiles WHERE tile_column=3`).Scan(&size); err != nil || size != 9<<20 {
		t.Fatalf("large valid tile lost or truncated: size=%d err=%v", size, err)
	}
}

func TestRetryBaselineByteBatchesPreserveCommittedTilesOnLaterRollback(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	plan := storedBBoxTask(t, "byte-copy", "")
	plan.Format = PBF
	run, tiles := baselineFixture(t, plan, 6, 0, bytes.Repeat([]byte{0x34}, 3<<20))
	task := NewTask([]Layer{{Zoom: 12, Tiles: tiles}}, TileMap{Format: PBF, Schema: "xyz", URL: plan.URL}, TaskOptions{OutputFormat: "mbtiles"})
	defer task.cancel()
	task.File = filepath.Join(viper.GetString("output.directory"), "byte-copy.mbtiles")
	if err := task.setupOutput(); err != nil {
		t.Fatal(err)
	}
	if _, err := task.db.Exec(`CREATE TRIGGER fail_later_batch BEFORE INSERT ON tiles WHEN NEW.tile_column>=3 BEGIN SELECT RAISE(ABORT, 'fixture write failure'); END`); err != nil {
		t.Fatal(err)
	}
	if err := task.closeOutput(); err != nil {
		t.Fatal(err)
	}
	task.preserveOutput = true
	if err := store.prepareRetryBaseline(plan, task); err == nil {
		t.Fatal("expected later batch write failure")
	}
	db, err := sql.Open("sqlite", task.File)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var count int
	if err := db.QueryRow(`SELECT count(*) FROM tiles WHERE tile_row=4095`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("first byte-limited transaction was not retained, or failed transaction partially committed: rows=%d err=%v", count, err)
	}
	if counts := task.currentCounts(); counts.Success != 0 || counts.Failure != 0 {
		t.Fatalf("baseline reuse inflated download counters: %+v", counts)
	}
	published, err := store.publishedRun(plan.ID)
	if err != nil || published.ID != run.ID {
		t.Fatalf("failed retry replaced old artifact: %+v %v", published, err)
	}
}

func TestMBTilesByteBatchClearsReferencesOnCommitAndRollback(t *testing.T) {
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "references.mbtiles"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE tiles(zoom_level INTEGER,tile_column INTEGER CHECK(tile_column<3),tile_row INTEGER,tile_data BLOB)`); err != nil {
		t.Fatal(err)
	}
	for _, fail := range []bool{false, true} {
		batch := mbtilesBatch{tiles: make([]Tile, 0, mbtilesBatchSize)}
		write := func(tiles []Tile) error { return writeMBTileBatch(db, tiles) }
		for i := 0; i < 2; i++ {
			x := uint32(i)
			if fail && i == 1 {
				x = 3
			}
			if _, err := batch.add(Tile{T: maptile.New(x, 0, 4), C: bytes.Repeat([]byte{1}, 3<<20), URL: "fixture"}, write); err != nil {
				t.Fatal(err)
			}
		}
		if err := batch.flush(write); (err != nil) != fail {
			t.Fatalf("flush error=%v wantFailure=%v", err, fail)
		}
		if len(batch.tiles) != 0 || batch.bytes != 0 {
			t.Fatalf("consumed batch retained counters: len=%d bytes=%d", len(batch.tiles), batch.bytes)
		}
		for _, tile := range batch.tiles[:cap(batch.tiles)] {
			if tile.C != nil || tile.URL != "" {
				t.Fatal("consumed batch retains tile payload or URL")
			}
		}
		var rows int
		if err := db.QueryRow(`SELECT count(*) FROM tiles`).Scan(&rows); err != nil || rows != 2 {
			t.Fatalf("commit/rollback changed durable rows: %d %v", rows, err)
		}
	}
}

func TestMBTilesPreflushFailureRecordsConsumedTile(t *testing.T) {
	task := NewTask([]Layer{{Zoom: 4, Tiles: []maptile.Tile{maptile.New(0, 0, 4)}}}, TileMap{Format: PBF}, TaskOptions{OutputFormat: "mbtiles"})
	task.File = filepath.Join(t.TempDir(), "failed-saver.mbtiles")
	if err := task.setupOutput(); err != nil {
		t.Fatal(err)
	}
	if _, err := task.db.Exec(`CREATE TRIGGER fail_batch BEFORE INSERT ON tiles WHEN NEW.tile_column=1 BEGIN SELECT RAISE(ABORT, 'fixture write failure'); END`); err != nil {
		t.Fatal(err)
	}
	task.saveWG.Add(1)
	go task.runMBTilesSaverTicks(nil)
	t.Cleanup(func() { task.cancel(); task.saveWG.Wait(); task.closeOutput() })
	for i := 0; i < 3; i++ {
		task.savingpipe <- Tile{T: maptile.New(uint32(i), 0, 4), C: bytes.Repeat([]byte{byte(i)}, 3<<20)}
	}
	done := make(chan struct{})
	go func() { task.saveWG.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("failed preflush did not stop the saver")
	}
	counts := task.currentCounts()
	if counts.Success != 0 || counts.Failure != 3 || len(task.FailureRecords()) != 3 {
		t.Fatalf("consumed tile lost its retryable failure: counts=%+v failures=%+v", counts, task.FailureRecords())
	}
	var rows int
	if err := task.db.QueryRow(`SELECT count(*) FROM tiles`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("failed transaction partially committed: rows=%d err=%v", rows, err)
	}
}

func TestMBTilesLargePayloadResumeJournalUsesByteLimit(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	plan := storedBBoxTask(t, "byte-space", "")
	plan.Format = PBF
	run, tiles := baselineFixture(t, plan, 16, 0, bytes.Repeat([]byte{0x34}, 1<<20))
	task := NewTask([]Layer{{Zoom: 12, Tiles: tiles}}, TileMap{Format: PBF, Schema: "xyz", URL: plan.URL}, TaskOptions{OutputFormat: "mbtiles"})
	defer task.cancel()
	task.File = filepath.Join(viper.GetString("output.directory"), "byte-space.mbtiles")
	if err := store.prepareRetryBaseline(plan, task); err != nil {
		t.Fatal(err)
	}
	base, err := os.Stat(task.File)
	if err != nil {
		t.Fatal(err)
	}
	reader, err := openTileOutput(plan, run)
	if err != nil {
		t.Fatal(err)
	}
	space, err := baselineCopySpace(context.Background(), reader, task)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	if space.JournalBytes >= 16<<20 {
		t.Fatalf("resume reserves all 16 MiB payload instead of a bounded byte batch: %+v", space)
	}
	var peak, journal uint64
	done := make(chan error, 1)
	go func() { done <- store.prepareRetryBaseline(plan, task) }()
	ticker := time.NewTicker(100 * time.Microsecond)
	defer ticker.Stop()
	for {
		var total uint64
		for i, path := range []string{task.File, task.File + "-journal"} {
			if info, err := os.Stat(path); err == nil {
				total += uint64(info.Size())
				if i == 1 {
					journal = max(journal, uint64(info.Size()))
				}
			}
		}
		peak = max(peak, total)
		select {
		case err := <-done:
			if err != nil {
				t.Fatal(err)
			}
			additional := peak - uint64(base.Size())
			if journal == 0 || additional > space.DataBytes+space.JournalBytes {
				t.Fatalf("observed resume additional=%d journal=%d allowance=%+v", additional, journal, space)
			}
			t.Logf("large-payload resume additional=%d measured journal=%d allowance=%+v", additional, journal, space)
			return
		case <-ticker.C:
		}
	}
}
