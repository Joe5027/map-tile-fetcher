package main

import (
	"bytes"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/spf13/viper"
)

func TestPasswordRulesPreserveWhitespaceAndUnicode(t *testing.T) {
	for _, input := range []string{"short", strings.Repeat("a", 73), strings.Repeat("界", 25), string([]byte{255})} {
		if validateNewPassword(input) == nil {
			t.Fatal("accepted invalid password")
		}
	}
	for _, input := range []string{" 1234567890 ", strings.Repeat("界", 24), strings.Repeat("a", 72)} {
		if err := validateNewPassword(input); err != nil {
			t.Fatal(err)
		}
	}
}

func TestPasswordAtomicRollbackAndConcurrentSessionCreation(t *testing.T) {
	s := newSQLiteTestStore(t)
	s.db.SetMaxOpenConns(1)
	hash, _ := passwordHash("old-password-value")
	replacement, _ := passwordHash("new-password-value")
	_, err := s.db.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(1,'u',?,0),(2,'v',?,0)`, hash, hash)
	if err != nil {
		t.Fatal(err)
	}
	user, _ := s.getUserByID(1)
	other, _ := s.getUserByID(2)
	otherSession, err := s.createSession(other)
	if err != nil {
		t.Fatal(err)
	}
	oldSession, err := s.createSession(user)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_revoke BEFORE DELETE ON sessions BEGIN SELECT RAISE(ABORT,'fixture'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.replacePassword(1, hash, replacement); err == nil {
		t.Fatal("expected transaction failure")
	}
	unchanged, _ := s.getUserByID(1)
	if unchanged.PasswordHash != hash {
		t.Fatal("failed transaction changed password")
	}
	if _, err := s.getSession(oldSession.Token); err != nil {
		t.Fatal("failed transaction revoked session")
	}
	if _, err := s.db.Exec(`DROP TRIGGER fail_revoke`); err != nil {
		t.Fatal(err)
	}
	var group sync.WaitGroup
	for i := 0; i < 20; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			_, err := s.createSession(user)
			if err != nil && !errors.Is(err, errPasswordConflict) {
				t.Error(err)
			}
		}()
	}
	if err := s.replacePassword(1, hash, replacement); err != nil {
		t.Fatal(err)
	}
	group.Wait()
	if _, err := s.createSession(user); !errors.Is(err, errPasswordConflict) {
		t.Fatal("stale login issued session", err)
	}
	if err := s.replacePassword(1, hash, replacement); !errors.Is(err, errPasswordConflict) {
		t.Fatal("stale update accepted", err)
	}
	var count int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE user_id=1`).Scan(&count); err != nil || count != 0 {
		t.Fatal("old sessions remain", count, err)
	}
	if _, err := s.getSession(otherSession.Token); err != nil {
		t.Fatal("other user affected", err)
	}
}

func TestPasswordAttemptLimitPersistsAndExpires(t *testing.T) {
	s := newSQLiteTestStore(t)
	for i := 0; i < 7; i++ {
		allowed, err := s.reservePasswordAttempt(1)
		if err != nil || allowed != (i < 5) {
			t.Fatal(i, allowed, err)
		}
	}
	if allowed, err := s.reservePasswordAttempt(2); err != nil || !allowed {
		t.Fatal("other user throttled", err)
	}
	if _, err := s.db.Exec(`UPDATE password_attempts SET attempted_at=0 WHERE user_id=1`); err != nil {
		t.Fatal(err)
	}
	if allowed, err := s.reservePasswordAttempt(1); err != nil || !allowed {
		t.Fatal("expired attempts retained", err)
	}
}

