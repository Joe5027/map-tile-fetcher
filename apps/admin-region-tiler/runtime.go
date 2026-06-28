package main

import (
	"archive/zip"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/teris-io/shortid"
)

type ActiveRun struct {
	Plan *TaskRecord
	Run  *TaskRunRecord
	Cmd  *exec.Cmd
}

type RuntimeManager struct {
	mu     sync.RWMutex
	active map[string]*ActiveRun
}

type Scheduler struct {
	manager *RuntimeManager
	ticker  *time.Ticker
	stop    chan struct{}
}

var runtimeManager *RuntimeManager

const triggerRetryFailures = "retry_failures"

var errNoRetryableFailures = errors.New("no retryable failures")

func NewRuntimeManager() *RuntimeManager {
	return &RuntimeManager{
		active: make(map[string]*ActiveRun),
	}
}

func NewScheduler(manager *RuntimeManager) *Scheduler {
	return &Scheduler{
		manager: manager,
		stop:    make(chan struct{}),
	}
}

func (s *Scheduler) Start() {
	s.ticker = time.NewTicker(2 * time.Second)
	go func() {
		for {
			select {
			case <-s.ticker.C:
				s.dispatchDueTaskRecords()
			case <-s.stop:
				s.ticker.Stop()
				return
			}
		}
	}()
	s.dispatchDueTaskRecords()
}

func (s *Scheduler) Stop() {
	close(s.stop)
}

func (s *Scheduler) dispatchDueTaskRecords() {
	plans, err := store.listDueTaskRecords(time.Now())
	if err != nil {
		log.Errorf("failed to list due plans: %v", err)
		return
	}
	for _, plan := range plans {
		if err := s.manager.StartTaskRecord(plan); err != nil && !errors.Is(err, errTaskAlreadyActive) {
			log.Errorf("failed to start plan %s: %v", plan.ID, err)
		}
	}
}

var errTaskAlreadyActive = errors.New("task already active")

func (m *RuntimeManager) StartTaskRecord(plan *TaskRecord) error {
	return m.startTaskRecordWithTrigger(plan, string(plan.ScheduleMode))
}

func (m *RuntimeManager) RetryFailures(plan *TaskRecord) error {
	return m.startTaskRecordWithTrigger(plan, triggerRetryFailures)
}

func (m *RuntimeManager) startTaskRecordWithTrigger(plan *TaskRecord, triggerMode string) error {
	if plan.Kind == TaskRecordKindGroup {
		children, err := store.listTaskChildrenByParent(plan.ID)
		if err != nil {
			return err
		}
		if triggerMode == triggerRetryFailures {
			eligible := make([]*TaskRecord, 0, len(children))
			for _, child := range children {
				if child.Status == TaskRecordCancelled {
					continue
				}
				summary, err := store.failureSummary(child.ID)
				if err != nil {
					return err
				}
				if summary.Retryable > 0 {
					eligible = append(eligible, child)
				}
			}
			if len(eligible) == 0 {
				return errNoRetryableFailures
			}
			children = eligible
		}
		if err := store.updateTaskRecordStatus(plan.ID, TaskRecordRunning); err != nil {
			return err
		}
		var started bool
		for _, child := range children {
			if child.Status == TaskRecordCancelled {
				continue
			}
			if err := m.startTaskRecordWithTrigger(child, triggerMode); err == nil {
				started = true
				continue
			} else if !errors.Is(err, errTaskAlreadyActive) {
				log.Errorf("failed to start child plan %s: %v", child.ID, err)
			}
		}
		if !started {
			if triggerMode == triggerRetryFailures {
				return errNoRetryableFailures
			}
			return m.refreshParentStatus(plan.ID)
		}
		return nil
	}

	if triggerMode == triggerRetryFailures {
		summary, err := store.failureSummary(plan.ID)
		if err != nil {
			return err
		}
		if summary.Retryable == 0 {
			return errNoRetryableFailures
		}
	}

	m.mu.Lock()
	if _, exists := m.active[plan.ID]; exists {
		m.mu.Unlock()
		return errTaskAlreadyActive
	}
	m.mu.Unlock()

	task, err := buildTaskFromRecord(plan)
	if err != nil {
		_ = store.updateTaskRecordStatus(plan.ID, TaskRecordFailed)
		return err
	}

	runID, _ := shortid.Generate()
	now := time.Now()
	run := &TaskRunRecord{
		ID:             runID,
		TaskRecordID:   plan.ID,
		UserID:         plan.UserID,
		Status:         TaskRunning,
		TriggerMode:    triggerMode,
		ArtifactStatus: ArtifactNone,
		StartedAt:      &now,
		Total:          task.Total,
	}

	if err := store.createRun(run); err != nil {
		return err
	}
	if err := store.markTaskRecordRunning(plan.ID, runID); err != nil {
		return err
	}

	cmd, err := launchWorkerProcess(plan.ID, run.ID)
	if err != nil {
		_ = failRunBeforeStart(plan, run, err)
		return err
	}

	active := &ActiveRun{Plan: plan, Run: run, Cmd: cmd}
	m.mu.Lock()
	m.active[plan.ID] = active
	m.mu.Unlock()

	go m.monitorWorker(active)
	return nil
}

