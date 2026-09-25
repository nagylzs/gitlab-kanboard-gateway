package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

func GetAllExternalTaskLinks(kbCfg config.KanboardConfig, taskId int) ([]KbResponseExternalTaskLink, error) {
	// https://docs.kanboard.org/v1/api/external_task_link_procedures/#getallexternaltasklinks
	var resp KbResponse[[]KbResponseExternalTaskLink]
	err := WebClientRpcCall(kbCfg, newRequest("getAllExternalTaskLinks", KbTaskIdParam{TaskId: taskId}), &resp)
	return resp.Result, err
}
