package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// OpenTask reopens a closed task; opening an open task also succeeds.
func OpenTask(kbCfg config.KanboardConfig, taskId int) error {
	// https://docs.kanboard.org/v1/api/task_procedures/#opentask
	return writeOk(kbCfg, "openTask", KbTaskIdParam{TaskId: taskId})
}
