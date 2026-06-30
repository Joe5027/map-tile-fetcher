package main

import (
	"encoding/json"
	"errors"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	log "github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/teris-io/shortid"

	"tiler/internal/area"
	"tiler/internal/downloader"
)

type LevelRequest struct {
	MinZoom      int          `json:"minZoom" binding:"required"`
	MaxZoom      int          `json:"maxZoom" binding:"required"`
	Geojson      string       `json:"geojson,omitempty"`
	URL          string       `json:"url"`
	Mode         string       `json:"mode,omitempty"`
	BBox         *BBoxRequest `json:"bbox,omitempty"`
	OutputFormat string       `json:"outputFormat,omitempty"`
}

type SourceRequest struct {
	ID     int    `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url" binding:"required"`
	Format string `json:"format" binding:"required"`
	Schema string `json:"schema"`
}

type BBoxRequest struct {
	MinLon float64 `json:"minLon"`
	MinLat float64 `json:"minLat"`
	MaxLon float64 `json:"maxLon"`
	MaxLat float64 `json:"maxLat"`
}

type CoordinateRequest struct {
	Lon float64 `json:"lon"`
	Lat float64 `json:"lat"`
}

type AreaRequest struct {
	BBox     *BBoxRequest        `json:"bbox,omitempty"`
	Polygon  []CoordinateRequest `json:"polygon,omitempty"`
	GeoJSON  string              `json:"geojson,omitempty"`
	RegionID string              `json:"regionId,omitempty"`
}

type ZoomRangeRequest struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

type OutputRequest struct {
	Format string `json:"format,omitempty"`
}

type CreateTaskRequest struct {
	Name         string            `json:"name" binding:"required"`
	Mode         string            `json:"mode,omitempty"`
	Area         AreaRequest       `json:"area,omitempty"`
	Zoom         *ZoomRangeRequest `json:"zoom,omitempty"`
	SourceName   string            `json:"sourceName,omitempty"`
	URL          string            `json:"url"`
	Format       string            `json:"format"`
	Schema       string            `json:"schema"`
	Workers      int               `json:"workers"`
	SavePipe     int               `json:"savePipe"`
	TimeDelay    int               `json:"timeDelay"`
	ScheduleMode ScheduleMode      `json:"scheduleMode"`
	RunAt        string            `json:"runAt"`
	Levels       []LevelRequest    `json:"levels"`
	Sources      []SourceRequest   `json:"sources"`
	Output       OutputRequest     `json:"output,omitempty"`
}

type TaskAreaLevelResponse struct {
	MinZoom int    `json:"minZoom"`
	MaxZoom int    `json:"maxZoom"`
	GeoJSON string `json:"geojson,omitempty"`
}

type TaskAreaResponse struct {
	Mode   string                  `json:"mode"`
	BBox   *BBoxRequest            `json:"bbox,omitempty"`
	Levels []TaskAreaLevelResponse `json:"levels,omitempty"`
}

type TaskSourceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Layer  string `json:"layer,omitempty"`
	URL    string `json:"url"`
	Format string `json:"format"`
	Schema string `json:"schema"`
}

type TaskProgressResponse struct {
	Total   int64   `json:"total"`
	Current int64   `json:"current"`
	Success int64   `json:"success"`
	Failure int64   `json:"failure"`
	Ratio   float64 `json:"ratio"`
}

type TaskArtifactResponse struct {
	Status        ArtifactStatus `json:"status"`
	Name          string         `json:"name,omitempty"`
	DownloadURL   string         `json:"downloadUrl,omitempty"`
	Format        string         `json:"format,omitempty"`
	PackageFormat string         `json:"packageFormat,omitempty"`
}

type TaskFailureSummaryResponse struct {
	FailureCount          int64 `json:"failureCount"`
	RetryableFailureCount int64 `json:"retryableFailureCount"`
	CanRetryFailures      bool  `json:"canRetryFailures"`
}

type TaskResponse struct {
	ID                    string                     `json:"id"`
	ParentID              string                     `json:"parentId,omitempty"`
	Kind                  string                     `json:"kind"`
	Mode                  string                     `json:"mode"`
	Name                  string                     `json:"name"`
	SourceName            string                     `json:"sourceName,omitempty"`
	Area                  TaskAreaResponse           `json:"area"`
	Sources               []TaskSourceResponse       `json:"sources,omitempty"`
	Zoom                  *ZoomRangeRequest          `json:"zoom,omitempty"`
	File                  string                     `json:"file,omitempty"`
	MinZoom               int                        `json:"minZoom"`
	MaxZoom               int                        `json:"maxZoom"`
	Total                 int64                      `json:"total"`
	Current               int64                      `json:"current"`
	Progress              TaskProgressResponse       `json:"progress"`
	Status                string                     `json:"status"`
	SuccessCount          int64                      `json:"successCount"`
	FailureCount          int64                      `json:"failureCount"`
	RetryableFailureCount int64                      `json:"retryableFailureCount"`
	CanRetryFailures      bool                       `json:"canRetryFailures"`
	Artifact              TaskArtifactResponse       `json:"artifact"`
	FailureSummary        TaskFailureSummaryResponse `json:"failureSummary"`
	StartedAt             string                     `json:"startedAt,omitempty"`
	FinishedAt            string                     `json:"finishedAt,omitempty"`
	ErrorMessage          string                     `json:"errorMessage,omitempty"`
	ScheduleMode          ScheduleMode               `json:"scheduleMode"`
	RunAt                 string                     `json:"runAt"`
	ArtifactStatus        ArtifactStatus             `json:"artifactStatus"`
	ArtifactName          string                     `json:"artifactName,omitempty"`
	DownloadURL           string                     `json:"downloadUrl,omitempty"`
	TotalChildren         int                        `json:"totalChildren,omitempty"`
	CompletedChildren     int                        `json:"completedChildren,omitempty"`
	RunningChildren       int                        `json:"runningChildren,omitempty"`
	PausedChildren        int                        `json:"pausedChildren,omitempty"`
	FailedChildren        int                        `json:"failedChildren,omitempty"`
	CancelledChildren     int                        `json:"cancelledChildren,omitempty"`
	Children              []TaskResponse             `json:"children,omitempty"`
}

