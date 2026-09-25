package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetProjectById returns nil (and no error) when the project does not exist.
func GetProjectById(kbCfg config.KanboardConfig, projectId int) (*KbResponseProject, error) {
	// https://docs.kanboard.org/v1/api/project_procedures/#getprojectbyid
	var resp KbResponse[*KbResponseProject]
	err := WebClientRpcCall(kbCfg, newRequest("getProjectById", KbProjectIdParam{ProjectId: int32(projectId)}), &resp)
	return resp.Result, err
}
