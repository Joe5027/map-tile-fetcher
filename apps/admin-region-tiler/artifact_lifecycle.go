package main

import (
	"archive/zip"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

func managedRunDirectory(plan *TaskRecord, runID string) (string, error) {
	parent := plan.ParentID
	if parent == "" {
		parent = plan.ID
	}
	if parent == "" || plan.ID == "" || runID == "" {
		return "", errors.New("task and run IDs are required")
	}
	encode := func(id string) string { return hex.EncodeToString([]byte(id)) }
	root := viper.GetString("output.directory")
	return ensurePathWithinRoot(filepath.Join(root, "tasks", encode(parent), encode(plan.ID), encode(runID)), root, false)
}

func validateArchive(path string) error {
	r, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, file := range r.File {
		reader, err := file.Open()
		if err != nil {
			return err
		}
		_, copyErr := io.Copy(io.Discard, reader)
		closeErr := reader.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
	}
	return nil
}

func (s *SQLiteStore) publishedRun(planID string) (*TaskRunRecord, error) {
	var id string
	err := s.db.QueryRow(`SELECT id FROM task_runs WHERE plan_id = ? AND artifact_status = 'ready' AND artifact_path <> '' ORDER BY created_at DESC, rowid DESC LIMIT 1`, planID).Scan(&id)
	if err != nil {
		return nil, err
	}
	return s.getRun(id)
}

func (s *SQLiteStore) checkNotDeleting(planID string) error {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM task_deletions WHERE plan_id = ? OR plan_id = (SELECT parent_id FROM plans WHERE id = ?)`, planID, planID).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		return errors.New("task cleanup is pending; retry deletion")
	}
	return nil
}

func (m *RuntimeManager) purgeManagedTask(plan *TaskRecord) error {
	fresh, err := store.getTaskRecordByID(plan.ID)
	if errors.Is(err, errTaskNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	plans := []*TaskRecord{fresh}
	children, err := store.listTaskChildrenByParent(plan.ID)
	if err != nil {
		return err
	}
	plans = append(plans, children...)
	ids := make(map[string]bool)
	paths := make(map[string]string)
	for _, p := range plans {
		ids[p.ID] = true
		if _, activeErr := m.getActive(p.ID); activeErr == nil || p.Status == TaskRecordRunning || p.Status == TaskRecordPaused {
			return errors.New("运行中、暂停或打包中的任务不能删除，请先取消并等待结束。")
		}
		runs, err := store.listRunsByTaskRecord(p.ID)
		if err != nil {
			return err
		}
		for _, run := range runs {
			if run.ArtifactStatus == ArtifactPacking {
				return errors.New("artifact is still packing")
			}
			for _, path := range collectTaskPaths([]*TaskRunRecord{run}) {
				paths[path] = "output"
			}
			dir, err := managedRunDirectory(p, run.ID)
			if err != nil {
				return err
			}
			paths[dir] = "output"
		}
		for _, path := range generatedAreaPaths(p) {
			paths[path] = "area"
		}
	}
	// Legacy runs can share filenames. Keep any path referenced outside this
	// deletion group, including directories containing a surviving artifact.
	rows, err := store.db.Query(`SELECT plan_id, output_path, artifact_path FROM task_runs`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, output, artifact string
		if err := rows.Scan(&id, &output, &artifact); err != nil {
			rows.Close()
			return err
		}
		if ids[id] {
			continue
		}
		for path, kind := range paths {
			if kind != "output" {
				continue
			}
			for _, other := range []string{output, artifact} {
				if other != "" && pathsOverlap(path, other) {
					delete(paths, path)
				}
			}
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	rows, err = store.db.Query(`SELECT id, levels_json FROM plans`)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, levels string
		if err := rows.Scan(&id, &levels); err != nil {
			rows.Close()
			return err
		}
		if ids[id] {
			continue
		}
		var configs []LevelConfig
		if err := json.Unmarshal([]byte(levels), &configs); err != nil {
			rows.Close()
			return err
		}
		for _, path := range generatedAreaPaths(&TaskRecord{Levels: configs}) {
			delete(paths, path)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	// Persist the complete manifest before removing the first file. A failed
	// cleanup leaves both task records and the manifest available for retry.
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT OR IGNORE INTO task_deletions(plan_id) VALUES (?)`, plan.ID); err != nil {
		return err
	}
	for path, kind := range paths {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO deletion_paths(plan_id,path,kind) VALUES (?,?,?)`, plan.ID, path, kind); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	rows, err = store.db.Query(`SELECT path,kind FROM deletion_paths WHERE plan_id = ? AND done = 0`, plan.ID)
	if err != nil {
		return err
	}
	pending := make(map[string]string)
	for rows.Next() {
		var path, kind string
		if err := rows.Scan(&path, &kind); err != nil {
			rows.Close()
			return err
		}
		pending[path] = kind
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()
	for path, kind := range pending {
		if kind == "area" {
			err = removeGeneratedAreaPath(path)
		} else {
			err = removeTaskPath(path)
		}
		if err != nil {
			_, _ = store.db.Exec(`UPDATE task_deletions SET error_message = ? WHERE plan_id = ?`, err.Error(), plan.ID)
			return err
		}
		if _, err := store.db.Exec(`UPDATE deletion_paths SET done = 1 WHERE plan_id = ? AND path = ?`, plan.ID, path); err != nil {
			return err
		}
	}
	return store.purgeTaskRecord(plan.ID)
}

func pathsOverlap(a, b string) bool {
	a, errA := filepath.Abs(a)
	b, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return true
	}
	rel, err := filepath.Rel(a, b)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}