type FailureRecordResponse struct {
	TaskID       string `json:"taskId"`
	RunID        string `json:"runId"`
	SourceID     string `json:"sourceId,omitempty"`
	Z            int    `json:"z"`
	X            int    `json:"x"`
	Y            int    `json:"y"`
	URL          string `json:"url"`
	ErrorMessage string `json:"errorMessage"`
	Retryable    bool   `json:"retryable"`
	Attempt      int    `json:"attempt"`
	CreatedAt    string `json:"createdAt"`
}

type AuthLoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

type TileMapConfig struct {
	ID      int    `json:"id"`
	Name    string `json:"name"`
	URL     string `json:"url"`
	Format  string `json:"format"`
	Schema  string `json:"schema"`
	MinZoom int    `json:"min_zoom"`
	MaxZoom int    `json:"max_zoom"`
}

type DownloadRegionConfig struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	MinZoom int    `json:"min_zoom"`
	MaxZoom int    `json:"max_zoom"`
	GeoJSON string `json:"geojson"`
}

type RegionCatalogItem struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Level    string `json:"level"`
	ParentID string `json:"parentId"`
	GeoJSON  string `json:"geojson"`
}

type RegionCatalogResponse struct {
	Available []RegionCatalogItem `json:"available"`
	Missing   []RegionCatalogItem `json:"missing"`
}

func initServer() {
	runtimeManager = NewRuntimeManager()
	scheduler := NewScheduler(runtimeManager)
	scheduler.Start()

	gin.SetMode(gin.ReleaseMode)
	r := gin.Default()
	r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"*"},
		AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowHeaders:     []string{"Origin", "Content-Type", "Accept"},
		ExposeHeaders:    []string{"Content-Length", "Content-Disposition"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	}))

	r.Static("/static", "./static")
	r.GET("/", func(c *gin.Context) {
		c.File("./static/index.html")
	})

	r.POST("/api/auth/login", loginHandler)
	r.POST("/api/auth/logout", logoutHandler)

	protected := r.Group("/api")
	protected.Use(authMiddleware())
	{
		protected.GET("/auth/me", meHandler)
		protected.POST("/tasks", createTask)
		protected.GET("/tasks", listTasks)
		protected.GET("/tasks/:id", getTask)
		protected.PUT("/tasks/:id/pause", pauseTask)
		protected.PUT("/tasks/:id/resume", resumeTask)
		protected.DELETE("/tasks/:id", cancelTask)
		protected.DELETE("/tasks/:id/purge", purgeTask)
		protected.GET("/tasks/:id/download", downloadTaskArtifact)
		protected.GET("/tasks/:id/failures", getTaskFailures)
		protected.POST("/tasks/:id/retry-failures", retryTaskFailures)

		protected.GET("/maps", getMaps)
		protected.GET("/config/tilemaps", getConfiguredMaps)
		protected.GET("/config/regions", getConfiguredRegions)
		protected.GET("/config/region-catalog", getRegionCatalog)
		protected.GET("/config/region-catalog/:id/geojson", getRegionCatalogGeoJSON)
		protected.GET("/config/geojson-files", getGeoJSONFiles)
	}

	port := strings.TrimSpace(viper.GetString("app.port"))
	if port == "" {
		port = "8081"
	}
	if !strings.HasPrefix(port, ":") {
		port = ":" + port
	}
	addr := port
	log.Infof("Server starting on %s", addr)
	if err := r.Run(addr); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}

func loginHandler(c *gin.Context) {
	if !authEnabled() {
		user, err := store.defaultUser()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load default user"})
			return
		}
		c.JSON(http.StatusOK, gin.H{"id": user.ID, "username": user.Username, "authEnabled": false})
		return
	}

	var req AuthLoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	user, err := store.authenticateUser(strings.TrimSpace(req.Username), req.Password)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid username or password"})
		return
	}

	session, err := store.createSession(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	c.SetCookie(sessionCookie, session.Token, int(sessionMaxAge.Seconds()), "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "username": user.Username})
}

func logoutHandler(c *gin.Context) {
	if !authEnabled() {
		c.JSON(http.StatusOK, gin.H{"status": "ok", "authEnabled": false})
		return
	}
	token, _ := c.Cookie(sessionCookie)
	if token != "" {
		_ = store.deleteSession(token)
	}
	c.SetCookie(sessionCookie, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func meHandler(c *gin.Context) {
	user := currentUser(c)
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "username": user.Username, "authEnabled": authEnabled()})
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if !authEnabled() {
			user, err := store.defaultUser()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load default user"})
				c.Abort()
				return
			}
			c.Set("user", user)
			c.Next()
			return
		}

		token, err := c.Cookie(sessionCookie)
		if err != nil || token == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "authentication required"})
			c.Abort()
			return
		}

		session, err := store.getSession(token)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid session"})
			c.Abort()
			return
		}

		user, err := store.getUserByID(session.UserID)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid user"})
			c.Abort()
			return
		}

		c.Set("user", user)
		c.Next()
	}
}

