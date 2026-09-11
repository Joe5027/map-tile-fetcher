package main

import (
	"fmt"
	"github.com/gin-gonic/gin"
	"net/http"
)

func recreateTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(404, gin.H{"error": "task not found"})
		return
	}
	name := []rune(plan.Name)
	if len(name) > 70 {
		name = name[:70]
	}
	req := CreateTaskRequest{Name: string(name) + " (重新创建)", URL: plan.URL, Format: plan.Format, Schema: plan.Schema, Workers: plan.Workers, SavePipe: plan.SavePipe, TimeDelay: plan.TimeDelay}
	for _, level := range plan.Levels {
		req.Levels = append(req.Levels, LevelRequest(level))
		req.Output.Format = level.OutputFormat
	}
	if plan.Kind == TaskRecordKindGroup {
		children, err := store.listTaskChildrenByParent(plan.ID)
		if err != nil {
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
		for _, child := range children {
			req.Sources = append(req.Sources, SourceRequest{Name: child.SourceName, URL: child.URL, Format: child.Format, Schema: child.Schema})
		}
	}
	createTaskFromRequest(c, req)
}

func reconcileTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	m := runtimeManager
	m.operations.Lock()
	plans := []*TaskRecord{plan}
	if plan.Kind == TaskRecordKindGroup {
		plans, err = store.listTaskChildrenByParent(plan.ID)
		if err != nil {
			m.operations.Unlock()
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	for _, p := range plans {
		if _, err := m.getActive(p.ID); err == nil || store.integrityState(p.ID).Status == "checking" {
			m.operations.Unlock()
			c.JSON(409, gin.H{"error": "task is active or already checking"})
			return
		}
		if err := store.checkNotDeleting(p.ID); err != nil {
			m.operations.Unlock()
			c.JSON(409, gin.H{"error": err.Error()})
			return
		}
	}
	for _, p := range plans {
		if err := saveIntegrity(store.db, p.ID, IntegrityState{Status: "checking"}); err != nil {
			m.operations.Unlock()
			c.JSON(500, gin.H{"error": err.Error()})
			return
		}
	}
	m.mu.Lock()
	for _, p := range plans {
		m.active[p.ID] = &ActiveRun{Plan: p}
	}
	m.mu.Unlock()
	m.operations.Unlock()
	go func() {
		for _, p := range plans {
			state, err := store.reconcileRecord(p)
			if err != nil {
				state.Status = "incomplete"
				state.Error = err.Error()
			}
			_ = saveIntegrity(store.db, p.ID, state)
			m.mu.Lock()
			delete(m.active, p.ID)
			m.mu.Unlock()
		}
	}()
	c.JSON(http.StatusAccepted, gin.H{"status": "checking"})
}

func (s *SQLiteStore) reconcileRecord(plan *TaskRecord) (IntegrityState, error) {
	state := IntegrityState{Status: "incomplete"}
	run, err := s.publishedRun(plan.ID)
	if err != nil {
		return state, fmt.Errorf("no published artifact: %w", err)
	}
	task, err := buildTaskFromRecord(plan)
	if err != nil {
		return state, err
	}
	state, err = s.inspectOutput(plan, task, run, true)
	if err != nil {
		return state, err
	}
	// This uses the existing published run; no worker or tile request is started.
	if state.Status == "complete" {
		run.Status = TaskCompleted
	} else {
		run.Status = TaskPartialFailed
	}
	if err := s.commitPublication(run, state); err != nil {
		return state, err
	}
	return s.integrityState(plan.ID), nil
}
