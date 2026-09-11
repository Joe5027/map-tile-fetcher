package main

import (
	"archive/zip"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/paulmach/orb/maptile"
	"github.com/spf13/viper"
)

var errMissingBaseline = errors.New("previous successful tiles are missing or damaged; recreate a complete task")

type IntegrityState struct {
	Status    string `json:"status"`
	Expected  int64  `json:"expected"`
	Available int64  `json:"available"`
	Missing   int64  `json:"missing"`
	Error     string `json:"error,omitempty"`
	RunID     string `json:"runId,omitempty"`
}

func (s *SQLiteStore) initIntegritySchema() error {
	columns, err := s.tableColumnSet("failures")
	if err != nil {
		return err
	}
	for name, definition := range map[string]string{"resolved_at": "INTEGER NOT NULL DEFAULT 0", "resolved_run_id": "TEXT NOT NULL DEFAULT ''"} {
		if !columns[name] {
			if _, err := s.db.Exec("ALTER TABLE failures ADD COLUMN " + name + " " + definition); err != nil {
				return err
			}
		}
	}
	for _, statement := range []string{
		`CREATE TABLE IF NOT EXISTS execution_queue(plan_id TEXT PRIMARY KEY,trigger_mode TEXT NOT NULL,due_at INTEGER NOT NULL,enqueued_at INTEGER NOT NULL,state TEXT NOT NULL)`,
		`CREATE INDEX IF NOT EXISTS idx_execution_queue_order ON execution_queue(state,due_at,enqueued_at,plan_id)`,
		`CREATE INDEX IF NOT EXISTS idx_failure_coordinates ON failures(source_id,resolved_at,z,x,y,url)`,
		`CREATE TABLE IF NOT EXISTS task_integrity(plan_id TEXT PRIMARY KEY,status TEXT NOT NULL DEFAULT 'unchecked',expected INTEGER NOT NULL DEFAULT 0,available INTEGER NOT NULL DEFAULT 0,missing INTEGER NOT NULL DEFAULT 0,error_message TEXT NOT NULL DEFAULT '',run_id TEXT NOT NULL DEFAULT '')`,
		`CREATE TABLE IF NOT EXISTS run_coverage(run_id TEXT NOT NULL,z INTEGER NOT NULL,x INTEGER NOT NULL,y INTEGER NOT NULL,url TEXT NOT NULL,PRIMARY KEY(run_id,z,x,y,url))`,
		`CREATE TABLE IF NOT EXISTS pending_publications(run_id TEXT PRIMARY KEY,record_json TEXT NOT NULL,integrity_json TEXT NOT NULL)`,
	} {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) integrityState(planID string) IntegrityState {
	state := IntegrityState{Status: "unchecked"}
	_ = s.db.QueryRow(`SELECT status,expected,available,missing,error_message,run_id FROM task_integrity WHERE plan_id = ?`, planID).Scan(&state.Status, &state.Expected, &state.Available, &state.Missing, &state.Error, &state.RunID)
	return state
}

func saveIntegrity(execer sqlExecer, planID string, state IntegrityState) error {
	_, err := execer.Exec(`INSERT INTO task_integrity(plan_id,status,expected,available,missing,error_message,run_id) VALUES(?,?,?,?,?,?,?) ON CONFLICT(plan_id) DO UPDATE SET status=excluded.status,expected=excluded.expected,available=excluded.available,missing=excluded.missing,error_message=excluded.error_message,run_id=excluded.run_id`, planID, state.Status, state.Expected, state.Available, state.Missing, state.Error, state.RunID)
	return err
}

type tileOutputReader struct {
	directory string
	archive   *zip.ReadCloser
	entries   map[string]*zip.File
	db        *sql.DB
	format    string
	schema    string
}

func openTileOutput(plan *TaskRecord, run *TaskRunRecord) (*tileOutputReader, error) {
	reader := &tileOutputReader{format: plan.Format, schema: plan.Schema}
	for _, candidate := range []string{run.OutputPath, run.ArtifactPath} {
		path, err := ensurePathWithinRoot(candidate, viper.GetString("output.directory"), false)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		if info.IsDir() {
			reader.directory = path
			continue
		}
		if strings.HasSuffix(strings.ToLower(path), ".mbtiles") {
			db, err := sql.Open("sqlite", filepath.ToSlash(path)+"?mode=ro")
			if err != nil {
				continue
			}
			var check string
			if err := db.QueryRow("PRAGMA quick_check").Scan(&check); err != nil || check != "ok" {
				db.Close()
				continue
			}
			reader.db = db
			return reader, nil
		}
		if strings.HasSuffix(strings.ToLower(path), ".zip") {
			r, err := zip.OpenReader(path)
			if err != nil {
				continue
			}
			reader.archive = r
			reader.entries = make(map[string]*zip.File)
			for _, file := range r.File {
				reader.entries[file.Name] = file
			}
			return reader, nil
		}
	}
	if reader.directory != "" {
		return reader, nil
	}
	return nil, errMissingBaseline
}

func (r *tileOutputReader) Close() {
	if r.archive != nil {
		r.archive.Close()
	}
	if r.db != nil {
		r.db.Close()
	}
}

func (r *tileOutputReader) read(tile maptile.Tile) ([]byte, error) {
	y := tile.Y
	if r.db != nil || strings.EqualFold(r.schema, "tms") {
		y = uint32(1<<tile.Z) - 1 - y
	}
	if r.db != nil {
		var data []byte
		err := r.db.QueryRow(`SELECT tile_data FROM tiles WHERE zoom_level=? AND tile_column=? AND tile_row=?`, tile.Z, tile.X, y).Scan(&data)
		return data, err
	}
	name := fmt.Sprintf("%d/%d/%d.%s", tile.Z, tile.X, y, r.format)
	var source io.ReadCloser
	var err error
	if r.archive != nil {
		entry := r.entries[name]
		if entry == nil {
			return nil, os.ErrNotExist
		}
		source, err = entry.Open()
	} else {
		path, pathErr := ensurePathWithinRoot(filepath.Join(r.directory, filepath.FromSlash(name)), r.directory, false)
		if pathErr != nil {
			return nil, pathErr
		}
		source, err = os.Open(path)
	}
	if err != nil {
		return nil, err
	}
	defer source.Close()
	return readLimitedResponseBody(source, maxTileResponseBytes)
}

// Coverage is recorded on disk, and only publication makes matching failures
// resolved. A cancelled or failed run cannot resolve failures in an older ZIP.
func (s *SQLiteStore) inspectOutput(plan *TaskRecord, task *Task, run *TaskRunRecord, recordCoverage bool) (IntegrityState, error) {
	state := IntegrityState{Status: "incomplete", RunID: run.ID}
	reader, err := openTileOutput(plan, run)
	if err != nil {
		state.Error = err.Error()
		return state, err
	}
	defer reader.Close()
	var coverage *sql.Tx
	if recordCoverage {
		if _, err := s.db.Exec(`DELETE FROM run_coverage WHERE run_id=?`, run.ID); err != nil {
			return state, err
		}
		coverage, err = s.db.Begin()
		if err != nil {
			return state, err
		}
		defer func() {
			if coverage != nil {
				coverage.Rollback()
			}
		}()
	}
	err = task.forEachExpected(func(job TileJob) error {
		state.Expected++
		data, err := reader.read(job.Tile)
		if err == nil {
			err = validateTileResponse(data, plan.Format)
		}
		if err != nil {
			state.Missing++
			return nil
		}
		state.Available++
		if recordCoverage {
			_, err = coverage.Exec(`INSERT OR IGNORE INTO run_coverage(run_id,z,x,y,url) VALUES(?,?,?,?,?)`, run.ID, job.Tile.Z, job.Tile.X, job.Tile.Y, prepareTileURL(job.Tile, job.URL))
			if err == nil && state.Available%256 == 0 {
				if err = coverage.Commit(); err != nil {
					return err
				}
				coverage, err = s.db.Begin()
			}
		}
		return err
	})
	if err != nil {
		return state, err
	}
	if coverage != nil {
		if err := coverage.Commit(); err != nil {
			return state, err
		}
		coverage = nil
	}
	if state.Missing == 0 && state.Expected > 0 {
		state.Status = "complete"
	}
	return state, nil
}

func (s *SQLiteStore) validateRetryBaseline(plan *TaskRecord) (*TaskRunRecord, error) {
	run, err := s.publishedRun(plan.ID)
	if err != nil {
		return nil, errMissingBaseline
	}
	task, err := buildTaskFromRecord(plan)
	if err != nil {
		return nil, err
	}
	reader, err := openTileOutput(plan, run)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	err = task.forEachExpected(func(job TileJob) error {
		data, readErr := reader.read(job.Tile)
		if readErr == nil && validateTileResponse(data, plan.Format) == nil {
			return nil
		}
		var count int
		err := s.db.QueryRow(`SELECT COUNT(*) FROM failures WHERE source_id=? AND z=? AND x=? AND y=? AND url=? AND resolved_at=0`, plan.ID, job.Tile.Z, job.Tile.X, job.Tile.Y, prepareTileURL(job.Tile, job.URL)).Scan(&count)
		if err != nil {
			return err
		}
		if count == 0 {
			return errMissingBaseline
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return run, nil
}

func copyRetryBaseline(plan *TaskRecord, task *Task, previous *TaskRunRecord) error {
	reader, err := openTileOutput(plan, previous)
	if err != nil {
		return err
	}
	defer reader.Close()
	var size uint64
	if reader.archive != nil {
		for _, entry := range reader.archive.File {
			size += entry.UncompressedSize64
		}
	} else if reader.directory != "" {
		err = filepath.Walk(reader.directory, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return errors.New("baseline contains a symlink")
			}
			if info.Mode().IsRegular() {
				size += uint64(info.Size())
			}
			return nil
		})
		if err != nil {
			return err
		}
	} else {
		var bytes int64
		if err := reader.db.QueryRow(`SELECT COALESCE(SUM(length(tile_data)),0) FROM tiles`).Scan(&bytes); err != nil {
			return err
		}
		size = uint64(bytes)
	}
	free, err := availableDiskBytes(filepath.Dir(task.File))
	if err != nil {
		return err
	}
	if free < size+(1<<30) {
		return errors.New("insufficient disk space for baseline copy plus 1 GiB reserve")
	}
	if err := task.setupOutput(); err != nil {
		return err
	}
	err = task.forEachExpected(func(job TileJob) error {
		if err := task.ctx.Err(); err != nil {
			return err
		}
		data, err := reader.read(job.Tile)
		if err != nil {
			return nil
		}
		if err := validateTileResponse(data, plan.Format); err != nil {
			return nil
		}
		return task.saveTile(Tile{T: job.Tile, C: data, URL: job.URL})
	})
	closeErr := task.closeOutput()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	task.preserveOutput = true
	return nil
}

func (s *SQLiteStore) publishRun(run *TaskRunRecord, state IntegrityState) error {
	raw, err := json.Marshal(run)
	if err != nil {
		return err
	}
	integrity, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO pending_publications(run_id,record_json,integrity_json) VALUES(?,?,?)`, run.ID, string(raw), string(integrity)); err != nil {
		return err
	}
	return s.commitPublication(run, state)
}

func (s *SQLiteStore) commitPublication(run *TaskRunRecord, state IntegrityState) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.finalizeRunWithExec(tx, run); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE failures SET resolved_at=?,resolved_run_id=? WHERE source_id=? AND resolved_at=0 AND EXISTS (SELECT 1 FROM run_coverage c WHERE c.run_id=? AND c.z=failures.z AND c.x=failures.x AND c.y=failures.y AND c.url=failures.url)`, time.Now().Unix(), run.ID, run.TaskRecordID, run.ID); err != nil {
		return err
	}
	if err := saveIntegrity(tx, run.TaskRecordID, state); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE plans SET status=? WHERE id=?`, statusToTaskRecordStatus(run.Status), run.TaskRecordID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM pending_publications WHERE run_id=?`, run.ID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *SQLiteStore) recoverPublications() error {
	rows, err := s.db.Query(`SELECT record_json,integrity_json FROM pending_publications`)
	if err != nil {
		return err
	}
	type pending struct {
		run   TaskRunRecord
		state IntegrityState
	}
	items := []pending{}
	for rows.Next() {
		var raw, state string
		if err := rows.Scan(&raw, &state); err != nil {
			rows.Close()
			return err
		}
		var item pending
		if err := json.Unmarshal([]byte(raw), &item.run); err != nil {
			rows.Close()
			return err
		}
		if err := json.Unmarshal([]byte(state), &item.state); err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for _, item := range items {
		path, err := ensurePathWithinRoot(item.run.ArtifactPath, viper.GetString("output.directory"), false)
		if err != nil {
			return err
		}
		if strings.HasSuffix(path, ".zip") {
			if err := validateArchive(path); err != nil {
				return err
			}
		}
		if _, err := os.Stat(path); err != nil {
			return err
		}
		if err := s.commitPublication(&item.run, item.state); err != nil {
			return err
		}
	}
	_, err = s.db.Exec(`UPDATE task_integrity SET status='interrupted',error_message='service restarted during reconciliation' WHERE status='checking'`)
	return err
}
