package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

const workerDBRetryAttempts = 12
const workerDBRetryDelay = 300 * time.Millisecond

type workerController struct {
	planID string
	task   *Task
	stop   chan struct{}
	once   sync.Once
}

func runWorkerProcess(planID, runID string) error {
	var plan *PlanRecord
	err := retryOnBusy(func() error {
		var innerErr error
		plan, innerErr = store.getPlanByID(planID)
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

	task, err := buildTaskFromPlan(plan)
	if err != nil {
		_ = failRunBeforeStart(plan, run, err)
		return err
	}

	run.Total = task.Total
	run.Status = TaskRunning
	run.StartedAt = timePtrOrNow(run.StartedAt)
	if err := retryOnBusy(func() error { return store.updateRunProgress(run) }); err != nil {
		return err
	}

	controller := &workerController{
		planID: planID,
		task:   task,
		stop:   make(chan struct{}),
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
	if err := retryOnBusy(func() error { return store.updatePlanStatus(plan.ID, statusToPlanStatus(run.Status)) }); err != nil {
		return err
	}
	if plan.ParentID != "" {
		manager := NewRuntimeManager()
		if err := retryOnBusy(func() error { return manager.refreshParentStatus(plan.ParentID) }); err != nil {
			log.Errorf("failed to refresh parent plan %s from worker: %v", plan.ParentID, err)
		}
	}
	return nil
}

func launchWorkerProcess(planID, runID string) (*exec.Cmd, error) {
	exePath, err := os.Executable()
	if err != nil {
		return nil, err
	}

	args := []string{
		"-c", resolveConfigPath(cf),
		"-worker-plan-id", planID,
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
					log.Errorf("worker control sync failed for plan %s: %v", c.planID, err)
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
	var plan *PlanRecord
	err := retryOnBusy(func() error {
		var innerErr error
		plan, innerErr = store.getPlanByID(c.planID)
		return innerErr
	})
	if err != nil {
		return err
	}

	c.task.mu.RLock()
	status := c.task.Status
	c.task.mu.RUnlock()

	switch plan.Status {
	case PlanPaused:
		if status == TaskRunning {
			return c.task.Pause()
		}
	case PlanRunning:
		if status == TaskPaused {
			return c.task.Resume()
		}
	case PlanCancelled:
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

func finalizeUnexpectedWorkerExit(plan *PlanRecord, run *TaskRunRecord, waitErr error) {
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
	if err := retryOnBusy(func() error { return store.updatePlanStatus(plan.ID, PlanFailed) }); err != nil {
		log.Errorf("failed to mark plan %s failed after worker exit: %v", plan.ID, err)
	}
}

func failRunBeforeStart(plan *PlanRecord, run *TaskRunRecord, cause error) error {
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
	return retryOnBusy(func() error { return store.updatePlanStatus(plan.ID, PlanFailed) })
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