func TestOfflinePasswordRecoveryLockBackupAndTaskPreservation(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	s := &SQLiteStore{db: db}
	if err = s.initSchema(); err != nil {
		t.Fatal(err)
	}
	hash, _ := passwordHash("original-password")
	if _, err = db.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(1,'u',?,0)`, hash); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`CREATE TABLE recovery_sentinel(value TEXT); INSERT INTO recovery_sentinel VALUES('running task unchanged')`); err != nil {
		t.Fatal(err)
	}
	if _, err = s.createSession(&UserRecord{ID: 1, PasswordHash: hash}); err != nil {
		t.Fatal(err)
	}
	db.Close()
	read := func() ([]byte, error) { return []byte("replacement-password"), nil }
	args := []string{"reset-password", "--database", path, "--username", "u"}
	var output bytes.Buffer
	first, err := acquireMaintenanceLock(path, false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := acquireMaintenanceLock(path, false)
	if err != nil {
		t.Fatal(err)
	}
	if err = runAdmin(args, read, &output); err == nil {
		t.Fatal("recovered active database")
	}
	first.Close()
	if err = runAdmin(args, read, &output); err == nil {
		t.Fatal("ignored remaining worker")
	}
	second.Close()
	if err = runAdmin(args, read, &output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "replacement-password") || strings.Contains(output.String(), hash) {
		t.Fatal("secret in output")
	}
	backups, _ := filepath.Glob(path + ".backup-*.db")
	if len(backups) != 1 {
		t.Fatal("missing consistent backup", backups)
	}
	for _, entry := range []struct {
		path, password string
		sessions       int
	}{{path, "replacement-password", 0}, {backups[0], "original-password", 1}} {
		opened, err := sql.Open("sqlite", entry.path)
		if err != nil {
			t.Fatal(err)
		}
		func() {
			defer opened.Close()
			local := &SQLiteStore{db: opened}
			if _, err := local.authenticateUser("u", entry.password); err != nil {
				t.Fatal(err)
			}
			var value string
			var sessions int
			if err := opened.QueryRow(`SELECT value FROM recovery_sentinel`).Scan(&value); err != nil || value != "running task unchanged" {
				t.Fatal("task changed", err)
			}
			if err := opened.QueryRow(`SELECT COUNT(*) FROM sessions`).Scan(&sessions); err != nil || sessions != entry.sessions {
				t.Fatal("session mismatch", sessions, err)
			}
		}()
	}
	missing := filepath.Join(root, "absent.db")
	if err := runAdmin([]string{"reset-password", "--database", missing, "--username", "u"}, read, &output); err == nil {
		t.Fatal("created new database")
	}
	if _, err := os.Stat(missing); !os.IsNotExist(err) {
		t.Fatal("missing database created")
	}
	if err := runAdmin(append(args, "--password", "secret"), read, &output); err == nil {
		t.Fatal("plaintext flag accepted")
	}
	if err := runAdmin([]string{"reset-password", "--database", path, "--username", "absent"}, read, &output); err == nil {
		t.Fatal("unknown user accepted")
	}
}

func TestPasswordHTTPValidationAndAuthDisabled(t *testing.T) {
	old := store
	store = newSQLiteTestStore(t)
	t.Cleanup(func() { store = old; viper.Reset() })
	viper.Set("auth.enabled", true)
	hash, _ := passwordHash("original-password")
	if _, err := store.db.Exec(`INSERT INTO users(id,username,password_hash,created_at) VALUES(1,'admin',?,0)`, hash); err != nil {
		t.Fatal(err)
	}
	user, _ := store.getUserByID(1)
	session, _ := store.createSession(user)
	router := gin.New()
	router.POST("/", authMiddleware(), passwordHandler)
	call := func(body string, authenticated bool) int {
		r := httptest.NewRequest("POST", "/", strings.NewReader(body))
		if authenticated {
			r.AddCookie(&http.Cookie{Name: sessionCookie, Value: session.Token})
		}
		w := httptest.NewRecorder()
		router.ServeHTTP(w, r)
		return w.Code
	}
	valid := `{"currentPassword":"original-password","newPassword":"another-password"}`
	if status := call(valid, false); status != 401 {
		t.Fatal(status)
	}
	for _, body := range []string{`{}`, `{"currentPassword":"original-password","newPassword":"short"}`, fmt.Sprintf(`{"currentPassword":"%s","newPassword":"another-password"}`, strings.Repeat("a", 5000))} {
		if status := call(body, true); status != 400 {
			t.Fatal(status)
		}
	}
	for i := 0; i < 5; i++ {
		if status := call(`{"currentPassword":"incorrect","newPassword":"another-password"}`, true); status != 403 {
			t.Fatal(status)
		}
	}
	if status := call(valid, true); status != 403 {
		t.Fatal("attempt cap ignored", status)
	}
	viper.Set("auth.enabled", false)
	viper.Set("auth.default_username", "admin")
	if status := call(valid, false); status != 403 {
		t.Fatal("auth-disabled mode accepted change", status)
	}
}

func TestPasswordChangeRevokesAllSessions(t *testing.T) {
	oldStore := store
	store = newSQLiteTestStore(t)
	t.Cleanup(func() { store = oldStore; viper.Reset() })
	viper.Set("auth.enabled", true)
	viper.Set("auth.default_username", "admin")
	viper.Set("auth.default_password", "adminmap")
	if err := store.seedDefaultUser(); err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	router.POST("/login", loginHandler)
	router.POST("/api/auth/password", authMiddleware(), passwordHandler)
	router.GET("/me", authMiddleware(), meHandler)
	login := func() *http.Cookie {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest("POST", "/login", strings.NewReader(`{"username":"admin","password":"adminmap"}`)))
		if w.Code != 200 {
			t.Fatal(w.Code, w.Body.String())
		}
		return w.Result().Cookies()[0]
	}
	cookies := []*http.Cookie{login(), login()}
	req := httptest.NewRequest("POST", "/api/auth/password", strings.NewReader(`{"currentPassword":"adminmap","newPassword":"new secure password"}`))
	req.AddCookie(cookies[0])
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != 204 {
		t.Fatal(w.Code, w.Body.String())
	}
	for _, cookie := range cookies {
		req = httptest.NewRequest("GET", "/me", nil)
		req.AddCookie(cookie)
		w = httptest.NewRecorder()
		router.ServeHTTP(w, req)
		if w.Code != 401 {
			t.Fatalf("old session survived: %d", w.Code)
		}
	}
	if _, err := store.authenticateUser("admin", "adminmap"); err == nil {
		t.Fatal("old password survived")
	}
	if _, err := store.authenticateUser("admin", "new secure password"); err != nil {
		t.Fatal(err)
	}
}
