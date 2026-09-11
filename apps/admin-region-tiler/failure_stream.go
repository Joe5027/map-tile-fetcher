package main

import (
	"fmt"
	"github.com/paulmach/orb/maptile"
)

func (s *SQLiteStore) walkRetryFailures(planID string, visit func(TileJob) error) error {
	for offset := 0; ; offset += 256 {
		rows, err := s.db.Query(`SELECT z,x,y,url FROM failures WHERE source_id=? AND resolved_at=0 AND retryable=1 GROUP BY z,x,y,url ORDER BY z,x,y,url LIMIT 256 OFFSET ?`, planID, offset)
		if err != nil {
			return err
		}
		batch := make([]TileJob, 0, 256)
		for rows.Next() {
			var z, x, y int
			var url string
			if err := rows.Scan(&z, &x, &y, &url); err != nil {
				rows.Close()
				return err
			}
			batch = append(batch, TileJob{Tile: maptile.New(uint32(x), uint32(y), maptile.Zoom(z)), URL: url})
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return err
		}
		rows.Close()
		for _, job := range batch {
			if err := visit(job); err != nil {
				return err
			}
		}
		if len(batch) < 256 {
			return nil
		}
	}
}

func (s *SQLiteStore) failureSink(plan *TaskRecord, runID string) func(TileFailureRecord) error {
	index := 0
	taskID := plan.ParentID
	if taskID == "" {
		taskID = plan.ID
	}
	return func(record TileFailureRecord) error {
		index++
		_, err := s.db.Exec(`INSERT INTO failures(id,task_id,run_id,source_id,z,x,y,url,error_message,retryable,attempt,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)`, fmt.Sprintf("%s:event:%d", runID, index), taskID, runID, plan.ID, record.Z, record.X, record.Y, record.URL, record.ErrorMessage, record.Retryable, record.Attempt, record.CreatedAt.Unix())
		return err
	}
}
