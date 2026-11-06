package main

func prepareDerivedArtifacts(plan *TaskRecord, task *Task, run *TaskRunRecord) error {
	if task.deriveArtifacts == nil {
		return nil
	}
	return task.deriveArtifacts(plan, task, run)
}
