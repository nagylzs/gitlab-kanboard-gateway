package kanboard

import (
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

func GetAllTasks(kbCfg config.KanboardConfig, projectId int32) (KbResponseGetAllTasks, error) {
	req := KbProjectLevelRequest{
		JsonRpc: "2.0",
		Method:  "getAllTasks",
		Id:      1,
		Params:  KbProjectIdParam{ProjectId: projectId},
	}
	var resp KbResponseGetAllTasks
	err := WebClientRpcCall(kbCfg, req, &resp)
	if err != nil {
		return resp, err
	}
	return resp, err
}
