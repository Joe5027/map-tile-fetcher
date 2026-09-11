package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/paulmach/orb/maptile"
	log "github.com/sirupsen/logrus"
)

const workerDBRetryAttempts = 12
const workerDBRetryDelay = 300 * time.Millisecond

type workerController struct {
	taskRecordID string
	task         *Task
	stop         chan struct{}
	once         sync.Once
}

func runWorkerProcess(taskRecordID, runID string) error {
	var taskRecord *TaskRecord
	err := retryOnBusy(func() error {
		var innerErr error
		taskRecord, innerErr = store.getTaskRecordByID(taskRecordID)
		return innerErr
	})
	if err != nil {
		return err
	}
	var run *TaskRunRecord
	err = retryOnBusy(func() error {
		var innerErr error
		run, innerErr = store.getRun(runID)
		return innerErr
	})
	if err != nil {
		return err
	}

	task, err := buildTaskFromRecord(taskRecord)
	if err != nil {
		_ = failRunBeforeStart(taskRecord, run, err)
		return err
	}
	task.File = run.OutputPath
	if task.File != "" {
		if err := os.MkdirAll(filepath.Dir(task.File), directoryPermissions); err != nil {
			return failRunBeforeStart(taskRecord, run, err)
		}
	}
	if run.TriggerMode == triggerRetryFailures {
		previous, err := store.validateRetryBaseline(taskRecord)
		if err != nil {
			_ = failRunBeforeStart(taskRecord, run, err)
			return err
		}
		if err := copyRetryBaseline(taskRecord, task, previous); err != nil {
			_ = failRunBeforeStart(taskRecord, run, err)
			return err
		}
		summary, err := store.failureSummary(taskRecord.ID)
		if err != nil {
			_ = failRunBeforeStart(taskRecord, run, err)
			return err
		}
		if summary.Retryable == 0 {
			_ = failRunBeforeStart(taskRecord, run, errNoRetryableFailures)
			return errNoRetryableFailures
		}
		task.Total = summary.Retryable
		task.retrySource = func(visit func(TileJob) error) error { return store.walkRetryFailures(taskRecord.ID, visit) }
	} else {
		var count int64
		if err := task.forEachExpected(func(TileJob) error { count++; return nil }); err != nil {
			return failRunBeforeStart(taskRecord, run, err)
		}
		task.Total = count
	}
	task.failureSink = store.failureSink(taskRecord, run.ID)

	run.Total = task.Total
	run.Status = TaskRunning
	run.StartedAt = timePtrOrNow(run.StartedAt)
	if err := retryOnBusy(func() error { return store.updateRunProgress(run) }); err != nil {
		return err
	}

	controller := &workerController{
		taskRecordID: taskRecordID,
		task:         task,
		stop:         make(chan struct{}),
	}

	done := make(chan struct{})
	progressDone := make(chan struct{})
	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	go func() {
		defer close(progressDone)
		for {
			select {
			case <-progressTicker.C:
				persistRunProgress(run, task)
			case <-done:
				return
			}
		}
	}()

	controller.start()
	task.Run()
	controller.stopLoop()
	close(done)
	<-progressDone

	persistRunProgress(run, task)
	applyTaskSnapshot(run, task)

	state := IntegrityState{Status: "unchecked"}
	if run.Status == TaskCompleted || run.Status == TaskPartialFailed {
		state, err = store.inspectOutput(taskRecord, task, run, true)
		if err != nil {
			run.Status = TaskFailed
			run.ErrorMessage = err.Error()
		}
		if state.Missing > 0 {
			run.Status = TaskPartialFailed
			task.setStatus(TaskPartialFailed)
		}
	}
	if err := prepareArtifactForRun(taskRecord, task, run); err != nil {
		run.ArtifactStatus = ArtifactFailed
		run.Status = TaskFailed
		if errors.Is(err, context.Canceled) {
			run.Status = TaskCancelled
		}
		if run.ErrorMessage == "" {
			run.ErrorMessage = err.Error()
		}
	} else if run.ArtifactPath != "" {
		run.ArtifactStatus = ArtifactReady
	}

	finalize := func() error { return store.finalizeRun(run) }
	if run.ArtifactStatus == ArtifactReady {
		finalize = func() error { return store.publishRun(run, state) }
	}
	if err := retryOnBusy(finalize); err != nil {
		return err
	}
	if err := retryOnBusy(func() error { return store.updateTaskRecordStatus(taskRecord.ID, statusToTaskRecordStatus(run.Status)) }); err != nil {
		return err
	}
	if taskRecord.ParentID != "" {
		manager := NewRuntimeManager()
		if err := retryOnBusy(func() error { return manager.refreshParentStatus(taskRecord.ParentID) }); err != nil {
			log.Errorf("failed to refresh parent task record %s from worker: %v", taskRecord.ParentID, err)
		}
	}
	return nil
}

