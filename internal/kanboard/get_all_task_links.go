package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetAllTaskLinks returns the internal (task-to-task) links of a task.
func GetAllTaskLinks(kbCfg config.KanboardConfig, taskId int) ([]KbResponseTaskLink, error) {
	// https://docs.kanboard.org/v1/api/internal_task_link_procedures/#getalltasklinks
	var resp KbResponse[[]KbResponseTaskLink]
	err := WebClientRpcCall(kbCfg, newRequest("getAllTaskLinks", KbTaskIdParam{TaskId: taskId}), &resp)
	return resp.Result, err
}
