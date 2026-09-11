package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func acquireMaintenanceLock(database string, exclusive bool) (*os.File, error) {
	absolute, err := filepath.Abs(database)
	if err != nil {
		return nil, err
	}
	if _, err = os.Stat(absolute); err == nil {
		absolute, err = filepath.EvalSymlinks(absolute)
	} else if os.IsNotExist(err) && !exclusive {
		var parent string
		parent, err = filepath.EvalSymlinks(filepath.Dir(absolute))
		absolute = filepath.Join(parent, filepath.Base(absolute))
	}
	if err != nil {
		return nil, fmt.Errorf("existing database/path required: %w", err)
	}
	file, err := os.OpenFile(absolute+".maintenance.lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	if err = lockMaintenanceFile(file, exclusive); err != nil {
		file.Close()
		return nil, fmt.Errorf("database is in use or locked for maintenance: %w", err)
	}
	// Never remove the sidecar: removing it would allow a second lock inode.
	return file, nil
}
