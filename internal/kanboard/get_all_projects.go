package kanboard

import (
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

func ListAllProjects(kbCfg config.KanboardConfig) (KbResponseGetAllProjects, error) {
	req := KbAccountLevelRequest{
		JsonRpc: "2.0",
		Method:  "getAllProjects",
		Id:      1,
	}
	var resp KbResponseGetAllProjects
	err := WebClientRpcCall(kbCfg, req, &resp)
	if err != nil {
		return resp, err
	}
	return resp, err
}
