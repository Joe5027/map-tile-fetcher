package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

const integrationSignatureWindow = 5 * time.Minute

type integrationTaskSummary struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Status string `json:"status"`
}

type integrationTaskListResponse struct {
	Tasks []integrationTaskSummary `json:"tasks"`
}

type integrationTaskResources struct {
	ElectronicMap string `json:"electronicMap"`
	SatelliteMap  string `json:"satelliteMap"`
	RoadNetwork   string `json:"roadNetwork"`
	Terrain       string `json:"terrain"`
}

type integrationTaskResourcesResponse struct {
	TaskID    string                   `json:"taskId"`
	TaskName  string                   `json:"taskName"`
	Resources integrationTaskResources `json:"resources"`
}

func integrationAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		clientID := strings.TrimSpace(c.GetHeader("X-Integration-Client"))
		timestamp := strings.TrimSpace(c.GetHeader("X-Integration-Timestamp"))
		signature := strings.TrimSpace(c.GetHeader("X-Integration-Signature"))
		if clientID == "" || timestamp == "" || signature == "" || !validIntegrationSignature(c.Request, clientID, timestamp, signature, time.Now()) {
			integrationUnauthorized(c)
			return
		}

		user, err := store.defaultUser()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load integration user"})
			c.Abort()
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func validIntegrationSignature(request *http.Request, clientID, timestamp, signature string, now time.Time) bool {
	configuredClient := strings.TrimSpace(viper.GetString("integration.client_id"))
	secret := strings.TrimSpace(viper.GetString("integration.secret"))
	if configuredClient == "" || secret == "" || !hmac.Equal([]byte(clientID), []byte(configuredClient)) {
		return false
	}
	unixSeconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil {
		return false
	}
	signedAt := time.Unix(unixSeconds, 0)
	if signedAt.Before(now.Add(-integrationSignatureWindow)) || signedAt.After(now.Add(integrationSignatureWindow)) {
		return false
	}
	provided, err := hex.DecodeString(signature)
	if err != nil || len(provided) != sha256.Size {
		return false
	}
	payload := request.Method + "\n" + request.URL.EscapedPath() + "\n" + timestamp
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return hmac.Equal(provided, mac.Sum(nil))
}

func integrationUnauthorized(c *gin.Context) {
	c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or expired integration signature"})
	c.Abort()
}

func listIntegrationTasks(c *gin.Context) {
	user := currentUser(c)
	plans, err := store.listTaskRecordsByUser(user.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load tasks"})
		return
	}

	tasks := make([]integrationTaskSummary, 0, len(plans))
	for _, plan := range plans {
		response := taskResponseFromRecord(plan)
		tasks = append(tasks, integrationTaskSummary{ID: plan.ID, Name: plan.Name, Status: response.Status})
	}
	c.JSON(http.StatusOK, integrationTaskListResponse{Tasks: tasks})
}

func getIntegrationTaskResources(c *gin.Context) {
	user := currentUser(c)
	plan, err := store.getTaskRecordForUser(user.ID, c.Param("id"))
	if err != nil || plan.ParentID != "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}

	resources := integrationTaskResources{}
	candidates := []*TaskRecord{plan}
	if plan.Kind == TaskRecordKindGroup {
		candidates = plan.Children
	}
	for _, candidate := range candidates {
		resourceType := integrationResourceType(candidate)
		if resourceType == "" || integrationResourceAlreadySet(resources, resourceType) {
			continue
		}
		run, err := store.publishedRun(candidate.ID)
		if err != nil || run.ArtifactStatus != ArtifactReady || strings.TrimSpace(run.ArtifactPath) == "" {
			continue
		}
		path := artifactDownloadURL(candidate.ID, run)
		if path == "" || !strings.HasPrefix(path, "/api/downloads/") {
			continue
		}
		setIntegrationResource(&resources, resourceType, integrationAbsoluteURL(c, path))
	}

	c.JSON(http.StatusOK, integrationTaskResourcesResponse{
		TaskID: plan.ID, TaskName: plan.Name, Resources: resources,
	})
}

func integrationResourceType(plan *TaskRecord) string {
	if plan == nil {
		return ""
	}
	if isTerrainPlan(plan) {
		return "terrain"
	}
	for _, level := range plan.Levels {
		if level.Source == nil {
			continue
		}
		kind := strings.ToLower(strings.TrimSpace(level.Source.Kind))
		encoding := strings.ToLower(strings.TrimSpace(level.Source.Encoding))
		if kind == "elevation" || encoding == "terrarium" {
			return "terrain"
		}
	}

	nameAndURL := strings.ToLower(strings.TrimSpace(plan.SourceName + " " + plan.Name + " " + plan.URL))
	if strings.Contains(nameAndURL, "cta") {
		return ""
	}
	switch inferLayerName(plan.SourceName+" "+plan.Name, plan.URL) {
	case "vec":
		return "electronicMap"
	case "img":
		return "satelliteMap"
	case "cia":
		return "roadNetwork"
	}
	switch {
	case containsAny(nameAndURL, "电子图", "电子地图", "矢量", "vector"):
		return "electronicMap"
	case containsAny(nameAndURL, "卫星", "影像", "satellite", "imagery"):
		return "satelliteMap"
	case containsAny(nameAndURL, "路网", "注记", "道路", "road network", "annotation"):
		return "roadNetwork"
	case containsAny(nameAndURL, "高程", "terrarium", "elevation", "cesium"):
		return "terrain"
	default:
		return ""
	}
}

func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}

func integrationResourceAlreadySet(resources integrationTaskResources, resourceType string) bool {
	switch resourceType {
	case "electronicMap":
		return resources.ElectronicMap != ""
	case "satelliteMap":
		return resources.SatelliteMap != ""
	case "roadNetwork":
		return resources.RoadNetwork != ""
	case "terrain":
		return resources.Terrain != ""
	default:
		return true
	}
}

func setIntegrationResource(resources *integrationTaskResources, resourceType, value string) {
	switch resourceType {
	case "electronicMap":
		resources.ElectronicMap = value
	case "satelliteMap":
		resources.SatelliteMap = value
	case "roadNetwork":
		resources.RoadNetwork = value
	case "terrain":
		resources.Terrain = value
	}
}

func integrationAbsoluteURL(c *gin.Context, path string) string {
	scheme := "http"
	if c.Request.TLS != nil {
		scheme = "https"
	}
	if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Proto"), ",")[0]); forwarded != "" {
		scheme = forwarded
	}
	host := strings.TrimSpace(c.Request.Host)
	if forwarded := strings.TrimSpace(strings.Split(c.GetHeader("X-Forwarded-Host"), ",")[0]); forwarded != "" {
		host = forwarded
	}
	return scheme + "://" + host + path
}