func tileJobsFromFailureRecords(records []FailureRecord) []TileJob {
	jobs := make([]TileJob, 0, len(records))
	seen := make(map[string]struct{}, len(records))
	for _, record := range records {
		if !record.Retryable || record.Z < 0 || record.X < 0 || record.Y < 0 || strings.TrimSpace(record.URL) == "" {
			continue
		}
		key := fmt.Sprintf("%d/%d/%d/%s", record.Z, record.X, record.Y, strings.TrimSpace(record.URL))
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		jobs = append(jobs, TileJob{
			Tile: maptile.New(uint32(record.X), uint32(record.Y), maptile.Zoom(record.Z)),
			URL:  strings.TrimSpace(record.URL),
		})
	}
	return jobs
}

func launchWorkerProcess(taskRecordID, runID string) (*exec.Cmd, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-c", resolveConfigPath(cf),
		"-worker-task-record-id", taskRecordID,
		"-worker-run-id", runID,
	}

	cmd := exec.Command(exePath, args...)
	cmd.Dir = workerProcessWorkingDir(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
}

func workerProcessWorkingDir(exePath string) string {
	workingDir, err := os.Getwd()
	if err == nil && strings.TrimSpace(workingDir) != "" {
		return workingDir
	}
	return filepath.Dir(exePath)
}

func resolveConfigPath(path string) string {
	if strings.TrimSpace(path) == "" {
		return "conf.toml"
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return path
	}
	return absolute
}

func (c *workerController) start() {
	ticker := time.NewTicker(1 * time.Second)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				if err := c.sync(); err != nil {
					log.Errorf("worker control sync failed for task record %s: %v", c.taskRecordID, err)
				}
			case <-c.stop:
				return
			}
		}
	}()
}

func (c *workerController) stopLoop() {
	c.once.Do(func() {
		close(c.stop)
	})
}

func (c *workerController) sync() error {
	var plan *TaskRecord
	err := retryOnBusy(func() error {
		var innerErr error
		plan, innerErr = store.getTaskRecordByID(c.taskRecordID)
		return innerErr
	})
	if err != nil {
		return err
	}

	c.task.mu.RLock()
	status := c.task.Status
	c.task.mu.RUnlock()

	switch plan.Status {
	case TaskRecordPaused:
		if status == TaskRunning {
			return c.task.Pause()
		}
	case TaskRecordRunning:
		if status == TaskPaused {
			return c.task.Resume()
		}
	case TaskRecordCancelled:
		if status != TaskCancelled && status != TaskCompleted && status != TaskFailed {
			return c.task.Cancel()
		}
	}

	return nil
}

func persistRunProgress(run *TaskRunRecord, task *Task) {
	applyTaskSnapshot(run, task)
	if err := retryOnBusy(func() error { return store.updateRunProgress(run) }); err != nil {
		log.Errorf("failed to update run progress %s: %v", run.ID, err)
	}
}

func applyTaskSnapshot(run *TaskRunRecord, task *Task) {
	snapshot := task.snapshot()
	run.Status = TaskStatus(snapshot.Status)
	run.Total = snapshot.Total
	run.Current = snapshot.Current
	run.SuccessCount = snapshot.SuccessCount
	run.FailureCount = snapshot.FailureCount
	run.ErrorMessage = snapshot.ErrorMessage
	run.OutputPath = snapshot.File
	if snapshot.StartedAt != "" {
		startedAt, err := time.Parse(time.RFC3339, snapshot.StartedAt)
		if err == nil {
			run.StartedAt = &startedAt
		}
	}
	if snapshot.FinishedAt != "" {
		finishedAt, err := time.Parse(time.RFC3339, snapshot.FinishedAt)
		if err == nil {
			run.FinishedAt = &finishedAt
		}
	}
}