func authEnabled() bool {
	return viper.GetBool("auth.enabled")
}

func currentUser(c *gin.Context) *UserRecord {
	user, _ := c.Get("user")
	return user.(*UserRecord)
}

func createTask(c *gin.Context) {
	user := currentUser(c)

	var req CreateTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	plan, children, err := buildTaskRecordsFromRequest(user.ID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := store.createTaskRecord(plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store task"})
		return
	}
	for _, child := range children {
		if err := store.createTaskRecord(child); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store child task"})
			return
		}
	}

	if plan.RunAt.Before(time.Now().Add(2 * time.Second)) {
		_ = runtimeManager.StartTaskRecord(plan)
	}

	refreshed, err := store.getTaskRecordForUser(user.ID, plan.ID)
	if err != nil {
		c.JSON(http.StatusCreated, taskResponseFromRecord(plan))
		return
	}
	c.JSON(http.StatusCreated, taskResponseFromRecord(refreshed))
}

func listTasks(c *gin.Context) {
	user := currentUser(c)

	plans, err := store.listTaskRecordsByUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}

	response := make([]TaskResponse, 0, len(plans))
	for _, plan := range plans {
		response = append(response, taskResponseFromRecord(plan))
	}
	c.JSON(http.StatusOK, response)
}

func getTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, taskResponseFromRecord(plan))
}

func pauseTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err := runtimeManager.Pause(plan.ID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	refreshed, _ := store.getTaskRecordForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusOK, taskResponseFromRecord(refreshed))
}

func resumeTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err := runtimeManager.Resume(plan.ID); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	refreshed, _ := store.getTaskRecordForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusOK, taskResponseFromRecord(refreshed))
}

func cancelTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if err := runtimeManager.Cancel(plan); err != nil {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	refreshed, _ := store.getTaskRecordForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusOK, taskResponseFromRecord(refreshed))
}

func purgeTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	switch plan.Status {
	case TaskRecordRunning, TaskRecordPaused:
		c.JSON(http.StatusConflict, gin.H{"error": "running task cannot be deleted"})
		return
	}

	if plan.LastRun != nil {
		switch plan.LastRun.Status {
		case TaskRunning, TaskPaused, TaskPending:
			c.JSON(http.StatusConflict, gin.H{"error": "running task cannot be deleted"})
			return
		}
	}

	if err := runtimeManager.Purge(plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted", "id": plan.ID})
}

func downloadTaskArtifact(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if plan.LastRun == nil || plan.LastRun.ArtifactStatus != ArtifactReady || plan.LastRun.ArtifactPath == "" {
		c.JSON(http.StatusConflict, gin.H{"error": "artifact is not ready"})
		return
	}
	c.FileAttachment(plan.LastRun.ArtifactPath, plan.LastRun.ArtifactName)
}

func getTaskFailures(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	records, err := store.listFailureRecords(plan.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load failure records"})
		return
	}
	response := make([]FailureRecordResponse, 0, len(records))
	for _, record := range records {
		response = append(response, FailureRecordResponse{
			TaskID:       record.TaskID,
			RunID:        record.RunID,
			SourceID:     record.SourceID,
			Z:            record.Z,
			X:            record.X,
			Y:            record.Y,
			URL:          record.URL,
			ErrorMessage: record.ErrorMessage,
			Retryable:    record.Retryable,
			Attempt:      record.Attempt,
			CreatedAt:    record.CreatedAt.Format(time.RFC3339),
		})
	}
	c.JSON(http.StatusOK, response)
}

func retryTaskFailures(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	if !canRetryFailureStatus(plan.Status) {
		c.JSON(http.StatusConflict, gin.H{"error": "task status does not allow failure retry"})
		return
	}
	summary, err := store.failureSummary(plan.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to inspect failure records"})
		return
	}
	if summary.Retryable == 0 {
		c.JSON(http.StatusConflict, gin.H{"error": "task has no retryable failures"})
		return
	}
	if err := runtimeManager.RetryFailures(plan); err != nil {
		if errors.Is(err, errNoRetryableFailures) {
			c.JSON(http.StatusConflict, gin.H{"error": "task has no retryable failures"})
			return
		}
		if errors.Is(err, errTaskAlreadyActive) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	refreshed, _ := store.getTaskRecordForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusAccepted, taskResponseFromRecord(refreshed))
}

func canRetryFailureStatus(status TaskRecordStatus) bool {
	switch status {
	case TaskRecordFailed, TaskRecordPartialFailed, TaskRecordCompleted:
		return true
	default:
		return false
	}
}

