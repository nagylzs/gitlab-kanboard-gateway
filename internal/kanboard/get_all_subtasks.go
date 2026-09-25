package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

func GetAllSubtasks(kbCfg config.KanboardConfig, taskId int) ([]KbResponseSubtask, error) {
	// https://docs.kanboard.org/v1/api/subtask_procedures/#getallsubtasks
	var resp KbResponse[[]KbResponseSubtask]
	err := WebClientRpcCall(kbCfg, newRequest("getAllSubtasks", KbTaskIdParam{TaskId: taskId}), &resp)
	return resp.Result, err
}
