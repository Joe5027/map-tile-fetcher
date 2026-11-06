package main

import (
	"database/sql"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Protected by artifactDownloadGrants.Mutex, including cache-hit requests.
var artifactDownloadGrantLastSweep time.Time

// responseView is populated once for a page, so rendering does not issue a query
// for each task, child, or artifact. The legacy detail endpoint keeps its path.
type taskResponseView struct {
	Integrity IntegrityState
	Queue     QueueState
	Published *TaskRunRecord
	Failures  FailureSummary
	Phase     RunPhase
}

type taskPageResponse struct {
	Items      []TaskResponse `json:"items"`
	Total      int            `json:"total"`
	Counts     map[string]int `json:"counts"`
	queryCount int
}

func listTaskPage(c *gin.Context) {
	limit, err := strconv.Atoi(c.DefaultQuery("limit", "50"))
	if err != nil || limit < 1 || limit > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "limit must be between 1 and 100"})
		return
	}
	offset, err := strconv.Atoi(c.DefaultQuery("offset", "0"))
	if err != nil || offset < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "offset must be non-negative"})
		return
	}
	status := c.DefaultQuery("status", "all")
	switch status {
	case "all", "scheduled", "pending", "queued", "running", "paused", "completed", "failed", "partial_failed", "cancelled":
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "unknown task status"})
		return
	}
	page, err := store.loadTaskPage(currentUser(c).ID, limit, offset, status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}
	c.JSON(http.StatusOK, page)
}

type taskPageLoader struct {
	store      *SQLiteStore
	queryCount int
}

func (l *taskPageLoader) each(query string, args []any, visit func(*sql.Rows) error) error {
	l.queryCount++
	rows, err := l.store.db.Query(query, args...)
	if err != nil {
		return fmt.Errorf("task page query %d: %w", l.queryCount, err)
	}
	defer rows.Close()
	for rows.Next() {
		if err := visit(rows); err != nil {
			return err
		}
	}
	return rows.Err()
}

func taskPageStatusMatches(status, filter string) bool {
	switch filter {
	case "all":
		return true
	case "scheduled":
		return status == "scheduled" || status == "pending" || status == "queued"
	case "failed":
		return status == "failed" || status == "partial_failed"
	default:
		return status == filter
	}
}

const pagePlanColumns = `id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay, schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at`
const pageRunColumns = `id, plan_id, user_id, status, trigger_mode, output_path, artifact_path, artifact_name, artifact_status, total, current, success_count, failure_count, error_message, started_at, finished_at, created_at, updated_at`