func loadPlanForCurrentUser(c *gin.Context, id string) (*TaskRecord, error) {
	user := currentUser(c)
	return store.getTaskRecordForUser(user.ID, id)
}

func buildTaskRecordsFromRequest(userID int64, req CreateTaskRequest) (*TaskRecord, []*TaskRecord, error) {
	req.Name = strings.TrimSpace(req.Name)

	if req.Name == "" {
		return nil, nil, errors.New("name is required")
	}

	mode := req.ScheduleMode
	if mode == "" {
		mode = ScheduleImmediate
	}
	if mode != ScheduleImmediate && mode != ScheduleOnce {
		return nil, nil, errors.New("unsupported schedule mode")
	}

	runAt := time.Now()
	if mode == ScheduleOnce {
		if strings.TrimSpace(req.RunAt) == "" {
			return nil, nil, errors.New("runAt is required for scheduled tasks")
		}
		parsed, err := time.Parse(time.RFC3339, req.RunAt)
		if err != nil {
			return nil, nil, errors.New("runAt must be RFC3339 time")
		}
		runAt = parsed
	}

	sources, err := normalizeSources(req)
	if err != nil {
		return nil, nil, err
	}
	outputFormat, err := normalizeOutputFormat(req.Output.Format)
	if err != nil {
		return nil, nil, err
	}

	if err := normalizeAreaLevels(&req); err != nil {
		return nil, nil, err
	}

	if len(req.Levels) == 0 {
		return nil, nil, errors.New("at least one level configuration is required")
	}

	levels := make([]LevelConfig, 0, len(req.Levels))
	for _, level := range req.Levels {
		normalized, err := normalizeLevelConfig(level)
		if err != nil {
			return nil, nil, err
		}
		normalized.OutputFormat = outputFormat
		levels = append(levels, normalized)
	}

	groupID, _ := shortid.Generate()
	parent := &TaskRecord{
		ID:           groupID,
		UserID:       userID,
		Kind:         TaskRecordKindGroup,
		Name:         req.Name,
		URL:          sources[0].URL,
		Format:       sources[0].Format,
		Schema:       sources[0].Schema,
		Workers:      firstPositive(req.Workers, viper.GetInt("task.workers")),
		SavePipe:     firstPositive(req.SavePipe, viper.GetInt("task.savepipe")),
		TimeDelay:    maxInt(req.TimeDelay, 0),
		ScheduleMode: mode,
		RunAt:        runAt,
		Status:       TaskRecordScheduled,
		Levels:       levels,
	}

	children := make([]*TaskRecord, 0, len(sources))
	for _, source := range sources {
		id, _ := shortid.Generate()
		children = append(children, &TaskRecord{
			ID:           id,
			UserID:       userID,
			ParentID:     groupID,
			Kind:         TaskRecordKindChild,
			Name:         req.Name,
			SourceName:   source.Name,
			URL:          source.URL,
			Format:       source.Format,
			Schema:       source.Schema,
			Workers:      parent.Workers,
			SavePipe:     parent.SavePipe,
			TimeDelay:    parent.TimeDelay,
			ScheduleMode: mode,
			RunAt:        runAt,
			Status:       TaskRecordScheduled,
			Levels:       levels,
		})
	}

	return parent, children, nil
}

func normalizeAreaLevels(req *CreateTaskRequest) error {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" && (req.Area.BBox != nil || len(req.Area.Polygon) > 0) {
		mode = string(area.ModeBBox)
	}
	if mode == "" {
		mode = string(area.ModeRegion)
	}

	switch area.Mode(mode) {
	case area.ModeRegion:
		if len(req.Levels) > 0 {
			return nil
		}
		if req.Zoom == nil {
			return errors.New("zoom is required when region levels are omitted")
		}
		zoom := area.ZoomRange{Min: req.Zoom.Min, Max: req.Zoom.Max}
		if err := zoom.Validate(); err != nil {
			return err
		}
		geojsonPath := strings.TrimSpace(req.Area.GeoJSON)
		if geojsonPath == "" {
			return errors.New("area.geojson is required for region tasks without levels")
		}
		req.Levels = []LevelRequest{{
			MinZoom: zoom.Min,
			MaxZoom: zoom.Max,
			Geojson: geojsonPath,
		}}
		return nil
	case area.ModeBBox:
		if req.Area.BBox == nil && len(req.Area.Polygon) == 0 {
			return errors.New("area.bbox or area.polygon is required for bbox tasks")
		}
		if req.Zoom == nil {
			return errors.New("zoom is required for bbox tasks")
		}
		zoom := area.ZoomRange{Min: req.Zoom.Min, Max: req.Zoom.Max}
		if err := zoom.Validate(); err != nil {
			return err
		}
		if len(req.Area.Polygon) > 0 {
			geojsonPath, err := writeGeneratedPolygonGeoJSON(req.Area.Polygon)
			if err != nil {
				return err
			}
			req.Levels = []LevelRequest{{
				MinZoom: zoom.Min,
				MaxZoom: zoom.Max,
				Geojson: geojsonPath,
			}}
			return nil
		}
		box := area.BBox{
			MinLon: req.Area.BBox.MinLon,
			MinLat: req.Area.BBox.MinLat,
			MaxLon: req.Area.BBox.MaxLon,
			MaxLat: req.Area.BBox.MaxLat,
		}
		count, err := downloader.CountBBoxTiles(box, zoom)
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("bbox task has no tiles")
		}
		req.Levels = []LevelRequest{{
			MinZoom: zoom.Min,
			MaxZoom: zoom.Max,
			Mode:    string(area.ModeBBox),
			BBox:    req.Area.BBox,
		}}
		return nil
	default:
		return errors.New("mode must be bbox or region")
	}
}

