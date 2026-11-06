package main

// RunPhase describes observable work without changing the existing task status contract.
type RunPhase struct {
	Name    string `json:"name"`
	Current int64  `json:"current"`
	Total   int64  `json:"total"`
}

func (s *SQLiteStore) initRunPhaseSchema() error {
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS run_phases(run_id TEXT PRIMARY KEY,phase TEXT NOT NULL,current INTEGER NOT NULL DEFAULT 0,total INTEGER NOT NULL DEFAULT 0)`)
	return err
}

func (s *SQLiteStore) setRunPhase(runID, phase string, current, total int64) error {
	_, err := s.db.Exec(`INSERT INTO run_phases(run_id,phase,current,total) VALUES(?,?,?,?) ON CONFLICT(run_id) DO UPDATE SET phase=excluded.phase,current=excluded.current,total=excluded.total`, runID, phase, current, total)
	return err
}

func (s *SQLiteStore) runPhase(runID string) RunPhase {
	var phase RunPhase
	_ = s.db.QueryRow(`SELECT phase,current,total FROM run_phases WHERE run_id=?`, runID).Scan(&phase.Name, &phase.Current, &phase.Total)
	return phase
}