func (s *SQLiteStore) loadTaskPage(userID int64, limit, offset int, filter string) (*taskPageResponse, error) {
	loader := &taskPageLoader{store: s}
	page := &taskPageResponse{Items: []TaskResponse{}, Counts: map[string]int{"all": 0}}
	// Read only status/identity for global counts. Reuse the actual aggregation
	// functions instead of maintaining a second SQL version of task semantics.
	var parents []*TaskRecord
	all := map[string]*TaskRecord{}
	var children []*TaskRecord
	err := loader.each(`SELECT p.id,p.parent_id,p.kind,p.status,r.status
		FROM plans p LEFT JOIN task_runs r ON r.id=p.last_run_id
		WHERE p.user_id=? ORDER BY p.updated_at DESC,p.id DESC`, []any{userID}, func(rows *sql.Rows) error {
		p := &TaskRecord{}
		var runStatus sql.NullString
		if err := rows.Scan(&p.ID, &p.ParentID, &p.Kind, &p.Status, &runStatus); err != nil {
			return err
		}
		if runStatus.Valid {
			p.LastRun = &TaskRunRecord{Status: TaskStatus(runStatus.String)}
		}
		all[p.ID] = p
		if p.ParentID == "" {
			parents = append(parents, p)
		} else {
			children = append(children, p)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, child := range children {
		if parent := all[child.ParentID]; parent != nil {
			parent.Children = append(parent.Children, child)
		}
	}
	var selectedIDs []string
	for _, parent := range parents {
		status := string(effectiveTaskRecordStatusForResponse(parent, parent.LastRun))
		if parent.Kind == TaskRecordKindGroup {
			status = string(aggregateGroupStatus(parent))
		}
		page.Counts[status]++
		page.Counts["all"]++
		if taskPageStatusMatches(status, filter) {
			if page.Total >= offset && len(selectedIDs) < limit {
				selectedIDs = append(selectedIDs, parent.ID)
			}
			page.Total++
		}
	}
	if len(selectedIDs) == 0 {
		page.queryCount = loader.queryCount
		return page, nil
	}
	marks := strings.TrimSuffix(strings.Repeat("?,", len(selectedIDs)), ",")
	args := []any{userID}
	for i := 0; i < 2; i++ {
		for _, id := range selectedIDs {
			args = append(args, id)
		}
	}
	selected := `WITH selected AS (SELECT rowid AS source_order,* FROM plans WHERE user_id=? AND (id IN (` + marks + `) OR parent_id IN (` + marks + `))) `
	records := map[string]*TaskRecord{}
	var orderedRecords []*TaskRecord
	err = loader.each(selected+`SELECT `+pagePlanColumns+` FROM selected ORDER BY created_at,source_order`, args, func(rows *sql.Rows) error {
		p, err := scanTaskRecord(rows)
		if err != nil {
			return err
		}
		p.responseView = &taskResponseView{Integrity: IntegrityState{Status: "unchecked"}}
		records[p.ID] = p
		orderedRecords = append(orderedRecords, p)
		return nil
	})
	if err != nil {
		return nil, err
	}
	// Preserve source creation order without an extra child query for each parent.
	for _, child := range orderedRecords {
		if parent := records[child.ParentID]; parent != nil {
			parent.Children = append(parent.Children, child)
		}
	}
	qualifiedRuns := "r." + strings.ReplaceAll(pageRunColumns, ", ", ", r.")
	err = loader.each(selected+`SELECT `+qualifiedRuns+` FROM task_runs r JOIN selected p ON p.last_run_id=r.id`, args, func(rows *sql.Rows) error {
		run, err := scanRun(rows)
		if err != nil {
			return err
		}
		if p := records[run.TaskRecordID]; p != nil {
			p.LastRun = run
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = loader.each(selected+`, published AS (SELECT r.*,ROW_NUMBER() OVER (PARTITION BY r.plan_id ORDER BY r.created_at DESC,r.rowid DESC) AS position
		FROM task_runs r JOIN selected p ON p.id=r.plan_id WHERE r.artifact_status='ready' AND r.artifact_path<>'')
		SELECT `+pageRunColumns+` FROM published WHERE position=1`, args, func(rows *sql.Rows) error {
		run, err := scanRun(rows)
		if err != nil {
			return err
		}
		if p := records[run.TaskRecordID]; p != nil {
			p.responseView.Published = run
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = loader.each(selected+`SELECT i.plan_id,i.status,i.expected,i.available,i.missing,i.error_message,i.run_id
		FROM task_integrity i JOIN selected p ON p.id=i.plan_id`, args, func(rows *sql.Rows) error {
		var id string
		var state IntegrityState
		if err := rows.Scan(&id, &state.Status, &state.Expected, &state.Available, &state.Missing, &state.Error, &state.RunID); err != nil {
			return err
		}
		records[id].responseView.Integrity = state
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = loader.each(selected+`, ranked_queue AS (SELECT plan_id,state,due_at,
		CASE WHEN state='queued' THEN ROW_NUMBER() OVER (PARTITION BY state ORDER BY due_at,enqueued_at,plan_id) ELSE 0 END AS position FROM execution_queue)
		SELECT q.plan_id,q.state,q.due_at,q.position FROM ranked_queue q JOIN selected p ON p.id=q.plan_id`, args, func(rows *sql.Rows) error {
		var id string
		var state QueueState
		if err := rows.Scan(&id, &state.State, &state.DueAt, &state.Position); err != nil {
			return err
		}
		records[id].responseView.Queue = state
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = loader.each(selected+`, unresolved AS (SELECT f.task_id,f.source_id,f.z,f.x,f.y,f.url,MAX(f.retryable) AS retryable
		FROM failures f JOIN selected p ON p.id=f.task_id WHERE f.resolved_at=0 GROUP BY f.task_id,f.source_id,f.z,f.x,f.y,f.url)
		SELECT task_id,source_id,COUNT(*),COALESCE(SUM(retryable),0) FROM unresolved GROUP BY task_id,source_id`, args, func(rows *sql.Rows) error {
		var taskID, sourceID string
		var summary FailureSummary
		if err := rows.Scan(&taskID, &sourceID, &summary.Total, &summary.Retryable); err != nil {
			return err
		}
		if parent := records[taskID]; parent != nil {
			parent.responseView.Failures.Total += summary.Total
			parent.responseView.Failures.Retryable += summary.Retryable
		}
		if child := records[sourceID]; child != nil && child.ParentID == taskID {
			child.responseView.Failures = summary
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	err = loader.each(selected+`SELECT p.id,f.phase,f.current,f.total FROM run_phases f JOIN selected p ON p.last_run_id=f.run_id`, args, func(rows *sql.Rows) error {
		var id string
		var phase RunPhase
		if err := rows.Scan(&id, &phase.Name, &phase.Current, &phase.Total); err != nil {
			return err
		}
		records[id].responseView.Phase = phase
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, id := range selectedIDs {
		if p := records[id]; p != nil {
			page.Items = append(page.Items, taskResponseFromRecord(p))
		}
	}
	page.queryCount = loader.queryCount
	return page, nil
}

func applyRecordFailureSummary(plan *TaskRecord, response *TaskResponse) {
	if plan.responseView == nil {
		applyFailureSummary(plan.ID, response)
		return
	}
	summary := plan.responseView.Failures
	response.RetryableFailureCount = summary.Retryable
	response.CanRetryFailures = summary.Retryable > 0 && canRetryFailureStatus(TaskRecordStatus(response.Status))
	response.FailureSummary = TaskFailureSummaryResponse{FailureCount: summary.Total, RetryableFailureCount: summary.Retryable, CanRetryFailures: response.CanRetryFailures}
}
