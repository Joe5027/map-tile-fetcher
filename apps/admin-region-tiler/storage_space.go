package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Check only the additional bytes required by this phase. Existing outputs are
// already reflected in free space; there is no unconditional 1 GiB surcharge.
func requireOutputSpace(directory, phase string, additional uint64) error {
	free, err := availableDiskBytes(directory)
	if err != nil {
		return err
	}
	return checkOutputSpace(free, phase, additional)
}

func checkOutputSpace(free uint64, phase string, additional uint64) error {
	if reserve := viper.GetInt64("task.min_free_disk_bytes"); reserve > 0 {
		additional += uint64(reserve)
	}
	if free < additional {
		return fmt.Errorf("insufficient disk space for %s: need %d additional bytes, available %d", phase, additional, free)
	}
	return nil
}

type baselineSpace struct {
	DataBytes    uint64
	JournalBytes uint64 // SQLite journal, or overlap while atomically replacing a raw file.
}

// A fresh MBTiles copy needs the populated SQLite pages, including indexes,
// rather than only BLOB payload. Its DELETE journal is bounded to one batch:
// table/index search paths plus replaced overflow pages, capped by used pages.
// Freelist pages are not copied and must not make an old database fail admission.
func baselineCopySpace(ctx context.Context, reader *tileOutputReader, task *Task) (baselineSpace, error) {
	var space baselineSpace
	if reader.db != nil {
		var pageSize, pages, freePages, depth uint64
		for _, q := range []struct {
			sql   string
			value *uint64
		}{
			{"PRAGMA page_size", &pageSize}, {"PRAGMA page_count", &pages}, {"PRAGMA freelist_count", &freePages},
		} {
			if err := reader.db.QueryRowContext(ctx, q.sql).Scan(q.value); err != nil {
				return space, err
			}
		}
		depth = 1
		for _, name := range []string{"tiles", "tile_index"} {
			var leafDepth uint64
			err := reader.db.QueryRowContext(ctx, `SELECT length(path)-length(replace(path,'/','')) FROM dbstat WHERE name=? AND pagetype='leaf' LIMIT 1`, name).Scan(&leafDepth)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return space, err
			}
			depth = max(depth, leafDepth)
		}
		used := pages - freePages
		space.DataBytes = used * pageSize
		batchRows := min(uint64(max(task.Total, 0)), uint64(mbtilesBatchSize))
		// New BLOB/overflow pages do not enter a rollback journal. Only an
		// interrupted retry copy can replace BLOBs committed by its old worker.
		journalPages := batchRows*2*depth + 2
		if task.preserveOutput {
			if existing, err := os.Stat(task.File); err == nil && existing.Size() > 0 {
				existingBytes := uint64(existing.Size())
				space.DataBytes -= min(space.DataBytes, existingBytes)
				var maxBlob uint64
				if err := reader.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(length(tile_data)),0) FROM tiles`).Scan(&maxBlob); err != nil {
					return space, err
				}
				blobBytes := min(batchRows*maxBlob, max(uint64(mbtilesBatchBytes), maxBlob))
				// Overflow pages contain a next-page pointer. Account for payload
				// capacity and per-row rounding, without reserving 256 large BLOBs.
				journalPages += (blobBytes+pageSize-5)/(pageSize-4) + batchRows
				used = (existingBytes + pageSize - 1) / pageSize
			} else if err != nil && !os.IsNotExist(err) {
				return space, err
			}
		}
		journalPages = min(used, journalPages)
		space.JournalBytes = max(pageSize, uint64(4096)) + journalPages*(pageSize+8)
		return space, nil
	}
	// File copies always replace an existing coordinate atomically. Existing
	// bytes can be credited without reading/decoding the target: the validated
	// baseline is still copied over it. Reserve net growth plus the largest
	// old/new overlap while the temporary replacement exists.
	addFile := func(name string, size, previous uint64) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		var existing uint64
		if task.preserveOutput {
			info, err := os.Stat(filepath.Join(task.File, filepath.FromSlash(name)))
			if err == nil && info.Mode().IsRegular() {
				existing = uint64(info.Size())
			} else if err != nil && !os.IsNotExist(err) {
				return err
			}
		}
		growth := size - min(size, existing)
		previousGrowth := previous - min(previous, existing)
		space.DataBytes += growth - previousGrowth
		space.JournalBytes = max(space.JournalBytes, min(size, existing))
		return nil
	}
	if reader.archive != nil {
		for _, entry := range reader.archive.File {
			if strings.HasSuffix(entry.Name, "."+reader.format) {
				if err := addFile(entry.Name, entry.UncompressedSize64, 0); err != nil {
					return space, err
				}
			}
		}
	}
	if reader.directory != "" {
		err := filepath.Walk(reader.directory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("baseline contains a symlink: %s", path)
			}
			if info.Mode().IsRegular() && strings.HasSuffix(info.Name(), "."+reader.format) {
				relative, err := filepath.Rel(reader.directory, path)
				if err != nil {
					return err
				}
				name := filepath.ToSlash(relative)
				var previous uint64
				if entry := reader.entries[name]; entry != nil {
					previous = entry.UncompressedSize64
				}
				if size := uint64(info.Size()); size > previous {
					return addFile(name, size, previous)
				}
			}
			return nil
		})
		if err != nil {
			return space, err
		}
	}
	return space, nil
}
