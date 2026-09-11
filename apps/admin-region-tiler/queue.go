package main

import (
	"errors"
	"time"
)

func (m *RuntimeManager) enqueue(plan *TaskRecord, trigger string) error {
	fresh, err := store.getTaskRecordByID(plan.ID)
	if err != nil {
		return err
	}
	plans := []*TaskRecord{fresh}
	if fresh.Kind == TaskRecordKindGroup {
		plans, err = store.listTaskChildrenByParent(plan.ID)
		if err != nil {
			return err
		}
	}
	budgetPlan, sourceCount := fresh, len(plans)
	if fresh.ParentID != "" {
		budgetPlan, err = store.getTaskRecordByID(fresh.ParentID)
		if err != nil {
			return err
		}
		siblings, err := store.listTaskChildrenByParent(fresh.ParentID)
		if err != nil {
			return err
		}
		sourceCount = len(siblings)
	}
	if _, err := validateTaskBudget(budgetPlan.Levels, sourceCount); err != nil {
		return err
	}
	eligible := []*TaskRecord{}
	for _, p := range plans {
		if err := store.checkNotDeleting(p.ID); err != nil {
			return err
		}
		if _, err := m.getActive(p.ID); err == nil {
			return errTaskAlreadyActive
		}
		if trigger == triggerRetryFailures {
			summary, err := store.failureSummary(p.ID)
			if err != nil {
				return err
			}
			if summary.Retryable == 0 {
				continue
			}
			if _, err := store.validateRetryBaseline(p); err != nil {
				return err
			}
		}
		eligible = append(eligible, p)
	}
	if len(eligible) == 0 {
		return errNoRetryableFailures
	}
	tx, err := store.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, p := range eligible {
		due := p.RunAt.UnixMilli()
		if trigger == triggerRetryFailures {
			due = time.Now().UnixMilli()
		}
		if _, err := tx.Exec(`INSERT OR IGNORE INTO execution_queue(plan_id,trigger_mode,due_at,enqueued_at,state) VALUES(?,?,?,?,'queued')`, p.ID, trigger, due, time.Now().UnixMilli()); err != nil {
			return err
		}
		if _, err := tx.Exec(`UPDATE plans SET status='queued' WHERE id=?`, p.ID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE plans SET status='queued' WHERE id=?`, fresh.ID); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return m.dispatchQueued()
}

func (m *RuntimeManager) dispatchQueued() error {
	for {
		m.mu.RLock()
		count := len(m.active)
		m.mu.RUnlock()
		if count >= maxActiveTasks() {
			return nil
		}
		var id, trigger string
		err := store.db.QueryRow(`SELECT plan_id,trigger_mode FROM execution_queue WHERE state='queued' AND due_at<=? ORDER BY due_at,enqueued_at,plan_id LIMIT 1`, time.Now().UnixMilli()).Scan(&id, &trigger)
		if err != nil {
			return nil
		}
		plan, err := store.getTaskRecordByID(id)
		if err == nil {
			_, err = store.db.Exec(`UPDATE execution_queue SET state='running' WHERE plan_id=?`, id)
		}
		if err == nil {
			err = m.startTaskRecordWithTrigger(plan, trigger)
		}
		if err != nil {
			if errors.Is(err, errTaskAlreadyActive) {
				return nil
			}
			_ = store.updateTaskRecordStatus(id, TaskRecordFailed)
			_, _ = store.db.Exec(`DELETE FROM execution_queue WHERE plan_id=?`, id)
			continue
		}
	}
}

type QueueState struct {
	State    string `json:"state,omitempty"`
	DueAt    int64  `json:"dueAt,omitempty"`
	Position int    `json:"position,omitempty"`
}

func (s *SQLiteStore) queueState(id string) QueueState {
	q := QueueState{}
	_ = s.db.QueryRow(`SELECT state,due_at FROM execution_queue WHERE plan_id=?`, id).Scan(&q.State, &q.DueAt)
	if q.State == "queued" {
		_ = s.db.QueryRow(`SELECT COUNT(*) FROM execution_queue WHERE state='queued' AND (due_at,enqueued_at,plan_id)<=(SELECT due_at,enqueued_at,plan_id FROM execution_queue WHERE plan_id=?)`, id).Scan(&q.Position)
	}
	return q
}
