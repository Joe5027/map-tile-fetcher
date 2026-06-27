package main

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
)

var (
	errTaskNotFound = errors.New("task not found")
	errUserNotFound = errors.New("user not found")
)

type PlanStatus string

const (
	PlanScheduled     PlanStatus = "scheduled"
	PlanRunning       PlanStatus = "running"
	PlanPaused        PlanStatus = "paused"
	PlanCompleted     PlanStatus = "completed"
	PlanPartialFailed PlanStatus = "partial_failed"
	PlanFailed        PlanStatus = "failed"
	PlanCancelled     PlanStatus = "cancelled"
)

type PlanKind string

const (
	PlanKindSingle PlanKind = "single"
	PlanKindGroup  PlanKind = "group"
	PlanKindChild  PlanKind = "child"
)

type ScheduleMode string

const (
	ScheduleImmediate ScheduleMode = "immediate"
	ScheduleOnce      ScheduleMode = "once"
)

type ArtifactStatus string

const (
	ArtifactNone    ArtifactStatus = "none"
	ArtifactPacking ArtifactStatus = "packing"
	ArtifactReady   ArtifactStatus = "ready"
	ArtifactFailed  ArtifactStatus = "failed"
)

type UserRecord struct {
	ID           int64
	Username     string
	PasswordHash string
	CreatedAt    time.Time
}

type SessionRecord struct {
	Token     string
	UserID    int64
	ExpiresAt time.Time
	CreatedAt time.Time
}

type LevelConfig struct {
	MinZoom int    `json:"minZoom"`
	MaxZoom int    `json:"maxZoom"`
	Geojson string `json:"geojson"`
	URL     string `json:"url,omitempty"`
}

type PlanRecord struct {
	ID           string
	UserID       int64
	ParentID     string
	Kind         PlanKind
	Name         string
	SourceName   string
	URL          string
	Format       string
	Schema       string
	Workers      int
	SavePipe     int
	TimeDelay    int
	ScheduleMode ScheduleMode
	RunAt        time.Time
	Status       PlanStatus
	Levels       []LevelConfig
	LastRunID    string
	CreatedAt    time.Time
	UpdatedAt    time.Time
	LastRun      *TaskRunRecord
	Children     []*PlanRecord
}

type TaskRunRecord struct {
	ID             string
	PlanID         string
	UserID         int64
	Status         TaskStatus
	TriggerMode    string
	OutputPath     string
	ArtifactPath   string
	ArtifactName   string
	ArtifactStatus ArtifactStatus
	Total          int64
	Current        int64
	SuccessCount   int64
	FailureCount   int64
	ErrorMessage   string
	StartedAt      *time.Time
	FinishedAt     *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type FailureRecord struct {
	TaskID       string
	RunID        string
	SourceID     string
	Z            int
	X            int
	Y            int
	URL          string
	ErrorMessage string
	Retryable    bool
	Attempt      int
	CreatedAt    time.Time
}

type SQLiteStore struct {
	db *sql.DB
}

var (
	store          *SQLiteStore
	sessionMaxAge  = 7 * 24 * time.Hour
	sessionCookie  = "tiler_session"
	defaultDataDir = "data"
)

func initDB() {
	if err := os.MkdirAll(defaultDataDir, os.ModePerm); err != nil {
		log.Fatalf("failed to create data directory: %v", err)
	}

	dbPath := filepath.Join(defaultDataDir, viper.GetString("app.database"))
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		log.Fatalf("failed to open database: %v", err)
	}

	db.SetMaxOpenConns(8)
	db.SetMaxIdleConns(4)

	store = &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		log.Fatalf("failed to initialize schema: %v", err)
	}
	if err := store.seedDefaultUser(); err != nil {
		log.Fatalf("failed to seed default user: %v", err)
	}
	if err := store.recoverInterruptedPlans(); err != nil {
		log.Fatalf("failed to recover interrupted plans: %v", err)
	}
}