func writeGeneratedPolygonGeoJSON(points []CoordinateRequest) (string, error) {
	normalized, err := normalizePolygonCoordinates(points)
	if err != nil {
		return "", err
	}

	id, err := shortid.Generate()
	if err != nil {
		return "", err
	}

	ring := make([][]float64, 0, len(normalized)+1)
	for _, point := range normalized {
		ring = append(ring, []float64{point.Lon, point.Lat})
	}
	ring = append(ring, []float64{normalized[0].Lon, normalized[0].Lat})

	featureCollection := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{{
			"type": "Feature",
			"properties": map[string]any{
				"name": "range-polygon",
			},
			"geometry": map[string]any{
				"type":        "Polygon",
				"coordinates": [][][]float64{ring},
			},
		}},
	}
	data, err := json.MarshalIndent(featureCollection, "", "  ")
	if err != nil {
		return "", err
	}

	dir := filepath.Join("data", "generated-areas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "range-polygon-"+id+".geojson")
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return filepath.ToSlash(path), nil
}

func normalizePolygonCoordinates(points []CoordinateRequest) ([]CoordinateRequest, error) {
	if len(points) < 3 {
		return nil, errors.New("area.polygon requires at least three points")
	}

	normalized := make([]CoordinateRequest, 0, len(points))
	unique := make(map[string]struct{}, len(points))
	minLon, minLat := math.Inf(1), math.Inf(1)
	maxLon, maxLat := math.Inf(-1), math.Inf(-1)
	for _, point := range points {
		if !isValidRangeCoordinate(point) {
			return nil, errors.New("area.polygon contains invalid coordinates")
		}
		normalized = append(normalized, point)
		key := strings.TrimRight(strings.TrimRight(formatFloat6(point.Lon), "0"), ".") + "," + strings.TrimRight(strings.TrimRight(formatFloat6(point.Lat), "0"), ".")
		unique[key] = struct{}{}
		minLon = math.Min(minLon, point.Lon)
		minLat = math.Min(minLat, point.Lat)
		maxLon = math.Max(maxLon, point.Lon)
		maxLat = math.Max(maxLat, point.Lat)
	}
	if len(unique) < 3 {
		return nil, errors.New("area.polygon requires at least three unique points")
	}
	if minLon >= maxLon || minLat >= maxLat {
		return nil, errors.New("area.polygon has no area")
	}
	return normalized, nil
}

func isValidRangeCoordinate(point CoordinateRequest) bool {
	if math.IsNaN(point.Lon) || math.IsNaN(point.Lat) || math.IsInf(point.Lon, 0) || math.IsInf(point.Lat, 0) {
		return false
	}
	return point.Lon >= -180 && point.Lon <= 180 && point.Lat >= -85.05112878 && point.Lat <= 85.05112878
}

func formatFloat6(value float64) string {
	return strconv.FormatFloat(value, 'f', 6, 64)
}

func normalizeSources(req CreateTaskRequest) ([]SourceRequest, error) {
	sources := make([]SourceRequest, 0, len(req.Sources))
	for _, source := range req.Sources {
		source.URL = strings.TrimSpace(source.URL)
		source.Format = strings.ToLower(strings.TrimSpace(source.Format))
		source.Schema = strings.ToLower(strings.TrimSpace(source.Schema))
		source.Name = strings.TrimSpace(source.Name)
		if source.URL == "" {
			return nil, errors.New("source url is required")
		}
		if !isSupportedFormat(source.Format) {
			return nil, errors.New("unsupported source format")
		}
		if source.Schema == "" {
			source.Schema = "xyz"
		}
		if source.Schema != "xyz" && source.Schema != "tms" {
			return nil, errors.New("source schema must be xyz or tms")
		}
		if source.Name == "" {
			source.Name = source.URL
		}
		sources = append(sources, source)
	}

	if len(sources) == 0 {
		legacy := SourceRequest{
			Name:   "默认地图源",
			URL:    strings.TrimSpace(req.URL),
			Format: strings.ToLower(strings.TrimSpace(req.Format)),
			Schema: strings.ToLower(strings.TrimSpace(req.Schema)),
		}
		if legacy.URL == "" {
			return nil, errors.New("at least one source is required")
		}
		if !isSupportedFormat(legacy.Format) {
			return nil, errors.New("unsupported output format")
		}
		if legacy.Schema == "" {
			legacy.Schema = "xyz"
		}
		if legacy.Schema != "xyz" && legacy.Schema != "tms" {
			return nil, errors.New("schema must be xyz or tms")
		}
		sources = append(sources, legacy)
	}

	return sources, nil
}

