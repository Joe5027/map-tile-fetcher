package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/paulmach/orb/maptile"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
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
	if run.TriggerMode == triggerRetryFailures {
		var records []FailureRecord
		err = retryOnBusy(func() error {
			var innerErr error
			records, innerErr = store.listRetryableFailureRecords(taskRecord.ID)
			return innerErr
		})
		if err != nil {
			_ = failRunBeforeStart(taskRecord, run, err)
			return err
		}
		retryJobs := tileJobsFromFailureRecords(records)
		if len(retryJobs) == 0 {
			_ = failRunBeforeStart(taskRecord, run, errNoRetryableFailures)
			return errNoRetryableFailures
		}
		task.SetExplicitJobs(retryJobs)
	}

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
	progressTicker := time.NewTicker(1 * time.Second)
	defer progressTicker.Stop()

	go func() {
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

	persistRunProgress(run, task)
	applyTaskSnapshot(run, task)

	if err := retryOnBusy(func() error { return store.replaceFailureRecords(run, task.FailureRecords()) }); err != nil {
		return err
	}

	if err := prepareArtifactForRun(task, run); err != nil {
		run.ArtifactStatus = ArtifactFailed
		if run.ErrorMessage == "" {
			run.ErrorMessage = err.Error()
		}
	} else if run.ArtifactPath != "" {
		run.ArtifactStatus = ArtifactReady
	}

	if err := retryOnBusy(func() error { return store.finalizeRun(run) }); err != nil {
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
	cmd.Dir = filepath.Dir(exePath)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Env = os.Environ()
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	return cmd, nil
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
	run.Status = task.Status
	run.Total = task.Total
	run.Current = task.Current
	run.SuccessCount = task.SuccessCount
	run.FailureCount = task.FailureCount
	run.ErrorMessage = task.ErrorMessage
	run.OutputPath = task.File
	run.StartedAt = task.StartedAt
	run.FinishedAt = task.FinishedAt
}

func prepareArtifactForRun(task *Task, run *TaskRunRecord) error {
	if task.File == "" {
		return nil
	}

	artifactDir := filepath.Join(viper.GetString("output.directory"), "_artifacts")
	if err := os.MkdirAll(artifactDir, os.ModePerm); err != nil {
		return err
	}

	if strings.EqualFold(task.outformat, "mbtiles") || strings.HasSuffix(strings.ToLower(task.File), ".mbtiles") {
		run.ArtifactPath = task.File
		run.ArtifactName = filepath.Base(task.File)
		return nil
	}

	run.ArtifactStatus = ArtifactPacking
	zipPath := filepath.Join(artifactDir, run.ID+".zip")
	if err := zipDirectory(task.File, zipPath); err != nil {
		return err
	}
	run.ArtifactPath = zipPath
	run.ArtifactName = filepath.Base(zipPath)
	return nil
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
	case TaskCompleted, TaskCancelled, TaskFailed:
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
