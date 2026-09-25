package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

func GetAllTaskFiles(kbCfg config.KanboardConfig, taskId int) ([]KbResponseTaskFile, error) {
	// https://docs.kanboard.org/v1/api/task_file_procedures/#getalltaskfiles
	var resp KbResponse[[]KbResponseTaskFile]
	err := WebClientRpcCall(kbCfg, newRequest("getAllTaskFiles", KbTaskIdParam{TaskId: taskId}), &resp)
	return resp.Result, err
}
