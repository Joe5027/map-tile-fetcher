package main

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

func TestRetryRawCopySpaceCreditsExistingFilesAndAtomicReplacement(t *testing.T) {
	for _, mode := range []string{"raw", "zip", "raw+zip"} {
		t.Run(mode, func(t *testing.T) {
			useFileTestStore(t)
			setTestOutput(t)
			plan := storedBBoxTask(t, "raw-space", "")
			tiles := []maptile.Tile{maptile.New(0, 0, 2), maptile.New(1, 0, 2), maptile.New(2, 0, 2), maptile.New(3, 0, 2)}
			makeTask := func(name string) *Task {
				task := NewTask([]Layer{{Zoom: 2, Tiles: tiles}}, TileMap{Format: PNG, Schema: "xyz", URL: plan.URL}, TaskOptions{OutputFormat: "zip"})
				t.Cleanup(task.cancel)
				task.File = filepath.Join(viper.GetString("output.directory"), name)
				return task
			}
			source, target := makeTask("raw-source"), makeTask("raw-target")
			body := testPNG(t)
			for i, tile := range tiles {
				if err := saveToFiles(Tile{T: tile, C: body}, source); err != nil {
					t.Fatal(err)
				}
				if i < 3 {
					if err := saveToFiles(Tile{T: tile, C: body}, target); err != nil {
						t.Fatal(err)
					}
				}
			}
			run := &TaskRunRecord{ID: "raw-baseline", TaskRecordID: plan.ID, UserID: plan.UserID, Status: TaskCompleted, OutputPath: source.File, ArtifactPath: source.File, ArtifactStatus: ArtifactReady, SuccessCount: 4}
			if mode != "raw" {
				run.ArtifactPath = filepath.Join(viper.GetString("output.directory"), "raw-baseline.zip")
				if err := zipDirectory(source.File, run.ArtifactPath, nil); err != nil {
					t.Fatal(err)
				}
			}
			if mode == "zip" {
				run.OutputPath = ""
			}
			if err := store.createRun(run); err != nil {
				t.Fatal(err)
			}
			target.preserveOutput = true // The same setting used by a resumed retry worker.
			reader, err := openTileOutput(plan, run)
			if err != nil {
				t.Fatal(err)
			}
			space, err := baselineCopySpace(context.Background(), reader, target)
			reader.Close()
			if err != nil {
				t.Fatal(err)
			}
			if space.DataBytes != uint64(len(body)) || space.JournalBytes != uint64(len(body)) {
				t.Fatalf("resume must reserve one missing file plus one atomic replacement, not the existing three: space=%+v tile bytes=%d", space, len(body))
			}
			if err := store.prepareRetryBaseline(plan, target); err != nil {
				t.Fatal(err)
			}
			reader = &tileOutputReader{directory: target.File, format: PNG, schema: "xyz"}
			for _, tile := range tiles {
				if _, err := reader.readValidated(tile); err != nil {
					t.Fatalf("resumed copy missing %v: %v", tile, err)
				}
			}
		})
	}
}