func (s *SQLiteStore) initSchema() error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA busy_timeout = 5000;`,
		`CREATE TABLE IF NOT EXISTS users (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			username TEXT NOT NULL UNIQUE,
			password_hash TEXT NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS sessions (
			token TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			created_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS plans (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			parent_id TEXT NOT NULL DEFAULT '',
			kind TEXT NOT NULL DEFAULT 'single',
			name TEXT NOT NULL,
			source_name TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL,
			format TEXT NOT NULL,
			schema TEXT NOT NULL,
			workers INTEGER NOT NULL,
			save_pipe INTEGER NOT NULL,
			time_delay INTEGER NOT NULL,
			schedule_mode TEXT NOT NULL,
			run_at INTEGER NOT NULL,
			status TEXT NOT NULL,
			levels_json TEXT NOT NULL,
			last_run_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_runs (
			id TEXT PRIMARY KEY,
			plan_id TEXT NOT NULL,
			task_id TEXT NOT NULL DEFAULT '',
			user_id INTEGER NOT NULL,
			status TEXT NOT NULL,
			trigger_mode TEXT NOT NULL,
			output_path TEXT NOT NULL DEFAULT '',
			artifact_path TEXT NOT NULL DEFAULT '',
			artifact_name TEXT NOT NULL DEFAULT '',
			artifact_status TEXT NOT NULL DEFAULT 'none',
			total INTEGER NOT NULL DEFAULT 0,
			current INTEGER NOT NULL DEFAULT 0,
			success_count INTEGER NOT NULL DEFAULT 0,
			failure_count INTEGER NOT NULL DEFAULT 0,
			error_message TEXT NOT NULL DEFAULT '',
			started_at INTEGER NOT NULL DEFAULT 0,
			finished_at INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS tasks (
			id TEXT PRIMARY KEY,
			user_id INTEGER NOT NULL,
			parent_id TEXT NOT NULL DEFAULT '',
			mode TEXT NOT NULL,
			name TEXT NOT NULL,
			area_json TEXT NOT NULL,
			zoom_min INTEGER NOT NULL DEFAULT 0,
			zoom_max INTEGER NOT NULL DEFAULT 0,
			schedule_mode TEXT NOT NULL,
			run_at INTEGER NOT NULL,
			status TEXT NOT NULL,
			legacy_plan_id TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS task_sources (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			name TEXT NOT NULL,
			layer TEXT NOT NULL DEFAULT '',
			url TEXT NOT NULL,
			format TEXT NOT NULL,
			schema TEXT NOT NULL,
			position INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS artifacts (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			name TEXT NOT NULL,
			path TEXT NOT NULL,
			format TEXT NOT NULL DEFAULT '',
			package_format TEXT NOT NULL DEFAULT '',
			status TEXT NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS failures (
			id TEXT PRIMARY KEY,
			task_id TEXT NOT NULL,
			run_id TEXT NOT NULL DEFAULT '',
			source_id TEXT NOT NULL DEFAULT '',
			z INTEGER NOT NULL DEFAULT 0,
			x INTEGER NOT NULL DEFAULT 0,
			y INTEGER NOT NULL DEFAULT 0,
			url TEXT NOT NULL DEFAULT '',
			error_message TEXT NOT NULL DEFAULT '',
			retryable INTEGER NOT NULL DEFAULT 0,
			attempt INTEGER NOT NULL DEFAULT 0,
			created_at INTEGER NOT NULL
		);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_expires_at ON sessions(expires_at);`,
		`CREATE INDEX IF NOT EXISTS idx_plans_user_id ON plans(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_plans_status_run_at ON plans(status, run_at);`,
		`CREATE INDEX IF NOT EXISTS idx_task_runs_plan_id ON task_runs(plan_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_user_id ON tasks(user_id);`,
		`CREATE INDEX IF NOT EXISTS idx_tasks_status_run_at ON tasks(status, run_at);`,
		`CREATE INDEX IF NOT EXISTS idx_task_sources_task_id ON task_sources(task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_artifacts_task_id ON artifacts(task_id);`,
		`CREATE INDEX IF NOT EXISTS idx_failures_task_run ON failures(task_id, run_id);`,
	}

	for _, statement := range statements {
		if _, err := s.db.Exec(statement); err != nil {
			return err
		}
	}
	if err := s.ensurePlanColumns(); err != nil {
		return err
	}
	if err := s.ensureTaskRunColumns(); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_plans_parent_id ON plans(parent_id);`); err != nil {
		return err
	}
	if _, err := s.db.Exec(`CREATE INDEX IF NOT EXISTS idx_task_runs_task_id ON task_runs(task_id);`); err != nil {
		return err
	}
	return s.backfillNormalizedRecords()
}

func (s *SQLiteStore) ensurePlanColumns() error {
	columns, err := s.planColumnSet()
	if err != nil {
		return err
	}

	required := []struct {
		name       string
		definition string
	}{
		{name: "parent_id", definition: `ALTER TABLE plans ADD COLUMN parent_id TEXT NOT NULL DEFAULT ''`},
		{name: "kind", definition: `ALTER TABLE plans ADD COLUMN kind TEXT NOT NULL DEFAULT 'single'`},
		{name: "source_name", definition: `ALTER TABLE plans ADD COLUMN source_name TEXT NOT NULL DEFAULT ''`},
	}

	for _, item := range required {
		if columns[item.name] {
			continue
		}
		if _, err := s.db.Exec(item.definition); err != nil {
			return err
		}
	}
	return nil
}

func (s *SQLiteStore) planColumnSet() (map[string]bool, error) {
	return s.tableColumnSet("plans")
}

func (s *SQLiteStore) ensureTaskRunColumns() error {
	columns, err := s.tableColumnSet("task_runs")
	if err != nil {
		return err
	}
	if columns["task_id"] {
		return nil
	}
	_, err = s.db.Exec(`ALTER TABLE task_runs ADD COLUMN task_id TEXT NOT NULL DEFAULT ''`)
	return err
}

func (s *SQLiteStore) tableColumnSet(table string) (map[string]bool, error) {
	var statement string
	switch table {
	case "plans":
		statement = `PRAGMA table_info(plans)`
	case "task_runs":
		statement = `PRAGMA table_info(task_runs)`
	default:
		return nil, fmt.Errorf("unsupported table for column introspection: %s", table)
	}

	rows, err := s.db.Query(statement)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns := make(map[string]bool)
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		columns[name] = true
	}
	return columns, rows.Err()
}

func (s *SQLiteStore) backfillNormalizedRecords() error {
	rows, err := s.db.Query(
		`SELECT id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
		        schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		   FROM plans
		  ORDER BY CASE kind WHEN 'child' THEN 1 ELSE 0 END, created_at ASC`,
	)
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return err
		}
		if err := s.syncNormalizedPlan(plan); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	runRows, err := s.db.Query(`SELECT id, plan_id FROM task_runs WHERE task_id = ''`)
	if err != nil {
		return err
	}
	defer runRows.Close()

	for runRows.Next() {
		var runID string
		var planID string
		if err := runRows.Scan(&runID, &planID); err != nil {
			return err
		}
		if err := s.syncRunTaskID(runID, planID); err != nil {
			return err
		}
	}
	return runRows.Err()
}

func (s *SQLiteStore) syncNormalizedPlan(plan *PlanRecord) error {
	if plan == nil {
		return nil
	}
	switch plan.Kind {
	case PlanKindChild:
		return s.upsertNormalizedTaskSource(plan.ParentID, plan.ID, plan.SourceName, plan.URL, plan.Format, plan.Schema, 0, plan.CreatedAt, plan.UpdatedAt)
	case PlanKindSingle:
		if err := s.upsertNormalizedTask(plan); err != nil {
			return err
		}
		return s.upsertNormalizedTaskSource(plan.ID, plan.ID+":source", plan.SourceName, plan.URL, plan.Format, plan.Schema, 0, plan.CreatedAt, plan.UpdatedAt)
	default:
		return s.upsertNormalizedTask(plan)
	}
}

func (s *SQLiteStore) upsertNormalizedTask(plan *PlanRecord) error {
	areaJSON, err := json.Marshal(map[string]any{
		"levels": plan.Levels,
	})
	if err != nil {
		return err
	}
	zoomMin, zoomMax := zoomRangeFromLevels(plan.Levels)
	createdAt := plan.CreatedAt
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	updatedAt := plan.UpdatedAt
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}

	_, err = s.db.Exec(
		`INSERT INTO tasks (
			id, user_id, parent_id, mode, name, area_json, zoom_min, zoom_max, schedule_mode,
			run_at, status, legacy_plan_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			user_id = excluded.user_id,
			parent_id = excluded.parent_id,
			mode = excluded.mode,
			name = excluded.name,
			area_json = excluded.area_json,
			zoom_min = excluded.zoom_min,
			zoom_max = excluded.zoom_max,
			schedule_mode = excluded.schedule_mode,
			run_at = excluded.run_at,
			status = excluded.status,
			legacy_plan_id = excluded.legacy_plan_id,
			updated_at = excluded.updated_at`,
		plan.ID,
		plan.UserID,
		plan.ParentID,
		taskModeFromLevels(plan.Levels),
		plan.Name,
		string(areaJSON),
		zoomMin,
		zoomMax,
		string(plan.ScheduleMode),
		plan.RunAt.Unix(),
		string(plan.Status),
		plan.ID,
		createdAt.Unix(),
		updatedAt.Unix(),
	)
	return err
}

func (s *SQLiteStore) upsertNormalizedTaskSource(taskID, sourceID, name, rawURL, format, schema string, position int, createdAt, updatedAt time.Time) error {
	if strings.TrimSpace(taskID) == "" {
		return nil
	}
	if createdAt.IsZero() {
		createdAt = time.Now()
	}
	if updatedAt.IsZero() {
		updatedAt = createdAt
	}
	if strings.TrimSpace(name) == "" {
		name = rawURL
	}

	_, err := s.db.Exec(
		`INSERT INTO task_sources (
			id, task_id, name, layer, url, format, schema, position, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			task_id = excluded.task_id,
			name = excluded.name,
			layer = excluded.layer,
			url = excluded.url,
			format = excluded.format,
			schema = excluded.schema,
			position = excluded.position,
			updated_at = excluded.updated_at`,
		sourceID,
		taskID,
		name,
		inferLayerName(name, rawURL),
		rawURL,
		format,
		schema,
		position,
		createdAt.Unix(),
		updatedAt.Unix(),
	)
	return err
}

func (s *SQLiteStore) syncRunTaskID(runID, planID string) error {
	taskID, err := s.taskIDForPlan(planID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE task_runs SET task_id = ? WHERE id = ?`, taskID, runID)
	return err
}

func (s *SQLiteStore) syncTaskStatus(planID string, status PlanStatus) error {
	taskID, err := s.taskIDForPlan(planID)
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`UPDATE tasks SET status = ?, updated_at = ? WHERE id = ?`, string(status), time.Now().Unix(), taskID)
	return err
}

func (s *SQLiteStore) upsertArtifactFromRun(run *TaskRunRecord) error {
	if run == nil {
		return nil
	}
	if strings.TrimSpace(run.ArtifactPath) == "" && run.ArtifactStatus == ArtifactNone {
		return nil
	}
	taskID, err := s.taskIDForPlan(run.PlanID)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	name := strings.TrimSpace(run.ArtifactName)
	if name == "" {
		name = filepath.Base(run.ArtifactPath)
	}

	_, err = s.db.Exec(
		`INSERT INTO artifacts (
			id, task_id, run_id, name, path, format, package_format, status, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			task_id = excluded.task_id,
			run_id = excluded.run_id,
			name = excluded.name,
			path = excluded.path,
			format = excluded.format,
			package_format = excluded.package_format,
			status = excluded.status,
			updated_at = excluded.updated_at`,
		run.ID+":artifact",
		taskID,
		run.ID,
		name,
		run.ArtifactPath,
		artifactFormatFromPath(run.ArtifactPath),
		artifactPackageFromPath(run.ArtifactPath),
		string(run.ArtifactStatus),
		now,
		now,
	)
	return err
}

func (s *SQLiteStore) replaceFailureRecords(run *TaskRunRecord, records []TileFailureRecord) error {
	if run == nil {
		return nil
	}
	taskID, err := s.taskIDForPlan(run.PlanID)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM failures WHERE run_id = ?`, run.ID); err != nil {
		return err
	}

	for index, record := range records {
		createdAt := record.CreatedAt
		if createdAt.IsZero() {
			createdAt = time.Now()
		}
		retryable := 0
		if record.Retryable {
			retryable = 1
		}
		if _, err := tx.Exec(
			`INSERT INTO failures (
				id, task_id, run_id, source_id, z, x, y, url, error_message, retryable, attempt, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			fmt.Sprintf("%s:%d", run.ID, index),
			taskID,
			run.ID,
			"",
			record.Z,
			record.X,
			record.Y,
			record.URL,
			record.ErrorMessage,
			retryable,
			record.Attempt,
			createdAt.Unix(),
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *SQLiteStore) listFailureRecords(planID string) ([]FailureRecord, error) {
	taskID, err := s.taskIDForPlan(planID)
	if err != nil {
		return nil, err
	}
	rows, err := s.db.Query(
		`SELECT task_id, run_id, source_id, z, x, y, url, error_message, retryable, attempt, created_at
		   FROM failures
		  WHERE task_id = ?
		  ORDER BY created_at DESC, id DESC
		  LIMIT 1000`,
		taskID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	records := make([]FailureRecord, 0)
	for rows.Next() {
		var record FailureRecord
		var retryable int
		var createdAt int64
		if err := rows.Scan(
			&record.TaskID,
			&record.RunID,
			&record.SourceID,
			&record.Z,
			&record.X,
			&record.Y,
			&record.URL,
			&record.ErrorMessage,
			&retryable,
			&record.Attempt,
			&createdAt,
		); err != nil {
			return nil, err
		}
		record.Retryable = retryable == 1
		record.CreatedAt = time.Unix(createdAt, 0)
		records = append(records, record)
	}
	return records, rows.Err()
}

func (s *SQLiteStore) taskIDForPlan(planID string) (string, error) {
	var id string
	var parentID string
	err := s.db.QueryRow(`SELECT id, parent_id FROM plans WHERE id = ?`, planID).Scan(&id, &parentID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return planID, nil
		}
		return "", err
	}
	if strings.TrimSpace(parentID) != "" {
		return parentID, nil
	}
	return id, nil
}

func taskModeFromLevels(levels []LevelConfig) string {
	for _, level := range levels {
		path := filepath.ToSlash(level.Geojson)
		if strings.Contains(path, "data/generated-areas/") {
			return "bbox"
		}
	}
	return "region"
}

func zoomRangeFromLevels(levels []LevelConfig) (int, int) {
	if len(levels) == 0 {
		return 0, 0
	}
	minZoom := levels[0].MinZoom
	maxZoom := levels[0].MaxZoom
	for _, level := range levels[1:] {
		if level.MinZoom < minZoom {
			minZoom = level.MinZoom
		}
		if level.MaxZoom > maxZoom {
			maxZoom = level.MaxZoom
		}
	}
	return minZoom, maxZoom
}

func inferLayerName(name, rawURL string) string {
	lowered := strings.ToLower(name + " " + rawURL)
	for _, layer := range []string{"img", "cia", "vec"} {
		if strings.Contains(lowered, layer) {
			return layer
		}
	}
	return ""
}

func artifactFormatFromPath(path string) string {
	ext := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
	if ext == "mbtiles" {
		return "mbtiles"
	}
	return ext
}

func artifactPackageFromPath(path string) string {
	lowered := strings.ToLower(path)
	switch {
	case strings.HasSuffix(lowered, ".zip"):
		return "zip"
	case strings.HasSuffix(lowered, ".tar.gz") || strings.HasSuffix(lowered, ".tgz"):
		return "tar.gz"
	default:
		return ""
	}
}

func (s *SQLiteStore) seedDefaultUser() error {
	username := viper.GetString("auth.default_username")
	password := viper.GetString("auth.default_password")

	var existing int64
	err := s.db.QueryRow(`SELECT COUNT(1) FROM users WHERE username = ?`, username).Scan(&existing)
	if err != nil {
		return err
	}
	if existing > 0 {
		return nil
	}

	_, err = s.db.Exec(
		`INSERT INTO users (username, password_hash, created_at) VALUES (?, ?, ?)`,
		username,
		hashPassword(password),
		time.Now().Unix(),
	)
	return err
}

func (s *SQLiteStore) recoverInterruptedPlans() error {
	now := time.Now().Unix()
	if _, err := s.db.Exec(
		`UPDATE task_runs
		 SET status = ?, error_message = ?, finished_at = ?, updated_at = ?
		 WHERE status = ?`,
		string(TaskFailed),
		"service restarted before task completed",
		now,
		now,
		string(TaskRunning),
	); err != nil {
		return err
	}

	_, err := s.db.Exec(
		`UPDATE plans
		 SET status = ?, updated_at = ?
		 WHERE status = ?`,
		string(PlanScheduled),
		now,
		string(PlanRunning),
	)
	return err
}

func (s *SQLiteStore) authenticateUser(username, password string) (*UserRecord, error) {
	row := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE username = ?`, username)
	user, err := scanUser(row)
	if err != nil {
		return nil, err
	}
	if user.PasswordHash != hashPassword(password) {
		return nil, errors.New("invalid credentials")
	}
	return user, nil
}

func (s *SQLiteStore) createSession(userID int64) (*SessionRecord, error) {
	token, err := newToken()
	if err != nil {
		return nil, err
	}
	now := time.Now()
	session := &SessionRecord{
		Token:     token,
		UserID:    userID,
		CreatedAt: now,
		ExpiresAt: now.Add(sessionMaxAge),
	}
	_, err = s.db.Exec(
		`INSERT INTO sessions (token, user_id, expires_at, created_at) VALUES (?, ?, ?, ?)`,
		session.Token,
		session.UserID,
		session.ExpiresAt.Unix(),
		session.CreatedAt.Unix(),
	)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (s *SQLiteStore) getSession(token string) (*SessionRecord, error) {
	row := s.db.QueryRow(`SELECT token, user_id, expires_at, created_at FROM sessions WHERE token = ?`, token)
	var expiresAtUnix int64
	var createdAtUnix int64
	session := &SessionRecord{}
	err := row.Scan(&session.Token, &session.UserID, &expiresAtUnix, &createdAtUnix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errUserNotFound
		}
		return nil, err
	}
	session.ExpiresAt = time.Unix(expiresAtUnix, 0)
	session.CreatedAt = time.Unix(createdAtUnix, 0)
	if session.ExpiresAt.Before(time.Now()) {
		_ = s.deleteSession(token)
		return nil, errUserNotFound
	}
	return session, nil
}

func (s *SQLiteStore) deleteSession(token string) error {
	_, err := s.db.Exec(`DELETE FROM sessions WHERE token = ?`, token)
	return err
}

func (s *SQLiteStore) getUserByID(id int64) (*UserRecord, error) {
	row := s.db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE id = ?`, id)
	return scanUser(row)
}

func (s *SQLiteStore) createPlan(plan *PlanRecord) error {
	levelsJSON, err := json.Marshal(plan.Levels)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	plan.CreatedAt = time.Unix(now, 0)
	plan.UpdatedAt = plan.CreatedAt
	_, err = s.db.Exec(
		`INSERT INTO plans (
			id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
			schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		plan.ID,
		plan.UserID,
		plan.ParentID,
		string(plan.Kind),
		plan.Name,
		plan.SourceName,
		plan.URL,
		plan.Format,
		plan.Schema,
		plan.Workers,
		plan.SavePipe,
		plan.TimeDelay,
		string(plan.ScheduleMode),
		plan.RunAt.Unix(),
		string(plan.Status),
		string(levelsJSON),
		plan.LastRunID,
		now,
		now,
	)
	if err != nil {
		return err
	}
	return s.syncNormalizedPlan(plan)
}

func (s *SQLiteStore) listPlansByUser(userID int64) ([]*PlanRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
		        schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		   FROM plans
		  WHERE user_id = ? AND parent_id = ''
		  ORDER BY updated_at DESC`,
		userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*PlanRecord
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return s.attachPlanRelations(plans)
}

func (s *SQLiteStore) getPlanForUser(userID int64, planID string) (*PlanRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
		        schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		   FROM plans
		  WHERE id = ? AND user_id = ?`,
		planID,
		userID,
	)
	plan, err := scanPlan(row)
	if err != nil {
		return nil, err
	}
	attached, err := s.attachPlanRelations([]*PlanRecord{plan})
	if err != nil {
		return nil, err
	}
	return attached[0], nil
}

func (s *SQLiteStore) listRunsByPlan(planID string) ([]*TaskRunRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, plan_id, user_id, status, trigger_mode, output_path, artifact_path, artifact_name, artifact_status,
		        total, current, success_count, failure_count, error_message, started_at, finished_at, created_at, updated_at
		   FROM task_runs
		  WHERE plan_id = ?
		  ORDER BY created_at DESC`,
		planID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var runs []*TaskRunRecord
	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (s *SQLiteStore) getPlanByID(planID string) (*PlanRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
		        schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		   FROM plans
		  WHERE id = ?`,
		planID,
	)
	return scanPlan(row)
}

func (s *SQLiteStore) listDuePlans(now time.Time) ([]*PlanRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
		        schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		   FROM plans
		  WHERE status = ? AND run_at <= ? AND parent_id = ''
		  ORDER BY run_at ASC`,
		string(PlanScheduled),
		now.Unix(),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*PlanRecord
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachPlanRelations(plans)
}

func (s *SQLiteStore) markPlanRunning(planID, runID string) error {
	_, err := s.db.Exec(
		`UPDATE plans SET status = ?, last_run_id = ?, updated_at = ? WHERE id = ?`,
		string(PlanRunning),
		runID,
		time.Now().Unix(),
		planID,
	)
	if err != nil {
		return err
	}
	return s.syncTaskStatus(planID, PlanRunning)
}

func (s *SQLiteStore) updatePlanStatus(planID string, status PlanStatus) error {
	_, err := s.db.Exec(
		`UPDATE plans SET status = ?, updated_at = ? WHERE id = ?`,
		string(status),
		time.Now().Unix(),
		planID,
	)
	if err != nil {
		return err
	}
	return s.syncTaskStatus(planID, status)
}

func (s *SQLiteStore) createRun(run *TaskRunRecord) error {
	now := time.Now().Unix()
	run.CreatedAt = time.Unix(now, 0)
	run.UpdatedAt = run.CreatedAt
	_, err := s.db.Exec(
		`INSERT INTO task_runs (
			id, plan_id, user_id, status, trigger_mode, output_path, artifact_path, artifact_name, artifact_status,
			total, current, success_count, failure_count, error_message, started_at, finished_at, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.ID,
		run.PlanID,
		run.UserID,
		string(run.Status),
		run.TriggerMode,
		run.OutputPath,
		run.ArtifactPath,
		run.ArtifactName,
		string(run.ArtifactStatus),
		run.Total,
		run.Current,
		run.SuccessCount,
		run.FailureCount,
		run.ErrorMessage,
		timeToUnix(run.StartedAt),
		timeToUnix(run.FinishedAt),
		now,
		now,
	)
	if err != nil {
		return err
	}
	return s.syncRunTaskID(run.ID, run.PlanID)
}

func (s *SQLiteStore) updateRunProgress(run *TaskRunRecord) error {
	_, err := s.db.Exec(
		`UPDATE task_runs
		    SET status = ?, output_path = ?, total = ?, current = ?, success_count = ?, failure_count = ?,
		        error_message = ?, started_at = ?, finished_at = ?, artifact_status = ?, updated_at = ?
		  WHERE id = ?`,
		string(run.Status),
		run.OutputPath,
		run.Total,
		run.Current,
		run.SuccessCount,
		run.FailureCount,
		run.ErrorMessage,
		timeToUnix(run.StartedAt),
		timeToUnix(run.FinishedAt),
		string(run.ArtifactStatus),
		time.Now().Unix(),
		run.ID,
	)
	return err
}

func (s *SQLiteStore) finalizeRun(run *TaskRunRecord) error {
	_, err := s.db.Exec(
		`UPDATE task_runs
		    SET status = ?, output_path = ?, artifact_path = ?, artifact_name = ?, artifact_status = ?,
		        total = ?, current = ?, success_count = ?, failure_count = ?, error_message = ?,
		        started_at = ?, finished_at = ?, updated_at = ?
		  WHERE id = ?`,
		string(run.Status),
		run.OutputPath,
		run.ArtifactPath,
		run.ArtifactName,
		string(run.ArtifactStatus),
		run.Total,
		run.Current,
		run.SuccessCount,
		run.FailureCount,
		run.ErrorMessage,
		timeToUnix(run.StartedAt),
		timeToUnix(run.FinishedAt),
		time.Now().Unix(),
		run.ID,
	)
	if err != nil {
		return err
	}
	return s.upsertArtifactFromRun(run)
}

func (s *SQLiteStore) getRun(runID string) (*TaskRunRecord, error) {
	row := s.db.QueryRow(
		`SELECT id, plan_id, user_id, status, trigger_mode, output_path, artifact_path, artifact_name, artifact_status,
		        total, current, success_count, failure_count, error_message, started_at, finished_at, created_at, updated_at
		   FROM task_runs WHERE id = ?`,
		runID,
	)
	return scanRun(row)
}

func (s *SQLiteStore) purgePlan(planID string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM task_runs WHERE plan_id IN (SELECT id FROM plans WHERE id = ? OR parent_id = ?)`, planID, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM artifacts WHERE task_id = ? OR task_id IN (SELECT id FROM plans WHERE parent_id = ?)`, planID, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM failures WHERE task_id = ? OR task_id IN (SELECT id FROM plans WHERE parent_id = ?)`, planID, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM task_sources WHERE task_id = ? OR task_id IN (SELECT id FROM plans WHERE parent_id = ?)`, planID, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM tasks WHERE id = ? OR parent_id = ?`, planID, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM plans WHERE parent_id = ?`, planID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM plans WHERE id = ?`, planID); err != nil {
		return err
	}

	return tx.Commit()
}

func timeToUnix(t *time.Time) int64 {
	if t == nil {
		return 0
	}
	return t.Unix()
}

func unixToTime(v int64) *time.Time {
	if v == 0 {
		return nil
	}
	t := time.Unix(v, 0)
	return &t
}

func newToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func hashPassword(password string) string {
	sum := sha256.Sum256([]byte(password))
	return hex.EncodeToString(sum[:])
}

type scanner interface {
	Scan(dest ...interface{}) error
}

func scanUser(row scanner) (*UserRecord, error) {
	var createdAtUnix int64
	user := &UserRecord{}
	err := row.Scan(&user.ID, &user.Username, &user.PasswordHash, &createdAtUnix)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errUserNotFound
		}
		return nil, err
	}
	user.CreatedAt = time.Unix(createdAtUnix, 0)
	return user, nil
}

func scanPlan(row scanner) (*PlanRecord, error) {
	var runAtUnix, createdAtUnix, updatedAtUnix int64
	var parentID string
	var kind string
	var scheduleMode string
	var status string
	var sourceName string
	var levelsJSON string

	plan := &PlanRecord{}
	err := row.Scan(
		&plan.ID,
		&plan.UserID,
		&parentID,
		&kind,
		&plan.Name,
		&sourceName,
		&plan.URL,
		&plan.Format,
		&plan.Schema,
		&plan.Workers,
		&plan.SavePipe,
		&plan.TimeDelay,
		&scheduleMode,
		&runAtUnix,
		&status,
		&levelsJSON,
		&plan.LastRunID,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, errTaskNotFound
		}
		return nil, err
	}

	plan.ParentID = parentID
	if kind == "" {
		kind = string(PlanKindSingle)
	}
	plan.Kind = PlanKind(kind)
	plan.SourceName = sourceName
	plan.ScheduleMode = ScheduleMode(scheduleMode)
	plan.Status = PlanStatus(status)
	plan.RunAt = time.Unix(runAtUnix, 0)
	plan.CreatedAt = time.Unix(createdAtUnix, 0)
	plan.UpdatedAt = time.Unix(updatedAtUnix, 0)
	if err := json.Unmarshal([]byte(levelsJSON), &plan.Levels); err != nil {
		return nil, fmt.Errorf("invalid levels json: %w", err)
	}
	return plan, nil
}

func (s *SQLiteStore) listChildrenByParent(parentID string) ([]*PlanRecord, error) {
	rows, err := s.db.Query(
		`SELECT id, user_id, parent_id, kind, name, source_name, url, format, schema, workers, save_pipe, time_delay,
		        schedule_mode, run_at, status, levels_json, last_run_id, created_at, updated_at
		   FROM plans
		  WHERE parent_id = ?
		  ORDER BY created_at ASC`,
		parentID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var plans []*PlanRecord
	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return s.attachPlanRelations(plans)
}

func (s *SQLiteStore) attachPlanRelations(plans []*PlanRecord) ([]*PlanRecord, error) {
	for _, plan := range plans {
		if plan.LastRunID != "" {
			run, err := s.getRun(plan.LastRunID)
			if err == nil {
				plan.LastRun = run
			}
		}
		if plan.Kind == PlanKindGroup {
			children, err := s.listChildrenByParent(plan.ID)
			if err != nil {
				return nil, err
			}
			plan.Children = children
		}
	}
	return plans, nil
}

func scanRun(row scanner) (*TaskRunRecord, error) {
	var status string
	var artifactStatus string
	var startedAtUnix, finishedAtUnix, createdAtUnix, updatedAtUnix int64
	run := &TaskRunRecord{}
	err := row.Scan(
		&run.ID,
		&run.PlanID,
		&run.UserID,
		&status,
		&run.TriggerMode,
		&run.OutputPath,
		&run.ArtifactPath,
		&run.ArtifactName,
		&artifactStatus,
		&run.Total,
		&run.Current,
		&run.SuccessCount,
		&run.FailureCount,
		&run.ErrorMessage,
		&startedAtUnix,
		&finishedAtUnix,
		&createdAtUnix,
		&updatedAtUnix,
	)
	if err != nil {
		return nil, err
	}
	run.Status = TaskStatus(status)
	run.ArtifactStatus = ArtifactStatus(artifactStatus)
	run.StartedAt = unixToTime(startedAtUnix)
	run.FinishedAt = unixToTime(finishedAtUnix)
	run.CreatedAt = time.Unix(createdAtUnix, 0)
	run.UpdatedAt = time.Unix(updatedAtUnix, 0)
	return run, nil
}