func normalizeLevelConfig(level LevelRequest) (LevelConfig, error) {
	if level.MinZoom < ZoomMin || level.MaxZoom > ZoomMax {
		return LevelConfig{}, errors.New("zoom level out of supported range")
	}
	if level.MinZoom > level.MaxZoom {
		return LevelConfig{}, errors.New("minZoom cannot be greater than maxZoom")
	}

	mode := strings.ToLower(strings.TrimSpace(level.Mode))
	if mode == string(area.ModeBBox) || level.BBox != nil {
		if level.BBox == nil {
			return LevelConfig{}, errors.New("bbox level requires bbox")
		}
		box := area.BBox{
			MinLon: level.BBox.MinLon,
			MinLat: level.BBox.MinLat,
			MaxLon: level.BBox.MaxLon,
			MaxLat: level.BBox.MaxLat,
		}
		zoom := area.ZoomRange{Min: level.MinZoom, Max: level.MaxZoom}
		count, err := downloader.CountBBoxTiles(box, zoom)
		if err != nil {
			return LevelConfig{}, err
		}
		if count == 0 {
			return LevelConfig{}, errors.New("bbox task has no tiles")
		}
		bbox := *level.BBox
		return LevelConfig{
			MinZoom: level.MinZoom,
			MaxZoom: level.MaxZoom,
			URL:     strings.TrimSpace(level.URL),
			Mode:    string(area.ModeBBox),
			BBox:    &bbox,
		}, nil
	}

	path := strings.TrimSpace(level.Geojson)
	if path == "" {
		return LevelConfig{}, errors.New("geojson is required")
	}

	resolved, err := resolveGeoJSONPath(path)
	if err != nil {
		return LevelConfig{}, err
	}
	return LevelConfig{
		MinZoom: level.MinZoom,
		MaxZoom: level.MaxZoom,
		Geojson: resolved,
		URL:     strings.TrimSpace(level.URL),
	}, nil
}

func firstPositive(values ...int) int {
	for _, value := range values {
		if value > 0 {
			return value
		}
	}
	return 0
}

func isSupportedFormat(format string) bool {
	switch format {
	case JPG, PNG, PBF, WEBP:
		return true
	default:
		return false
	}
}

func normalizeOutputFormat(format string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(format))
	if normalized == "" {
		normalized = strings.ToLower(strings.TrimSpace(viper.GetString("output.format")))
	}
	if normalized == "" || normalized == "file" || normalized == "zip" {
		return "zip", nil
	}
	if normalized == "mbtiles" {
		return "mbtiles", nil
	}
	return "", errors.New("unsupported output format")
}

func taskResponseFromRecord(plan *TaskRecord) TaskResponse {
	response := TaskResponse{
		ID:             plan.ID,
		ParentID:       plan.ParentID,
		Kind:           string(plan.Kind),
		Mode:           taskModeFromLevels(plan.Levels),
		Name:           plan.Name,
		SourceName:     plan.SourceName,
		Area:           taskAreaFromLevels(plan.Levels),
		Sources:        taskSourcesFromRecord(plan),
		Status:         string(plan.Status),
		ScheduleMode:   plan.ScheduleMode,
		RunAt:          plan.RunAt.Format(time.RFC3339),
		ArtifactStatus: ArtifactNone,
	}

	minZoom := 100
	maxZoom := -1
	for _, level := range plan.Levels {
		if level.MinZoom < minZoom {
			minZoom = level.MinZoom
		}
		if level.MaxZoom > maxZoom {
			maxZoom = level.MaxZoom
		}
	}
	if minZoom != 100 {
		response.MinZoom = minZoom
	}
	if maxZoom >= 0 {
		response.MaxZoom = maxZoom
	}
	if response.MinZoom != 0 || response.MaxZoom != 0 {
		response.Zoom = &ZoomRangeRequest{Min: response.MinZoom, Max: response.MaxZoom}
	}

	if plan.Kind == TaskRecordKindGroup {
		applyGroupSummary(plan, &response)
		applyFailureSummary(plan.ID, &response)
		applyProgressAndArtifact(&response)
		return response
	}

	if plan.LastRun != nil {
		run := plan.LastRun
		response.File = run.OutputPath
		response.Total = run.Total
		response.Current = run.Current
		response.Status = string(effectiveTaskRecordStatusForResponse(plan, run))
		response.SuccessCount = run.SuccessCount
		response.FailureCount = run.FailureCount
		response.ErrorMessage = run.ErrorMessage
		if run.StartedAt != nil {
			response.StartedAt = run.StartedAt.Format(time.RFC3339)
		}
		if run.FinishedAt != nil {
			response.FinishedAt = run.FinishedAt.Format(time.RFC3339)
		}
		response.ArtifactStatus = run.ArtifactStatus
		response.ArtifactName = run.ArtifactName
		if run.ArtifactStatus == ArtifactReady {
			response.DownloadURL = "/api/tasks/" + plan.ID + "/download"
		}
	}

	applyFailureSummary(plan.ID, &response)
	applyProgressAndArtifact(&response)
	return response
}

