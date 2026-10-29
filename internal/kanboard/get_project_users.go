package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetProjectUsers returns user id -> display name (name, or username when the name is
// empty) of the project members, including members through groups. Unlike getUser it
// is allowed for every project member, not only for admins.
func GetProjectUsers(kbCfg config.KanboardConfig, projectId int) (map[int]string, error) {
	// https://docs.kanboard.org/v1/api/project_permission_procedures/#getprojectusers
	var resp KbResponse[map[int]string]
	err := WebClientRpcCall(kbCfg, newRequest("getProjectUsers", KbProjectIdParam{ProjectId: int32(projectId)}), &resp)
	return resp.Result, err
}
