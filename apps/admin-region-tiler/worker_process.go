package main

import (
	"context"
	"errors"
	"fmt"
	"io"
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
	done         chan struct{}
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
		// The manager handles this exit. A delayed worker must not write before
		// obtaining its run lock and passing its ownership check below.
		return err
	}
	task.workerManaged = true
	defer task.cancel()
	parentWatch := watchWorkerParent(task)
	runLock, err := lockRun(task.ctx, taskRecord, run.ID)
	if err != nil {
		if parentWatch.gone.Load() {
			return nil
		}
		return err
	}
	defer runLock.Close()
	owned, err := store.workerOwnsRun(taskRecord.ID)
	if err != nil {
		return err
	}
	if !owned || parentWatch.gone.Load() {
		return nil
	}
	failBeforeStart := func(cause error) error {
		if parentWatch.gone.Load() {
			return nil
		}
		return failRunBeforeStart(taskRecord, run, cause)
	}
	runDir, err := managedRunDirectory(taskRecord, run.ID)
	if err != nil {
		return failBeforeStart(err)
	}
	task.expectedPath = filepath.Join(runDir, "expected.sqlite")
	task.expectedLimit = maxTaskTiles()
	_ = store.setRunPhase(run.ID, "preparing", 0, 0)
	task.expectedProgress = func(current, estimate int64) error {
		if err := store.setRunPhase(run.ID, "enumerating", current, estimate); err != nil {
			log.Warnf("failed to persist enumeration progress for run %s: %v", run.ID, err)
		}
		return nil
	}
	controller := &workerController{taskRecordID: taskRecordID, task: task, stop: make(chan struct{})}
	if err := controller.sync(); err != nil {
		return failBeforeStart(err)
	}
	controller.start()
	defer controller.stopLoop()
	task.File = run.OutputPath
	if task.File != "" {
		if err := os.MkdirAll(filepath.Dir(task.File), directoryPermissions); err != nil {
			return failBeforeStart(err)
		}
	}
	expected, err := task.expectedCount()
	task.expectedProgress = nil
	if err != nil {
		return failBeforeStart(err)
	}
	if err := retryOnBusy(func() error { return store.reserveTaskBudget(taskRecord, expected) }); err != nil {
		return failBeforeStart(err)
	}
	resuming := os.Getenv(workerResumeEnv) == "1"
	task.preserveOutput = resuming
	if run.TriggerMode == triggerRetryFailures {
		if err := store.prepareRetryBaseline(taskRecord, task); err != nil {
			_ = failBeforeStart(err)
			return err
		}
		summary, err := store.failureSummary(taskRecord.ID)
		if err != nil {
			_ = failBeforeStart(err)
			return err
		}
		if summary.Retryable == 0 {
			_ = failBeforeStart(errNoRetryableFailures)
			return errNoRetryableFailures
		}
		task.Total = summary.Retryable
		task.retrySource = func(visit func(TileJob) error) error { return store.walkRetryFailures(taskRecord.ID, visit) }
	} else {
		task.Total = expected
	}
	if resuming {
		cleanup, err := prepareResumeJobs(taskRecord, task, run)
		if err != nil {
			return failBeforeStart(err)
		}
		defer cleanup()
	}
	task.failureSink = store.failureSink(taskRecord, run.ID)

	run.Total = task.Total
	run.Status = TaskRunning
	run.StartedAt = timePtrOrNow(run.StartedAt)
	_ = store.setRunPhase(run.ID, "downloading", task.Current, task.Total)
	if err := retryOnBusy(func() error { return store.updateRunProgress(run) }); err != nil {
		return err
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
				if parentWatch.gone.Load() {
					return
				}
				persistRunProgress(run, task)
			case <-done:
				return
			}
		}
	}()

	task.Run()
	close(done)
	<-progressDone
	if parentWatch.gone.Load() {
		return nil
	}

	persistRunProgress(run, task)
	applyTaskSnapshot(run, task)

	state := IntegrityState{Status: "unchecked"}
	if run.Status == TaskCompleted || run.Status == TaskPartialFailed {
		_ = store.setRunPhase(run.ID, "verifying", 0, expected)
		state, err = store.inspectOutput(taskRecord, task, run, true)
		if err != nil {
			run.Status = TaskFailed
			if errors.Is(err, context.Canceled) {
				run.Status = TaskCancelled
			}
			run.ErrorMessage = err.Error()
		}
		if err == nil && state.Missing > 0 {
			run.Status = TaskPartialFailed
			task.setStatus(TaskPartialFailed)
		}
	}
	if parentWatch.gone.Load() {
		return nil
	}
	if run.Status == TaskCompleted || run.Status == TaskPartialFailed {
		if err := prepareDerivedArtifacts(taskRecord, task, run); err != nil {
			run.Status, run.ErrorMessage = TaskFailed, err.Error()
			if errors.Is(err, context.Canceled) {
				run.Status = TaskCancelled
			}
		}
		if task.derivedIntegrity != nil {
			state = *task.derivedIntegrity
			state.RunID = run.ID
			if state.Status != "complete" && run.Status != TaskFailed && run.Status != TaskCancelled {
				run.Status = TaskPartialFailed
			}
		}
	}
	_ = store.setRunPhase(run.ID, "packing", 0, 0)
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
	if parentWatch.gone.Load() {
		return nil
	}

	finalize := func() error { return store.finalizeRun(run) }
	if run.ArtifactStatus == ArtifactReady {
		_ = store.setRunPhase(run.ID, "publishing", run.Current, run.Total)
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
	_ = store.setRunPhase(run.ID, string(run.Status), run.Current, run.Total)
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

func launchWorkerProcess(taskRecordID, runID string, resuming bool) (*exec.Cmd, io.WriteCloser, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, nil, err
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
	owner := fmt.Sprintf("%s:%d:%d", runID, os.Getpid(), time.Now().UnixNano())
	if _, err := store.db.Exec(`UPDATE execution_queue SET owner_token=? WHERE plan_id=?`, owner, taskRecordID); err != nil {
		return nil, nil, err
	}
	cmd.Env = append(cmd.Env, workerParentGuardEnv+"=1", workerOwnerEnv+"="+owner, workerResumeEnv+"=0")
	if resuming {
		cmd.Env = append(cmd.Env, workerResumeEnv+"=1")
	}
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	cmd.Stdin = readPipe
	if err := cmd.Start(); err != nil {
		readPipe.Close()
		writePipe.Close()
		return nil, nil, err
	}
	readPipe.Close()
	return cmd, writePipe, nil
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
	c.done = make(chan struct{})
	ticker := time.NewTicker(100 * time.Millisecond)
	go func() {
		defer close(c.done)
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
	if c.done != nil {
		<-c.done
	}
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
		if status == TaskRunning || status == TaskPending || status == TaskCompleted || status == TaskPartialFailed {
			return c.task.Pause()
		}
	case TaskRecordRunning:
		if status == TaskPaused {
			return c.task.Resume()
		}
	case TaskRecordCancelled:
		if status != TaskCancelled && status != TaskFailed {
			return c.task.Cancel()
		}
	case TaskRecordQueued:
		if status == TaskRunning {
			return c.task.Pause()
		}
	}

	return nil
}

func persistRunProgress(run *TaskRunRecord, task *Task) {
	applyTaskSnapshot(run, task)
	if err := retryOnBusy(func() error { return store.updateRunProgress(run) }); err != nil {
		log.Errorf("failed to update run progress %s: %v", run.ID, err)
	}
	_, _ = store.db.Exec(`UPDATE run_phases SET current=?,total=? WHERE run_id=? AND phase='downloading'`, run.Current, run.Total, run.ID)
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
	sourceDir := taskSnapshot.File
	if task.artifactDirectory != "" {
		sourceDir = task.artifactDirectory
	}
	zipPath := filepath.Join(artifactDir, "artifact.zip")
	stagingPath := zipPath + ".partial"
	defer os.Remove(stagingPath)
	var lastProgress time.Time
	if err := zipDirectory(sourceDir, stagingPath, func(current, total int) error {
		if task.workerManaged {
			if err := task.waitIfPaused(); err != nil {
				return err
			}
		}
		now := time.Now()
		if current != 0 && current != total && now.Sub(lastProgress) < 200*time.Millisecond {
			return nil
		}
		lastProgress = now
		latest, err := store.getTaskRecordByID(taskRecord.ID)
		if err != nil {
			return err
		}
		if latest.Status == TaskRecordCancelled {
			return context.Canceled
		}
		run.ArtifactName = fmt.Sprintf("压缩中：%d/%d", current, total)
		_ = store.setRunPhase(run.ID, "packing", int64(current), int64(total))
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
		if err := os.Remove(zipPath); err != nil {
			return err
		}
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
	if errors.Is(cause, context.Canceled) {
		run.Status = TaskCancelled
	}
	run.ErrorMessage = cause.Error()
	run.FinishedAt = &now
	if run.StartedAt == nil {
		run.StartedAt = &now
	}
	if err := retryOnBusy(func() error { return store.finalizeRun(run) }); err != nil {
		return err
	}
	_ = store.setRunPhase(run.ID, string(run.Status), run.Current, run.Total)
	return retryOnBusy(func() error { return store.updateTaskRecordStatus(taskRecord.ID, statusToTaskRecordStatus(run.Status)) })
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
