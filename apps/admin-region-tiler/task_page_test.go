package main

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func seedTaskPageHistory(t *testing.T, s *SQLiteStore, count int) {
	t.Helper()
	if err := s.initRunPhaseSchema(); err != nil {
		t.Fatal(err)
	}
	tx, err := s.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	exec := func(query string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(query, args...); err != nil {
			t.Fatal(err)
		}
	}
	statuses := []string{"completed", "running", "paused", "failed", "partial_failed", "queued", "cancelled"}
	for i := 0; i < count; i++ {
		parent := fmt.Sprintf("page-%04d", i)
		for c := -1; c < 3; c++ {
			id, parentID, kind, status, runID := parent, "", "group", "scheduled", ""
			if c >= 0 {
				id = fmt.Sprintf("%s-c%d", parent, c)
				parentID = parent
				kind = "child"
				status = statuses[i%len(statuses)]
				runID = id + "-latest"
			}
			exec(`INSERT INTO plans(id,user_id,parent_id,kind,name,source_name,url,format,schema,workers,save_pipe,time_delay,schedule_mode,run_at,status,levels_json,last_run_id,created_at,updated_at)
				VALUES(?,1,?,?,?,?,?,'png','xyz',3,1,0,'immediate',0,?,'[]',?,?,?)`, id, parentID, kind, id, id, "http://fixture.test/{z}/{x}/{y}", status, runID, i+1, i+1)
			if c < 0 {
				continue
			}
			runStatus := status
			if status == "queued" {
				runStatus = "paused"
			}
			exec(`INSERT INTO task_runs(id,plan_id,user_id,status,trigger_mode,total,current,success_count,failure_count,created_at,updated_at)
				VALUES(?,?,1,?,'initial',10,8,7,1,2,2)`, runID, id, runStatus)
			exec(`INSERT INTO task_runs(id,plan_id,user_id,status,trigger_mode,artifact_path,artifact_name,artifact_status,total,current,success_count,created_at,updated_at)
				VALUES(?,?,1,'completed','initial',?,'fixture.zip','ready',7,7,7,1,1)`, id+"-published", id, "/fixture/"+id+".zip")
			exec(`INSERT INTO task_integrity(plan_id,status,expected,available,missing,error_message,run_id) VALUES(?,'incomplete',10,7,3,'',?)`, id, runID)
			exec(`INSERT INTO run_phases(run_id,phase,current,total) VALUES(?,'downloading',8,10)`, runID)
			for duplicate := 0; duplicate < 2; duplicate++ {
				exec(`INSERT INTO failures(id,task_id,run_id,source_id,z,x,y,url,retryable,created_at) VALUES(?,?,?,?,1,0,0,'http://fixture.test/tile',1,1)`, fmt.Sprintf("%s-f%d", id, duplicate), parent, runID, id)
			}
			if status == "queued" {
				exec(`INSERT INTO execution_queue(plan_id,trigger_mode,due_at,enqueued_at,state) VALUES(?,'resume',0,?,'queued')`, id, i+1)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func TestTaskPageMatchesLegacyAndBoundsWork(t *testing.T) {
	s := useTestStore(t)
	seedTaskPageHistory(t, s, 1000)
	start := time.Now()
	legacyRecords, err := s.listTaskRecordsByUser(1)
	if err != nil {
		t.Fatal(err)
	}
	legacy := make([]TaskResponse, 0, len(legacyRecords))
	byID := map[string]TaskResponse{}
	counts := map[string]int{"all": 1000}
	for _, record := range legacyRecords {
		response := taskResponseFromRecord(record)
		legacy = append(legacy, response)
		byID[record.ID] = response
		counts[response.Status]++
	}
	legacyBytes, _ := json.Marshal(legacy)
	legacyElapsed := time.Since(start)
	start = time.Now()
	page, err := s.loadTaskPage(1, 50, 0, "all")
	if err != nil {
		t.Fatal(err)
	}
	pageBytes, _ := json.Marshal(page)
	elapsed := time.Since(start)
	if len(page.Items) != 50 || page.Total != 1000 || page.queryCount != 8 {
		t.Fatalf("unexpected page dimensions: items=%d total=%d queries=%d", len(page.Items), page.Total, page.queryCount)
	}
	if !reflect.DeepEqual(counts, page.Counts) {
		t.Fatalf("counts differ: legacy=%v page=%v", counts, page.Counts)
	}
	for _, item := range page.Items {
		if !reflect.DeepEqual(item, byID[item.ID]) {
			t.Fatalf("page differs from legacy for %s\npage=%+v\nlegacy=%+v", item.ID, item, byID[item.ID])
		}
	}
	if len(pageBytes) >= len(legacyBytes)/10 {
		t.Fatalf("page body unexpectedly large: %d vs %d", len(pageBytes), len(legacyBytes))
	}
	t.Logf("1000 parents x 3 children: legacy=%s/%d bytes; page=%s/%d bytes; page parents=%d children=%d batch_queries=%d", legacyElapsed, len(legacyBytes), elapsed, len(pageBytes), len(page.Items), len(page.Items)*3, page.queryCount)
	// A nil global store proves response rendering consumes the prefetched view.
	store = nil
	defer func() { store = s }()
	second, err := s.loadTaskPage(1, 50, 50, "all")
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Items) != 50 || second.Items[0].ID == page.Items[0].ID {
		t.Fatal("second page did not advance")
	}
	for _, item := range second.Items {
		if !reflect.DeepEqual(item, byID[item.ID]) {
			t.Fatalf("prefetched response without global store differs: %s", item.ID)
		}
	}
	filtered, err := s.loadTaskPage(1, 50, 0, "failed")
	if err != nil {
		t.Fatal(err)
	}
	if filtered.Total != counts["failed"]+counts["partial_failed"] {
		t.Fatalf("wrong filtered total: %d", filtered.Total)
	}
	for _, item := range filtered.Items {
		if !taskPageStatusMatches(item.Status, "failed") {
			t.Fatalf("unexpected filtered status %s", item.Status)
		}
	}
	empty, err := s.loadTaskPage(1, 50, 2000, "all")
	if err != nil || len(empty.Items) != 0 || empty.Total != 1000 || empty.queryCount != 1 {
		t.Fatalf("empty page: %+v %v", empty, err)
	}
	other, err := s.loadTaskPage(2, 50, 0, "all")
	if err != nil || other.Total != 0 || len(other.Items) != 0 {
		t.Fatalf("other user's page leaked tasks: %+v %v", other, err)
	}
}

func TestTaskPageHTTPValidation(t *testing.T) {
	s := useTestStore(t)
	seedTaskPageHistory(t, s, 2)
	r := gin.New()
	r.Use(func(c *gin.Context) { c.Set("user", &UserRecord{ID: 1}) })
	r.GET("/api/tasks/page", listTaskPage)
	r.GET("/api/tasks", listTasks)
	for _, path := range []string{"/api/tasks/page?limit=0", "/api/tasks/page?limit=101", "/api/tasks/page?offset=-1", "/api/tasks/page?status=unknown"} {
		w := httptest.NewRecorder()
		r.ServeHTTP(w, httptest.NewRequest("GET", path, nil))
		if w.Code != 400 {
			t.Fatalf("%s returned %d", path, w.Code)
		}
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks/page?limit=1", nil))
	var page taskPageResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &page) != nil || len(page.Items) != 1 || page.Total != 2 {
		t.Fatalf("unexpected page response: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	r.ServeHTTP(w, httptest.NewRequest("GET", "/api/tasks", nil))
	var old []TaskResponse
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &old) != nil || len(old) != 2 {
		t.Fatalf("legacy list changed: %d %s", w.Code, w.Body.String())
	}
}

func TestArtifactGrantSweepDoesNotExtendExpiredLink(t *testing.T) {
	run := &TaskRunRecord{ID: "sweep-run", ArtifactStatus: ArtifactReady, ArtifactPath: "/fixture.zip", ArtifactName: "fixture.zip"}
	first := artifactDownloadURL("sweep-task", run)
	token := first[len("/api/downloads/"):]
	artifactDownloadGrants.Lock()
	grant := artifactDownloadGrants.values[token]
	grant.ExpiresAt = time.Now().Add(-time.Second)
	artifactDownloadGrants.values[token] = grant
	artifactDownloadGrantLastSweep = time.Now()
	artifactDownloadGrants.Unlock()
	second := artifactDownloadURL("sweep-task", run)
	if first == second {
		t.Fatal("expired cached grant reused between sweeps")
	}
	if _, ok := artifactDownloadGrantForToken(token, time.Now()); ok {
		t.Fatal("expired link still accepted")
	}
}