func applyFailureSummary(planID string, response *TaskResponse) {
	if store == nil {
		return
	}
	summary, err := store.failureSummary(planID)
	if err != nil {
		log.Warnf("failed to load failure summary for task %s: %v", planID, err)
		return
	}
	if summary.Total > response.FailureCount {
		response.FailureCount = summary.Total
	}
	response.RetryableFailureCount = summary.Retryable
	response.CanRetryFailures = summary.Retryable > 0 && canRetryFailureStatus(TaskRecordStatus(response.Status))
	response.FailureSummary = TaskFailureSummaryResponse{
		FailureCount:          response.FailureCount,
		RetryableFailureCount: response.RetryableFailureCount,
		CanRetryFailures:      response.CanRetryFailures,
	}
}

func applyProgressAndArtifact(response *TaskResponse) {
	ratio := 0.0
	if response.Total > 0 {
		ratio = float64(response.Current) / float64(response.Total)
	}
	response.Progress = TaskProgressResponse{
		Total:   response.Total,
		Current: response.Current,
		Success: response.SuccessCount,
		Failure: response.FailureCount,
		Ratio:   ratio,
	}
	response.Artifact = TaskArtifactResponse{
		Status:      response.ArtifactStatus,
		Name:        response.ArtifactName,
		DownloadURL: response.DownloadURL,
	}
	if response.ArtifactName != "" {
		switch {
		case strings.HasSuffix(strings.ToLower(response.ArtifactName), ".zip"):
			response.Artifact.PackageFormat = "zip"
		case strings.HasSuffix(strings.ToLower(response.ArtifactName), ".mbtiles"):
			response.Artifact.Format = "mbtiles"
		}
	}
}

func taskAreaFromLevels(levels []LevelConfig) TaskAreaResponse {
	mode := taskModeFromLevels(levels)
	result := TaskAreaResponse{Mode: mode}
	if mode == string(area.ModeBBox) {
		for _, level := range levels {
			if level.BBox != nil {
				result.BBox = level.BBox
				return result
			}
		}
	}
	result.Levels = make([]TaskAreaLevelResponse, 0, len(levels))
	for _, level := range levels {
		result.Levels = append(result.Levels, TaskAreaLevelResponse{
			MinZoom: level.MinZoom,
			MaxZoom: level.MaxZoom,
			GeoJSON: level.Geojson,
		})
	}
	return result
}

func taskSourcesFromRecord(plan *TaskRecord) []TaskSourceResponse {
	if plan.Kind == TaskRecordKindGroup && len(plan.Children) > 0 {
		sources := make([]TaskSourceResponse, 0, len(plan.Children))
		for _, child := range plan.Children {
			sources = append(sources, taskSourceFromRecord(child))
		}
		return sources
	}
	if strings.TrimSpace(plan.URL) == "" {
		return nil
	}
	return []TaskSourceResponse{taskSourceFromRecord(plan)}
}

func taskSourceFromRecord(plan *TaskRecord) TaskSourceResponse {
	name := strings.TrimSpace(plan.SourceName)
	if name == "" {
		name = strings.TrimSpace(plan.Name)
	}
	return TaskSourceResponse{
		ID:     plan.ID,
		Name:   name,
		Layer:  inferLayerName(name, plan.URL),
		URL:    plan.URL,
		Format: plan.Format,
		Schema: plan.Schema,
	}
}

func effectiveTaskRecordStatusForResponse(plan *TaskRecord, run *TaskRunRecord) TaskStatus {
	if run == nil {
		switch plan.Status {
		case TaskRecordPaused:
			return TaskPaused
		case TaskRecordCancelled:
			return TaskCancelled
		case TaskRecordFailed:
			return TaskFailed
		case TaskRecordCompleted:
			return TaskCompleted
		default:
			return TaskStatus(plan.Status)
		}
	}

	switch plan.Status {
	case TaskRecordPaused:
		if run.Status == TaskRunning || run.Status == TaskPending {
			return TaskPaused
		}
	case TaskRecordCancelled:
		if run.Status == TaskRunning || run.Status == TaskPending || run.Status == TaskPaused {
			return TaskCancelled
		}
	case TaskRecordFailed:
		if run.Status == TaskRunning || run.Status == TaskPending || run.Status == TaskPaused {
			return TaskFailed
		}
	}

	return run.Status
}

func applyGroupSummary(plan *TaskRecord, response *TaskResponse) {
	response.TotalChildren = len(plan.Children)
	response.Children = make([]TaskResponse, 0, len(plan.Children))

	if len(plan.Children) == 0 {
		return
	}

	var totalTiles int64
	var currentTiles int64

	for _, child := range plan.Children {
		childResponse := taskResponseFromRecord(child)
		response.Children = append(response.Children, childResponse)
		totalTiles += childResponse.Total
		currentTiles += childResponse.Current

		switch childResponse.Status {
		case string(TaskCompleted):
			response.CompletedChildren++
		case string(TaskRunning):
			response.RunningChildren++
		case string(TaskPaused):
			response.PausedChildren++
		case string(TaskFailed):
			response.FailedChildren++
		case string(TaskCancelled):
			response.CancelledChildren++
		}
	}

	response.Total = totalTiles
	response.Current = currentTiles

	switch {
	case response.CompletedChildren == response.TotalChildren:
		response.Status = string(TaskRecordCompleted)
	case response.CancelledChildren == response.TotalChildren:
		response.Status = string(TaskRecordCancelled)
	case response.FailedChildren == response.TotalChildren:
		response.Status = string(TaskRecordFailed)
	case response.RunningChildren > 0:
		response.Status = string(TaskRecordRunning)
	case response.PausedChildren > 0 && response.RunningChildren == 0:
		response.Status = string(TaskRecordPaused)
	case response.CompletedChildren+response.FailedChildren+response.CancelledChildren == response.TotalChildren && response.FailedChildren > 0:
		response.Status = string(TaskRecordPartialFailed)
	default:
		response.Status = string(plan.Status)
	}
}

