package main

import (
	"fmt"
	"os"
	"strings"
)

// The maintenance lock remains shared with workers. Only the server owns the
// scheduler and recovery state, so claim that separately before any migration.
func acquireServerOwnership(maintenance *os.File) (*os.File, error) {
	path := strings.TrimSuffix(maintenance.Name(), ".maintenance.lock") + ".server.lock"
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err := lockMaintenanceFile(file, true); err != nil {
		file.Close()
		return nil, fmt.Errorf("database is already owned by another server: %w", err)
	}
	return file, nil
}
