package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetTask returns nil (and no error) when the task does not exist.
func GetTask(kbCfg config.KanboardConfig, taskId int) (*KbResponseTask, error) {
	// https://docs.kanboard.org/v1/api/task_procedures/#gettask
	var resp KbResponse[*KbResponseTask]
	err := WebClientRpcCall(kbCfg, newRequest("getTask", KbTaskIdParam{TaskId: taskId}), &resp)
	return resp.Result, err
}