func prepareArtifactForRun(taskRecord *TaskRecord, task *Task, run *TaskRunRecord) error {
	taskSnapshot := task.snapshot()
	if run.Status == TaskCancelled || run.Status == TaskFailed {
		return nil
	}
	if taskSnapshot.File == "" {
		return nil
	}

	artifactDir, err := managedRunDirectory(taskRecord, run.ID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(artifactDir, directoryPermissions); err != nil {
		return err
	}

	if strings.EqualFold(task.outformat, "mbtiles") || strings.HasSuffix(strings.ToLower(taskSnapshot.File), ".mbtiles") {
		run.ArtifactPath = taskSnapshot.File
		run.ArtifactName = strings.TrimSuffix(archiveFileName(taskRecord.Name, taskRecord.SourceName), ".zip") + ".mbtiles"
		return nil
	}

	run.ArtifactStatus = ArtifactPacking
	zipPath := filepath.Join(artifactDir, "artifact.zip")
	stagingPath := zipPath + ".partial"
	defer os.Remove(stagingPath)
	if err := zipDirectory(taskSnapshot.File, stagingPath, func(current, total int) error {
		latest, err := store.getTaskRecordByID(taskRecord.ID)
		if err != nil {
			return err
		}
		if latest.Status == TaskRecordCancelled {
			return context.Canceled
		}
		run.ArtifactName = fmt.Sprintf("压缩中：%d/%d", current, total)
		if err := retryOnBusy(func() error { return store.updateRunProgress(run) }); err != nil {
			log.Warnf("failed to persist archive progress for run %s: %v", run.ID, err)
		}
		return nil
	}); err != nil {
		return err
	}
	if err := validateArchive(stagingPath); err != nil {
		return err
	}
	if _, err := os.Stat(zipPath); err == nil {
		return fmt.Errorf("run artifact already exists")
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.Rename(stagingPath, zipPath); err != nil {
		return err
	}
	run.ArtifactPath = zipPath
	run.ArtifactName = archiveFileName(taskRecord.Name, taskRecord.SourceName)
	return nil
}

func archiveFileName(taskName, childName string) string {
	taskName = strings.TrimSpace(taskName)
	childName = strings.TrimSpace(childName)
	if taskName == "" {
		taskName = "task"
	}
	if childName == "" {
		childName = "subtask"
	}
	return safeFilePart(taskName) + "-" + safeFilePart(childName) + ".zip"
}

func safeFilePart(value string) string {
	value = strings.Trim(value, " .")
	value = strings.Map(func(r rune) rune {
		if r < 0x20 || strings.ContainsRune(`\\/:*?"<>|`, r) {
			return '_'
		}
		return r
	}, value)
	runes := []rune(strings.Trim(value, " ."))
	if len(runes) > 40 {
		runes = runes[:40]
	}
	if len(runes) == 0 {
		return "task"
	}
	return string(runes)
}

func finalizeUnexpectedWorkerExit(taskRecord *TaskRecord, run *TaskRunRecord, waitErr error) {
	var refreshed *TaskRunRecord
	err := retryOnBusy(func() error {
		var innerErr error
		refreshed, innerErr = store.getRun(run.ID)
		return innerErr
	})
	if err != nil {
		log.Errorf("failed to reload run %s after worker exit: %v", run.ID, err)
		return
	}

	switch refreshed.Status {
	case TaskCompleted, TaskPartialFailed, TaskCancelled, TaskFailed:
		return
	}

	now := time.Now()
	refreshed.Status = TaskFailed
	refreshed.FinishedAt = &now
	if waitErr != nil {
		refreshed.ErrorMessage = fmt.Sprintf("worker process exited unexpectedly: %v", waitErr)
	} else {
		refreshed.ErrorMessage = "worker process exited unexpectedly"
	}
	if err := retryOnBusy(func() error { return store.finalizeRun(refreshed) }); err != nil {
		log.Errorf("failed to finalize unexpected worker exit for run %s: %v", refreshed.ID, err)
	}
	if err := retryOnBusy(func() error { return store.updateTaskRecordStatus(taskRecord.ID, TaskRecordFailed) }); err != nil {
		log.Errorf("failed to mark task record %s failed after worker exit: %v", taskRecord.ID, err)
	}
}

func failRunBeforeStart(taskRecord *TaskRecord, run *TaskRunRecord, cause error) error {
	now := time.Now()
	run.Status = TaskFailed
	run.ErrorMessage = cause.Error()
	run.FinishedAt = &now
	if run.StartedAt == nil {
		run.StartedAt = &now
	}
	if err := retryOnBusy(func() error { return store.finalizeRun(run) }); err != nil {
		return err
	}
	return retryOnBusy(func() error { return store.updateTaskRecordStatus(taskRecord.ID, TaskRecordFailed) })
}

func timePtrOrNow(t *time.Time) *time.Time {
	if t != nil {
		return t
	}
	now := time.Now()
	return &now
}

func retryOnBusy(fn func() error) error {
	var lastErr error
	for attempt := 0; attempt < workerDBRetryAttempts; attempt++ {
		if err := fn(); err != nil {
			lastErr = err
			if !isSQLiteBusy(err) {
				return err
			}
			time.Sleep(workerDBRetryDelay)
			continue
		}
		return nil
	}
	return lastErr
}

func isSQLiteBusy(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToUpper(err.Error())
	return strings.Contains(msg, "SQLITE_BUSY") || strings.Contains(msg, "DATABASE IS LOCKED")
}
