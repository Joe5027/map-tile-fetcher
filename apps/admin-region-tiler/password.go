package main

import (
	"errors"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/gin-gonic/gin"
)

var errPasswordConflict = errors.New("password changed concurrently")

func validateNewPassword(password string) error {
	if !utf8.ValidString(password) || utf8.RuneCountInString(password) < 12 || len(password) > 72 {
		return errors.New("new password must contain at least 12 characters and at most 72 UTF-8 bytes")
	}
	return nil
}

func (s *SQLiteStore) replacePassword(userID int64, expectedHash, newHash string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE users SET password_hash=? WHERE id=? AND password_hash=?`, newHash, userID, expectedHash)
	if err != nil {
		return err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if n != 1 {
		return errPasswordConflict
	}
	if _, err = tx.Exec(`DELETE FROM sessions WHERE user_id=?`, userID); err != nil {
		return err
	}
	// Old databases need no schema migration for offline recovery.
	var attemptsTable int
	if err = tx.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='password_attempts'`).Scan(&attemptsTable); err != nil {
		return err
	}
	if attemptsTable != 0 {
		if _, err = tx.Exec(`DELETE FROM password_attempts WHERE user_id=?`, userID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *SQLiteStore) reservePasswordAttempt(userID int64) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().Unix()
	if _, err = tx.Exec(`DELETE FROM password_attempts WHERE attempted_at <= ?`, now-900); err != nil {
		return false, err
	}
	result, err := tx.Exec(`INSERT INTO password_attempts(user_id, attempted_at)
        SELECT ?, ? WHERE (SELECT COUNT(*) FROM password_attempts WHERE user_id=?) < 5`, userID, now, userID)
	if err != nil {
		return false, err
	}
	n, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return n == 1, tx.Commit()
}

func passwordHandler(c *gin.Context) {
	if !authEnabled() {
		c.JSON(403, gin.H{"error": "免登录模式不能修改密码", "code": "auth_disabled"})
		return
	}
	var req struct {
		CurrentPassword string `json:"currentPassword"`
		NewPassword     string `json:"newPassword"`
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 4096)
	if err := c.ShouldBindJSON(&req); err != nil || req.CurrentPassword == "" {
		c.JSON(400, gin.H{"error": "密码请求无效"})
		return
	}
	if err := validateNewPassword(req.NewPassword); err != nil {
		c.JSON(400, gin.H{"error": "新密码至少 12 个字符且 UTF-8 编码不超过 72 字节"})
		return
	}
	if req.CurrentPassword == req.NewPassword {
		c.JSON(400, gin.H{"error": "新密码不能与原密码相同"})
		return
	}
	user := currentUser(c)
	allowed, err := store.reservePasswordAttempt(user.ID)
	if err != nil {
		c.JSON(500, gin.H{"error": "无法检查密码尝试次数"})
		return
	}
	if !allowed {
		c.Header("Retry-After", "900")
		c.JSON(403, gin.H{"error": "密码尝试次数过多，请 15 分钟后重试", "code": "password_attempts_exceeded"})
		return
	}
	matches, _, err := passwordMatches(user.PasswordHash, req.CurrentPassword)
	if err != nil || !matches {
		c.JSON(403, gin.H{"error": "原密码错误"})
		return
	}
	hash, err := passwordHash(req.NewPassword)
	if err == nil {
		err = store.replacePassword(user.ID, user.PasswordHash, hash)
	}
	if errors.Is(err, errPasswordConflict) {
		c.JSON(409, gin.H{"error": "密码已变更，请重新登录"})
		return
	}
	if err != nil {
		c.JSON(500, gin.H{"error": "修改密码失败，原密码仍然有效"})
		return
	}
	setSessionCookie(c, "", -1)
	c.Status(204)
}
