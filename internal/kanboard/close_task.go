package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// CloseTask closes a task; closing a closed task also succeeds.
func CloseTask(kbCfg config.KanboardConfig, taskId int) error {
	// https://docs.kanboard.org/v1/api/task_procedures/#closetask
	return writeOk(kbCfg, "closeTask", KbTaskIdParam{TaskId: taskId})
}
