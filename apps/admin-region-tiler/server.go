package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
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
	MinZoom int    `json:"minZoom" binding:"required"`
	MaxZoom int    `json:"maxZoom" binding:"required"`
	Geojson string `json:"geojson" binding:"required"`
	URL     string `json:"url"`
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

type AreaRequest struct {
	BBox     *BBoxRequest `json:"bbox,omitempty"`
	GeoJSON  string       `json:"geojson,omitempty"`
	RegionID string       `json:"regionId,omitempty"`
}

type ZoomRangeRequest struct {
	Min int `json:"min"`
	Max int `json:"max"`
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
}

type TaskResponse struct {
	ID                string         `json:"id"`
	ParentID          string         `json:"parentId,omitempty"`
	Kind              string         `json:"kind"`
	Name              string         `json:"name"`
	SourceName        string         `json:"sourceName,omitempty"`
	File              string         `json:"file,omitempty"`
	MinZoom           int            `json:"minZoom"`
	MaxZoom           int            `json:"maxZoom"`
	Total             int64          `json:"total"`
	Current           int64          `json:"current"`
	Progress          float64        `json:"progress"`
	Status            string         `json:"status"`
	SuccessCount      int64          `json:"successCount"`
	FailureCount      int64          `json:"failureCount"`
	StartedAt         string         `json:"startedAt,omitempty"`
	FinishedAt        string         `json:"finishedAt,omitempty"`
	ErrorMessage      string         `json:"errorMessage,omitempty"`
	ScheduleMode      ScheduleMode   `json:"scheduleMode"`
	RunAt             string         `json:"runAt"`
	ArtifactStatus    ArtifactStatus `json:"artifactStatus"`
	ArtifactName      string         `json:"artifactName,omitempty"`
	DownloadURL       string         `json:"downloadUrl,omitempty"`
	TotalChildren     int            `json:"totalChildren,omitempty"`
	CompletedChildren int            `json:"completedChildren,omitempty"`
	RunningChildren   int            `json:"runningChildren,omitempty"`
	PausedChildren    int            `json:"pausedChildren,omitempty"`
	FailedChildren    int            `json:"failedChildren,omitempty"`
	CancelledChildren int            `json:"cancelledChildren,omitempty"`
	Children          []TaskResponse `json:"children,omitempty"`
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

		protected.GET("/maps", getMaps)
		protected.GET("/config/tilemaps", getConfiguredMaps)
		protected.GET("/config/regions", getConfiguredRegions)
		protected.GET("/config/region-catalog", getRegionCatalog)
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
	token, _ := c.Cookie(sessionCookie)
	if token != "" {
		_ = store.deleteSession(token)
	}
	c.SetCookie(sessionCookie, "", -1, "/", "", false, true)
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func meHandler(c *gin.Context) {
	user := currentUser(c)
	c.JSON(http.StatusOK, gin.H{"id": user.ID, "username": user.Username})
}

func authMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
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

	plan, children, err := buildPlansFromRequest(user.ID, req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if err := store.createPlan(plan); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store task"})
		return
	}
	for _, child := range children {
		if err := store.createPlan(child); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to store child task"})
			return
		}
	}

	if plan.RunAt.Before(time.Now().Add(2 * time.Second)) {
		_ = runtimeManager.StartPlan(plan)
	}

	refreshed, err := store.getPlanForUser(user.ID, plan.ID)
	if err != nil {
		c.JSON(http.StatusCreated, planResponseFromPlan(plan))
		return
	}
	c.JSON(http.StatusCreated, planResponseFromPlan(refreshed))
}

func listTasks(c *gin.Context) {
	user := currentUser(c)

	plans, err := store.listPlansByUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}

	response := make([]TaskResponse, 0, len(plans))
	for _, plan := range plans {
		response = append(response, planResponseFromPlan(plan))
	}
	c.JSON(http.StatusOK, response)
}

func getTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, planResponseFromPlan(plan))
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
	refreshed, _ := store.getPlanForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusOK, planResponseFromPlan(refreshed))
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
	refreshed, _ := store.getPlanForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusOK, planResponseFromPlan(refreshed))
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
	refreshed, _ := store.getPlanForUser(plan.UserID, plan.ID)
	c.JSON(http.StatusOK, planResponseFromPlan(refreshed))
}

func purgeTask(c *gin.Context) {
	plan, err := loadPlanForCurrentUser(c, c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	switch plan.Status {
	case PlanRunning, PlanPaused:
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

func loadPlanForCurrentUser(c *gin.Context, id string) (*PlanRecord, error) {
	user := currentUser(c)
	return store.getPlanForUser(user.ID, id)
}

func buildPlansFromRequest(userID int64, req CreateTaskRequest) (*PlanRecord, []*PlanRecord, error) {
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

	if err := normalizeAreaLevels(&req); err != nil {
		return nil, nil, err
	}

	if len(req.Levels) == 0 {
		return nil, nil, errors.New("at least one level configuration is required")
	}

	levels := make([]LevelConfig, 0, len(req.Levels))
	for _, level := range req.Levels {
		resolvedGeoJSON, err := validateLevel(level)
		if err != nil {
			return nil, nil, err
		}
		level.Geojson = resolvedGeoJSON
		levels = append(levels, LevelConfig(level))
	}

	groupID, _ := shortid.Generate()
	parent := &PlanRecord{
		ID:           groupID,
		UserID:       userID,
		Kind:         PlanKindGroup,
		Name:         req.Name,
		URL:          sources[0].URL,
		Format:       sources[0].Format,
		Schema:       sources[0].Schema,
		Workers:      firstPositive(req.Workers, viper.GetInt("task.workers")),
		SavePipe:     firstPositive(req.SavePipe, viper.GetInt("task.savepipe")),
		TimeDelay:    maxInt(req.TimeDelay, 0),
		ScheduleMode: mode,
		RunAt:        runAt,
		Status:       PlanScheduled,
		Levels:       levels,
	}

	children := make([]*PlanRecord, 0, len(sources))
	for _, source := range sources {
		id, _ := shortid.Generate()
		children = append(children, &PlanRecord{
			ID:           id,
			UserID:       userID,
			ParentID:     groupID,
			Kind:         PlanKindChild,
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
			Status:       PlanScheduled,
			Levels:       levels,
		})
	}

	return parent, children, nil
}

func normalizeAreaLevels(req *CreateTaskRequest) error {
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" && req.Area.BBox != nil {
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
		if req.Area.BBox == nil {
			return errors.New("area.bbox is required for bbox tasks")
		}
		if req.Zoom == nil {
			return errors.New("zoom is required for bbox tasks")
		}
		box := area.BBox{
			MinLon: req.Area.BBox.MinLon,
			MinLat: req.Area.BBox.MinLat,
			MaxLon: req.Area.BBox.MaxLon,
			MaxLat: req.Area.BBox.MaxLat,
		}
		zoom := area.ZoomRange{Min: req.Zoom.Min, Max: req.Zoom.Max}
		count, err := downloader.CountBBoxTiles(box, zoom)
		if err != nil {
			return err
		}
		if count == 0 {
			return errors.New("bbox task has no tiles")
		}
		geojsonPath, err := writeGeneratedBBoxGeoJSON(req.Name, box)
		if err != nil {
			return err
		}
		req.Levels = []LevelRequest{{
			MinZoom: zoom.Min,
			MaxZoom: zoom.Max,
			Geojson: geojsonPath,
		}}
		return nil
	default:
		return errors.New("mode must be bbox or region")
	}
}

func writeGeneratedBBoxGeoJSON(name string, box area.BBox) (string, error) {
	id, err := shortid.Generate()
	if err != nil {
		return "", err
	}
	dir := filepath.Join("data", "generated-areas")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", err
	}

	path := filepath.Join(dir, fmt.Sprintf("bbox-%s.geojson", id))
	payload := map[string]any{
		"type": "FeatureCollection",
		"features": []map[string]any{
			{
				"type": "Feature",
				"properties": map[string]any{
					"name": strings.TrimSpace(name),
					"mode": string(area.ModeBBox),
				},
				"geometry": map[string]any{
					"type": "Polygon",
					"coordinates": [][][]float64{{
						{box.MinLon, box.MinLat},
						{box.MaxLon, box.MinLat},
						{box.MaxLon, box.MaxLat},
						{box.MinLon, box.MaxLat},
						{box.MinLon, box.MinLat},
					}},
				},
			},
		},
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return "", err
	}
	return path, nil
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

func validateLevel(level LevelRequest) (string, error) {
	if level.MinZoom < ZoomMin || level.MaxZoom > ZoomMax {
		return "", errors.New("zoom level out of supported range")
	}
	if level.MinZoom > level.MaxZoom {
		return "", errors.New("minZoom cannot be greater than maxZoom")
	}
	path := strings.TrimSpace(level.Geojson)
	if path == "" {
		return "", errors.New("geojson is required")
	}

	return resolveGeoJSONPath(path)
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

func planResponseFromPlan(plan *PlanRecord) TaskResponse {
	response := TaskResponse{
		ID:             plan.ID,
		ParentID:       plan.ParentID,
		Kind:           string(plan.Kind),
		Name:           plan.Name,
		SourceName:     plan.SourceName,
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

	if plan.Kind == PlanKindGroup {
		applyGroupSummary(plan, &response)
		return response
	}

	if plan.LastRun != nil {
		run := plan.LastRun
		response.File = run.OutputPath
		response.Total = run.Total
		response.Current = run.Current
		if run.Total > 0 {
			response.Progress = float64(run.Current) / float64(run.Total)
		}
		response.Status = string(effectiveTaskStatusForResponse(plan, run))
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

	return response
}

func effectiveTaskStatusForResponse(plan *PlanRecord, run *TaskRunRecord) TaskStatus {
	if run == nil {
		switch plan.Status {
		case PlanPaused:
			return TaskPaused
		case PlanCancelled:
			return TaskCancelled
		case PlanFailed:
			return TaskFailed
		case PlanCompleted:
			return TaskCompleted
		default:
			return TaskStatus(plan.Status)
		}
	}

	switch plan.Status {
	case PlanPaused:
		if run.Status == TaskRunning || run.Status == TaskPending {
			return TaskPaused
		}
	case PlanCancelled:
		if run.Status == TaskRunning || run.Status == TaskPending || run.Status == TaskPaused {
			return TaskCancelled
		}
	case PlanFailed:
		if run.Status == TaskRunning || run.Status == TaskPending || run.Status == TaskPaused {
			return TaskFailed
		}
	}

	return run.Status
}

func applyGroupSummary(plan *PlanRecord, response *TaskResponse) {
	response.TotalChildren = len(plan.Children)
	response.Children = make([]TaskResponse, 0, len(plan.Children))

	if len(plan.Children) == 0 {
		return
	}

	var totalTiles int64
	var currentTiles int64

	for _, child := range plan.Children {
		childResponse := planResponseFromPlan(child)
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
	if totalTiles > 0 {
		response.Progress = float64(currentTiles) / float64(totalTiles)
	}

	switch {
	case response.CompletedChildren == response.TotalChildren:
		response.Status = string(PlanCompleted)
	case response.CancelledChildren == response.TotalChildren:
		response.Status = string(PlanCancelled)
	case response.FailedChildren == response.TotalChildren:
		response.Status = string(PlanFailed)
	case response.RunningChildren > 0:
		response.Status = string(PlanRunning)
	case response.PausedChildren > 0 && response.RunningChildren == 0:
		response.Status = string(PlanPaused)
	case response.CompletedChildren+response.FailedChildren+response.CancelledChildren == response.TotalChildren && response.FailedChildren > 0:
		response.Status = string(PlanPartialFailed)
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
