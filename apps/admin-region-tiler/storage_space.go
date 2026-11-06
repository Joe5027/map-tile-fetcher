package main

import (
	"fmt"

	"github.com/spf13/viper"
)

// Check only the additional bytes required by this phase. Existing outputs are
// already reflected in free space; there is no unconditional 1 GiB surcharge.
func requireOutputSpace(directory, phase string, additional uint64) error {
	free, err := availableDiskBytes(directory)
	if err != nil {
		return err
	}
	if reserve := viper.GetInt64("task.min_free_disk_bytes"); reserve > 0 {
		additional += uint64(reserve)
	}
	if free < additional {
		return fmt.Errorf("insufficient disk space for %s: need %d additional bytes, available %d", phase, additional, free)
	}
	return nil
}
