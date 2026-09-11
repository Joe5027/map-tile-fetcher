package main

import (
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/term"
)

func runAdmin(args []string, readPassword func() ([]byte, error), output io.Writer) error {
	if len(args) == 0 || args[0] != "reset-password" {
		return errors.New("usage: tiler admin reset-password --database PATH --username NAME")
	}
	flags := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	flags.SetOutput(io.Discard)
	database := flags.String("database", "", "existing SQLite database")
	username := flags.String("username", "", "existing username")
	if err := flags.Parse(args[1:]); err != nil || flags.NArg() != 0 || *database == "" || *username == "" {
		return errors.New("expected --database PATH --username NAME; plaintext password arguments are not accepted")
	}
	path, err := filepath.Abs(*database)
	if err != nil {
		return err
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() {
		return errors.New("database must be an existing regular file")
	}
	path, err = filepath.EvalSymlinks(path)
	if err != nil {
		return err
	}
	lock, err := acquireMaintenanceLock(path, true)
	if err != nil {
		return err
	}
	defer lock.Close()
	uriPath := filepath.ToSlash(path)
	if filepath.VolumeName(path) != "" {
		uriPath = "/" + uriPath
	}
	uri := &url.URL{Scheme: "file", Path: uriPath, RawQuery: "mode=rw&_pragma=busy_timeout(1000)&_txlock=immediate"}
	db, err := sql.Open("sqlite", uri.String())
	if err != nil {
		return errors.New("could not open existing database")
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	local := &SQLiteStore{db: db}
	user, err := scanUser(db.QueryRow(`SELECT id, username, password_hash, created_at FROM users WHERE username=?`, *username))
	if err != nil {
		return errors.New("existing user could not be loaded; database was not migrated")
	}
	fmt.Fprint(output, "New password: ")
	first, err := readPassword()
	if err != nil {
		return errors.New("hidden terminal password input failed")
	}
	defer clear(first)
	fmt.Fprint(output, "\nConfirm password: ")
	second, err := readPassword()
	if err != nil {
		return errors.New("hidden terminal password input failed")
	}
	defer clear(second)
	fmt.Fprintln(output)
	if string(first) != string(second) {
		return errors.New("password confirmation does not match")
	}
	if err := validateNewPassword(string(first)); err != nil {
		return err
	}
	same, _, err := passwordMatches(user.PasswordHash, string(first))
	if err != nil {
		return errors.New("existing password hash cannot be verified")
	}
	if same {
		return errors.New("new password must differ from old password")
	}
	hash, err := passwordHash(string(first))
	if err != nil {
		return errors.New("could not hash new password")
	}
	backup := path + ".backup-" + time.Now().UTC().Format("20060102T150405.000000000") + ".db"
	backupFile, err := os.OpenFile(backup, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err != nil {
		return errors.New("cannot reserve private backup; password unchanged")
	}
	if err = backupFile.Close(); err != nil {
		return errors.New("cannot close backup; password unchanged")
	}
	// VACUUM INTO includes committed WAL content and gives a consistent standalone copy.
	if _, err = db.Exec(`VACUUM INTO ?`, backup); err != nil {
		return errors.New("consistent backup failed; password unchanged")
	}
	if err = os.Chmod(backup, 0600); err != nil {
		return errors.New("backup permissions failed; password unchanged")
	}
	if err = local.replacePassword(user.ID, user.PasswordHash, hash); err != nil {
		return errors.New("password transaction failed; password unchanged; backup retained")
	}
	fmt.Fprintf(output, "Password reset; sessions revoked. Backup: %s\n", backup)
	return nil
}

func hiddenPassword() ([]byte, error) {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return nil, errors.New("interactive terminal required")
	}
	return term.ReadPassword(int(os.Stdin.Fd()))
}
