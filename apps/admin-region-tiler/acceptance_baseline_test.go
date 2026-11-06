package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func baselineFixture(t *testing.T, plan *TaskRecord, count, deleted int, body []byte) (*TaskRunRecord, []maptile.Tile) {
	t.Helper()
	tiles := make([]maptile.Tile, count)
	for i := range tiles {
		tiles[i] = maptile.New(uint32(i), 0, 12)
	}
	source := NewTask([]Layer{{Zoom: 12, Tiles: tiles}}, TileMap{Format: plan.Format, Schema: "xyz", URL: plan.URL}, TaskOptions{OutputFormat: "mbtiles"})
	defer source.cancel()
	source.File = filepath.Join(viper.GetString("output.directory"), "source.mbtiles")
	if err := source.setupOutput(); err != nil {
		t.Fatal(err)
	}
	batch := make([]Tile, count+deleted)
	for i := range batch {
		batch[i] = Tile{T: maptile.New(uint32(i), 0, 12), C: body, URL: plan.URL}
	}
	if err := writeMBTileBatch(source.db, batch); err != nil {
		t.Fatal(err)
	}
	if deleted > 0 {
		if _, err := source.db.Exec(`DELETE FROM tiles WHERE tile_column>=?`, count); err != nil {
			t.Fatal(err)
		}
	}
	if err := source.closeOutput(); err != nil {
		t.Fatal(err)
	}
	run := &TaskRunRecord{ID: "baseline", TaskRecordID: plan.ID, UserID: plan.UserID, Status: TaskCompleted, OutputPath: source.File, ArtifactPath: source.File, ArtifactStatus: ArtifactReady, SuccessCount: int64(count)}
	if err := store.createRun(run); err != nil {
		t.Fatal(err)
	}
	return run, tiles
}

