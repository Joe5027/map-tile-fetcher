package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/paulmach/orb/maptile"
)

const triggerResumeRun = "resume"
const workerParentGuardEnv = "TILER_WORKER_PARENT_GUARD"
const workerOwnerEnv = "TILER_WORKER_OWNER"
const workerResumeEnv = "TILER_WORKER_RESUME"

// The parent's only reference to the write end dies with that process. This
// also handles a killed parent, for which normal shutdown callbacks cannot run.
type workerParentWatch struct{ gone atomic.Bool }

func watchWorkerParent(task *Task) *workerParentWatch {
	watch := &workerParentWatch{}
	if os.Getenv(workerParentGuardEnv) != "1" {
		return watch
	}
	go func() {
		_, _ = io.Copy(io.Discard, os.Stdin)
		watch.gone.Store(true)
		_ = task.Cancel()
		// The old geometry library can finish a non-cancellable calculation.
		// A worker is disposable: committed output survives, and SQLite rolls
		// back an unfinished batch before the replacement worker reads it.
		time.AfterFunc(5*time.Second, func() { os.Exit(0) })
	}()
	return watch
}

func lockRun(ctx context.Context, plan *TaskRecord, runID string) (*os.File, error) {
	dir, err := managedRunDirectory(plan, runID)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, directoryPermissions); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(filepath.Join(dir, "worker.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	for {
		if err := lockMaintenanceFile(file, true); err == nil {
			return file, nil
		}
		select {
		case <-ctx.Done():
			file.Close()
			return nil, fmt.Errorf("previous worker has not released run %s: %w", runID, ctx.Err())
		case <-time.After(50 * time.Millisecond):
		}
	}
}

func (s *SQLiteStore) workerOwnsRun(planID string) (bool, error) {
	owner := os.Getenv(workerOwnerEnv)
	if owner == "" {
		return true, nil
	}
	var current string
	err := s.db.QueryRow(`SELECT owner_token FROM execution_queue WHERE plan_id=?`, planID).Scan(&current)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	return current == owner, err
}

// Fence old workers before changing any of their durable state. A delayed old
// process must acquire the same lock and pass its ownership-token check.
func (s *SQLiteStore) lockInterruptedRuns() (func(), error) {
	rows, err := s.db.Query(`SELECT p.id,p.last_run_id FROM plans p JOIN execution_queue q ON q.plan_id=p.id WHERE q.owner_token<>'' AND p.last_run_id<>''`)
	if err != nil {
		return nil, err
	}
	type item struct{ plan, run string }
	items := []item{}
	for rows.Next() {
		var i item
		if err := rows.Scan(&i.plan, &i.run); err != nil {
			rows.Close()
			return nil, err
		}
		items = append(items, i)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	locks := []*os.File{}
	release := func() {
		for _, lock := range locks {
			lock.Close()
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, i := range items {
		plan, err := s.getTaskRecordByID(i.plan)
		if err != nil {
			release()
			return nil, err
		}
		lock, err := lockRun(ctx, plan, i.run)
		if err != nil {
			release()
			return nil, err
		}
		locks = append(locks, lock)
	}
	return release, nil
}

// Recovery reuses the original output, including committed tiles whose last
// progress update was lost. Only missing or invalid coordinates are requested.
func prepareResumeJobs(plan *TaskRecord, task *Task, run *TaskRunRecord) (func(), error) {
	task.preserveOutput = true
	reader, readErr := openTileOutput(plan, &TaskRunRecord{OutputPath: run.OutputPath})
	if reader != nil {
		defer reader.Close()
	}
	file, err := os.CreateTemp(filepath.Dir(run.OutputPath), "resume-jobs-*.sqlite")
	if err != nil {
		return nil, err
	}
	path := file.Name()
	file.Close()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		os.Remove(path)
		return nil, err
	}
	cleanup := func() { db.Close(); os.Remove(path) }
	if _, err := db.Exec(`PRAGMA journal_mode=OFF; CREATE TABLE jobs(z INTEGER,x INTEGER,y INTEGER,url TEXT)`); err != nil {
		cleanup()
		return nil, err
	}
	tx, err := db.Begin()
	if err != nil {
		cleanup()
		return nil, err
	}
	defer tx.Rollback()
	insert, err := tx.Prepare(`INSERT INTO jobs VALUES(?,?,?,?)`)
	if err != nil {
		tx.Rollback()
		cleanup()
		return nil, err
	}
	defer insert.Close()
	var expected, available int64
	err = task.forEachExpected(func(job TileJob) error {
		expected++
		if readErr == nil {
			data, err := reader.read(job.Tile)
			if err == nil && validateTileResponse(data, plan.Format) == nil {
				available++
				return nil
			}
		}
		_, err := insert.Exec(job.Tile.Z, job.Tile.X, job.Tile.Y, job.URL)
		return err
	})
	if err != nil {
		tx.Rollback()
		cleanup()
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		cleanup()
		return nil, err
	}
	task.mu.Lock()
	task.Total, task.Current, task.SuccessCount, task.FailureCount = expected, available, available, 0
	task.mu.Unlock()
	task.retrySource = func(visit func(TileJob) error) error {
		rows, err := db.Query(`SELECT z,x,y,url FROM jobs ORDER BY z,x,y,url`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			if err := task.ctx.Err(); err != nil {
				return err
			}
			var z, x, y uint32
			var url string
			if err := rows.Scan(&z, &x, &y, &url); err != nil {
				return err
			}
			if err := visit(TileJob{Tile: maptile.New(x, y, maptile.Zoom(z)), URL: url}); err != nil {
				return err
			}
		}
		return rows.Err()
	}
	return cleanup, nil
}