func (m *RuntimeManager) monitorWorker(active *ActiveRun) {
	waitErr := active.Cmd.Wait()
	if waitErr != nil {
		log.Errorf("worker process for plan %s exited with error: %v", active.Plan.ID, waitErr)
	}

	finalizeUnexpectedWorkerExit(active.Plan, active.Run, waitErr)

	if active.Plan.ParentID != "" {
		if err := m.refreshParentStatus(active.Plan.ParentID); err != nil {
			log.Errorf("failed to update parent plan status %s: %v", active.Plan.ParentID, err)
		}
	}

	m.mu.Lock()
	delete(m.active, active.Plan.ID)
	m.mu.Unlock()
}

func (m *RuntimeManager) Pause(planID string) error {
	plan, err := store.getTaskRecordByID(planID)
	if err == nil && plan.Kind == TaskRecordKindGroup {
		children, listErr := store.listTaskChildrenByParent(planID)
		if listErr != nil {
			return listErr
		}
		var paused bool
		for _, child := range children {
			if childErr := m.Pause(child.ID); childErr == nil {
				paused = true
			}
		}
		if paused {
			return m.refreshParentStatus(planID)
		}
		return errTaskNotFound
	}

	active, err := m.getActive(planID)
	if err != nil {
		return err
	}
	if active.Cmd == nil || active.Cmd.Process == nil {
		return errTaskNotFound
	}
	return store.updateTaskRecordStatus(planID, TaskRecordPaused)
}

func (m *RuntimeManager) Resume(planID string) error {
	plan, err := store.getTaskRecordByID(planID)
	if err == nil && plan.Kind == TaskRecordKindGroup {
		children, listErr := store.listTaskChildrenByParent(planID)
		if listErr != nil {
			return listErr
		}
		var resumed bool
		for _, child := range children {
			if childErr := m.Resume(child.ID); childErr == nil {
				resumed = true
			}
		}
		if resumed {
			return m.refreshParentStatus(planID)
		}
		return errTaskNotFound
	}

	active, err := m.getActive(planID)
	if err != nil {
		return err
	}
	if active.Cmd == nil || active.Cmd.Process == nil {
		return errTaskNotFound
	}
	return store.updateTaskRecordStatus(planID, TaskRecordRunning)
}

func (m *RuntimeManager) Cancel(plan *TaskRecord) error {
	if plan.Kind == TaskRecordKindGroup {
		children, err := store.listTaskChildrenByParent(plan.ID)
		if err != nil {
			return err
		}
		var cancelled bool
		for _, child := range children {
			if childErr := m.Cancel(child); childErr == nil {
				cancelled = true
			}
		}
		if !cancelled {
			return store.updateTaskRecordStatus(plan.ID, TaskRecordCancelled)
		}
		return m.refreshParentStatus(plan.ID)
	}

	active, err := m.getActive(plan.ID)
	if err == nil {
		if active.Cmd == nil || active.Cmd.Process == nil {
			return errTaskNotFound
		}
		return store.updateTaskRecordStatus(plan.ID, TaskRecordCancelled)
	}

	if plan.Status == TaskRecordScheduled {
		return store.updateTaskRecordStatus(plan.ID, TaskRecordCancelled)
	}

	if plan.LastRun != nil {
		switch plan.LastRun.Status {
		case TaskPending, TaskRunning, TaskPaused:
			run := *plan.LastRun
			now := time.Now()
			run.Status = TaskCancelled
			run.FinishedAt = &now
			if err := store.finalizeRun(&run); err != nil {
				return err
			}
			return store.updateTaskRecordStatus(plan.ID, TaskRecordCancelled)
		}
	}

	switch plan.Status {
	case TaskRecordRunning, TaskRecordPaused:
		return store.updateTaskRecordStatus(plan.ID, TaskRecordCancelled)
	}

	return err
}

func (m *RuntimeManager) Purge(plan *TaskRecord) error {
	if plan.Kind == TaskRecordKindGroup {
		children, err := store.listTaskChildrenByParent(plan.ID)
		if err != nil {
			return err
		}
		for _, child := range children {
			if _, childErr := m.getActive(child.ID); childErr == nil {
				return errors.New("cannot delete a running task")
			}
		}
	}

	if _, err := m.getActive(plan.ID); err == nil {
		return errors.New("cannot delete a running task")
	}

	runs, err := store.listRunsByTaskRecord(plan.ID)
	if err != nil {
		return err
	}

	paths := collectTaskPaths(runs)
	for _, path := range paths {
		if err := removeTaskPath(path); err != nil {
			return err
		}
	}

	return store.purgeTaskRecord(plan.ID)
}