func TestRetryBaselineReadsAndValidatesEachCoordinateOnce(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	plan := storedBBoxTask(t, "single-pass", "")
	run, tiles := baselineFixture(t, plan, 511, 0, testPNG(t))
	missing := maptile.New(511, 0, 12)
	tiles = append(tiles, missing)
	if err := store.replaceFailureRecords(run, []TileFailureRecord{{Z: 12, X: 511, Y: 0, URL: prepareTileURL(missing, plan.URL), Retryable: true}}); err != nil {
		t.Fatal(err)
	}
	task := NewTask([]Layer{{Zoom: 12, Tiles: tiles}}, TileMap{Format: PNG, Schema: "xyz", URL: plan.URL}, TaskOptions{OutputFormat: "mbtiles"})
	defer task.cancel()
	task.File = filepath.Join(viper.GetString("output.directory"), "copied.mbtiles")
	if err := task.setupOutput(); err != nil {
		t.Fatal(err)
	}
	defer task.closeOutput()
	reader, err := openTileOutput(plan, run)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	reads := map[maptile.Tile]int{}
	copies := map[maptile.Tile]int{}
	batch := []Tile{}
	flush := func() error { err := writeMBTileBatch(task.db, batch); batch = nil; return err }
	err = store.walkRetryBaseline(plan, task, func(tile maptile.Tile) ([]byte, error) { reads[tile]++; return reader.readValidated(tile) }, func(tile Tile) error {
		copies[tile.T]++
		batch = append(batch, tile)
		if len(batch) == mbtilesBatchSize {
			return flush()
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := flush(); err != nil {
		t.Fatal(err)
	}
	for _, tile := range tiles {
		if reads[tile] != 1 {
			t.Fatalf("coordinate %v read/validated %d times", tile, reads[tile])
		}
	}
	if len(copies) != 511 || copies[missing] != 0 {
		t.Fatalf("copies=%d missing=%d", len(copies), copies[missing])
	}
	for tile, count := range copies {
		if count != 1 {
			t.Fatalf("coordinate %v copied %d times", tile, count)
		}
	}
	var copied int
	if err := task.db.QueryRow(`SELECT COUNT(*) FROM tiles`).Scan(&copied); err != nil || copied != 511 {
		t.Fatalf("copied=%d err=%v", copied, err)
	}
	published, err := store.publishedRun(plan.ID)
	if err != nil || published.ID != run.ID {
		t.Fatalf("unpublished baseline replaced old artifact: %+v %v", published, err)
	}
}

func TestMBTilesCopySpaceIncludesPagesAndBoundedJournal(t *testing.T) {
	useFileTestStore(t)
	setTestOutput(t)
	plan := storedBBoxTask(t, "copy-space", "")
	plan.Format = PBF
	body := bytes.Repeat([]byte{0x12, 0x34, 0x56, 0x78}, 1024)
	run, tiles := baselineFixture(t, plan, 1000, 2500, body)
	task := NewTask([]Layer{{Zoom: 12, Tiles: tiles}}, TileMap{Format: PBF, Schema: "xyz", URL: plan.URL}, TaskOptions{OutputFormat: "mbtiles"})
	defer task.cancel()
	task.File = filepath.Join(viper.GetString("output.directory"), "copy.mbtiles")
	reader, err := openTileOutput(plan, run)
	if err != nil {
		t.Fatal(err)
	}
	space, err := baselineCopySpace(context.Background(), reader, task)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(run.OutputPath)
	if err != nil {
		t.Fatal(err)
	}
	if space.DataBytes <= uint64(len(body)*len(tiles)) || space.DataBytes >= uint64(info.Size()) || space.JournalBytes == 0 {
		t.Fatalf("payload/index/free-page accounting: %+v source=%d payload=%d", space, info.Size(), len(body)*len(tiles))
	}
	copyAndMeasure := func() (peak, journal uint64) {
		done := make(chan error, 1)
		go func() { done <- store.prepareRetryBaseline(plan, task) }()
		sample := func() {
			var size uint64
			for i, path := range []string{task.File, task.File + "-journal"} {
				if info, err := os.Stat(path); err == nil {
					size += uint64(info.Size())
					if i == 1 {
						journal = max(journal, uint64(info.Size()))
					}
				}
			}
			peak = max(peak, size)
		}
		ticker := time.NewTicker(100 * time.Microsecond)
		defer ticker.Stop()
		for {
			select {
			case err := <-done:
				if err != nil {
					t.Fatal(err)
				}
				sample()
				return
			case <-ticker.C:
				sample()
			}
		}
	}
	peak, journal := copyAndMeasure()
	if peak > space.DataBytes+space.JournalBytes {
		t.Fatalf("copy peak underestimated: observed=%d allowance=%+v", peak, space)
	}
	t.Logf("fresh source=%d populated pages=%d journal bound=%d measured peak=%d journal=%d", info.Size(), space.DataBytes, space.JournalBytes, peak, journal)
	// The production resume path preserves this target and runs the same copy
	// function again. Measure its actual replacement journal, without synthetic UPDATEs.
	base, err := os.Stat(task.File)
	if err != nil {
		t.Fatal(err)
	}
	reader, err = openTileOutput(plan, run)
	if err != nil {
		t.Fatal(err)
	}
	resumeSpace, err := baselineCopySpace(context.Background(), reader, task)
	reader.Close()
	if err != nil {
		t.Fatal(err)
	}
	peak, journal = copyAndMeasure()
	additional := peak - uint64(base.Size())
	if journal == 0 || additional > resumeSpace.DataBytes+resumeSpace.JournalBytes || resumeSpace.DataBytes >= space.DataBytes {
		t.Fatalf("resumed copy accounting: additional=%d journal=%d allowance=%+v", additional, journal, resumeSpace)
	}
	t.Logf("resume allocated=%d additional data=%d journal bound=%d observed additional peak=%d journal=%d", base.Size(), resumeSpace.DataBytes, resumeSpace.JournalBytes, additional, journal)
}

func TestOutputSpaceUsesActualAvailableBoundary(t *testing.T) {
	dir := t.TempDir()
	available, err := availableDiskBytes(dir)
	if err != nil {
		t.Fatal(err)
	}
	previous := viper.GetInt64("task.min_free_disk_bytes")
	viper.Set("task.min_free_disk_bytes", 0)
	t.Cleanup(func() { viper.Set("task.min_free_disk_bytes", previous) })
	if err := checkOutputSpace(available, "fixture", available); err != nil {
		t.Fatal(err)
	}
	if err := checkOutputSpace(available, "fixture", available+1); err == nil || !strings.Contains(err.Error(), "additional bytes") {
		t.Fatalf("one byte short accepted: %v", err)
	}
	viper.Set("task.min_free_disk_bytes", 64)
	if err := checkOutputSpace(available, "fixture", available-64); err != nil {
		t.Fatal(err)
	}
	if err := checkOutputSpace(available, "fixture", available-63); err == nil {
		t.Fatal("configured reserve not included")
	}
	t.Logf("checked actual available=%d at exact-fit and one-byte-short; no disk fill", available)
}