func getMaps(c *gin.Context) {
	maps := GetTileMapList()
	c.JSON(http.StatusOK, maps)
}

func getConfiguredMaps(c *gin.Context) {
	var tileMaps []TileMapConfig

	type ConfigTileMap struct {
		ID      int    `mapstructure:"id"`
		Name    string `mapstructure:"name"`
		URL     string `mapstructure:"url"`
		Format  string `mapstructure:"format"`
		Schema  string `mapstructure:"schema"`
		MinZoom int    `mapstructure:"min_zoom"`
		MaxZoom int    `mapstructure:"max_zoom"`
	}

	var configTileMaps []ConfigTileMap
	if err := viper.UnmarshalKey("tilemaps", &configTileMaps); err != nil {
		log.Errorf("Failed to unmarshal tilemaps: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tilemaps"})
		return
	}

	for _, tm := range configTileMaps {
		tileMaps = append(tileMaps, TileMapConfig{
			ID:      tm.ID,
			Name:    tm.Name,
			URL:     tm.URL,
			Format:  tm.Format,
			Schema:  tm.Schema,
			MinZoom: tm.MinZoom,
			MaxZoom: tm.MaxZoom,
		})
	}
	c.JSON(http.StatusOK, tileMaps)
}

func getGeoJSONFiles(c *gin.Context) {
	files, err := os.ReadDir("./geojson")
	if err != nil {
		log.Errorf("Failed to read geojson directory: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read geojson directory"})
		return
	}

	var geojsonFiles []map[string]string
	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".geojson") {
			name := strings.TrimSuffix(file.Name(), ".geojson")
			geojsonFiles = append(geojsonFiles, map[string]string{
				"file": file.Name(),
				"name": name,
				"path": "./geojson/" + file.Name(),
			})
		}
	}
	c.JSON(http.StatusOK, geojsonFiles)
}

func getRegionCatalog(c *gin.Context) {
	data, err := os.ReadFile("./geojson/regions.json")
	if err != nil {
		log.Errorf("Failed to read region catalog: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load region catalog"})
		return
	}

	var items []RegionCatalogItem
	if err := json.Unmarshal(data, &items); err != nil {
		log.Errorf("Failed to parse region catalog: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse region catalog"})
		return
	}

	availableItems := make([]RegionCatalogItem, 0, len(items))
	missingItems := make([]RegionCatalogItem, 0, len(items))
	for index := range items {
		resolvedPath, err := resolveGeoJSONPath(items[index].GeoJSON)
		if err != nil {
			log.Warnf("region catalog entry %s points to missing geojson %s: %v", items[index].ID, items[index].GeoJSON, err)
			missingItems = append(missingItems, items[index])
			continue
		}
		items[index].GeoJSON = resolvedPath
		availableItems = append(availableItems, items[index])
	}

	c.JSON(http.StatusOK, RegionCatalogResponse{
		Available: availableItems,
		Missing:   missingItems,
	})
}

func getRegionCatalogGeoJSON(c *gin.Context) {
	regionID := strings.TrimSpace(c.Param("id"))
	if regionID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "region id is required"})
		return
	}

	data, err := os.ReadFile("./geojson/regions.json")
	if err != nil {
		log.Errorf("Failed to read region catalog: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load region catalog"})
		return
	}

	var items []RegionCatalogItem
	if err := json.Unmarshal(data, &items); err != nil {
		log.Errorf("Failed to parse region catalog: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to parse region catalog"})
		return
	}

	for _, item := range items {
		if item.ID != regionID {
			continue
		}
		resolvedPath, err := resolveGeoJSONPath(item.GeoJSON)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "region geojson not found"})
			return
		}
		content, err := os.ReadFile(resolvedPath)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to read region geojson"})
			return
		}
		c.Data(http.StatusOK, "application/geo+json; charset=utf-8", content)
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "region not found"})
}

func getConfiguredRegions(c *gin.Context) {
	var regions []DownloadRegionConfig

	type ConfigRegion struct {
		ID      string `mapstructure:"id"`
		Name    string `mapstructure:"name"`
		MinZoom int    `mapstructure:"min_zoom"`
		MaxZoom int    `mapstructure:"max_zoom"`
		GeoJSON string `mapstructure:"geojson"`
	}

	var configRegions []ConfigRegion
	if err := viper.UnmarshalKey("download_regions", &configRegions); err != nil {
		log.Errorf("Failed to unmarshal download_regions: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load regions"})
		return
	}

	for _, cr := range configRegions {
		regions = append(regions, DownloadRegionConfig{
			ID:      cr.ID,
			Name:    cr.Name,
			MinZoom: cr.MinZoom,
			MaxZoom: cr.MaxZoom,
			GeoJSON: cr.GeoJSON,
		})
	}
	c.JSON(http.StatusOK, regions)
}