func (m *RuntimeManager) getActive(planID string) (*ActiveRun, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	active, ok := m.active[planID]
	if !ok {
		return nil, errTaskNotFound
	}
	return active, nil
}

func collectTaskPaths(runs []*TaskRunRecord) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0)
	for _, run := range runs {
		candidates := []string{run.OutputPath, run.ArtifactPath}
		for _, path := range candidates {
			if strings.TrimSpace(path) == "" {
				continue
			}
			clean := filepath.Clean(path)
			if _, exists := seen[clean]; exists {
				continue
			}
			seen[clean] = struct{}{}
			paths = append(paths, clean)
		}
	}
	return paths
}

func removeTaskPath(path string) error {
	if path == "" {
		return nil
	}

	clean := filepath.Clean(path)
	geojsonRoot := filepath.Clean("geojson")
	if clean == geojsonRoot || strings.HasPrefix(clean, geojsonRoot+string(os.PathSeparator)) {
		return errors.New("refusing to delete shared geojson resources")
	}

	if _, err := os.Stat(clean); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	return os.RemoveAll(clean)
}

func buildTaskFromRecord(plan *TaskRecord) (*Task, error) {
	request := CreateTaskRequest{
		Name:         plan.Name,
		SourceName:   plan.SourceName,
		URL:          plan.URL,
		Format:       plan.Format,
		Schema:       plan.Schema,
		Workers:      plan.Workers,
		SavePipe:     plan.SavePipe,
		TimeDelay:    plan.TimeDelay,
		ScheduleMode: plan.ScheduleMode,
		RunAt:        plan.RunAt.Format(time.RFC3339),
	}
	for _, level := range plan.Levels {
		request.Levels = append(request.Levels, LevelRequest(level))
	}
	return buildTaskFromRequest(request)
}

func zipDirectory(sourceDir, zipPath string) error {
	zipFile, err := os.Create(zipPath)
	if err != nil {
		return err
	}
	defer zipFile.Close()

	writer := zip.NewWriter(zipFile)
	defer writer.Close()

	return filepath.Walk(sourceDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}

		relative, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		fileWriter, err := writer.Create(filepath.ToSlash(relative))
		if err != nil {
			return err
		}

		sourceFile, err := os.Open(path)
		if err != nil {
			return err
		}
		_, err = io.Copy(fileWriter, sourceFile)
		closeErr := sourceFile.Close()
		if err != nil {
			return err
		}
		return closeErr
	})
}

func statusToTaskRecordStatus(status TaskStatus) TaskRecordStatus {
	switch status {
	case TaskCompleted:
		return TaskRecordCompleted
	case TaskPaused:
		return TaskRecordPaused
	case TaskCancelled:
		return TaskRecordCancelled
	case TaskFailed:
		return TaskRecordFailed
	default:
		return TaskRecordRunning
	}
}

func (m *RuntimeManager) refreshParentStatus(parentID string) error {
	parent, err := store.getTaskRecordByID(parentID)
	if err != nil {
		return err
	}
	children, err := store.listTaskChildrenByParent(parentID)
	if err != nil {
		return err
	}
	parent.Children = children
	status := aggregateGroupStatus(parent)
	return store.updateTaskRecordStatus(parent.ID, status)
}

func aggregateGroupStatus(plan *TaskRecord) TaskRecordStatus {
	if len(plan.Children) == 0 {
		return plan.Status
	}

	var completed, running, paused, failed, cancelled, scheduled int
	for _, child := range plan.Children {
		status := child.Status
		if child.LastRun != nil {
			status = statusToTaskRecordStatus(child.LastRun.Status)
		}
		switch status {
		case TaskRecordCompleted:
			completed++
		case TaskRecordRunning:
			running++
		case TaskRecordPaused:
			paused++
		case TaskRecordFailed:
			failed++
		case TaskRecordCancelled:
			cancelled++
		default:
			scheduled++
		}
	}

	total := len(plan.Children)
	switch {
	case completed == total:
		return TaskRecordCompleted
	case cancelled == total:
		return TaskRecordCancelled
	case failed == total:
		return TaskRecordFailed
	case running > 0:
		return TaskRecordRunning
	case paused > 0 && running == 0:
		return TaskRecordPaused
	case completed+failed+cancelled == total && failed > 0:
		return TaskRecordPartialFailed
	case scheduled == total:
		return TaskRecordScheduled
	default:
		return TaskRecordRunning
	}
}
