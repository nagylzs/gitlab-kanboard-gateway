package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetAllGroups is admin-only: a personal token of a non-admin user gets an error that
// satisfies IsForbidden.
func GetAllGroups(kbCfg config.KanboardConfig) ([]KbResponseGroup, error) {
	// https://docs.kanboard.org/v1/api/group_procedures/#getallgroups
	var resp KbResponse[[]KbResponseGroup]
	err := WebClientRpcCall(kbCfg, KbAccountLevelRequest{JsonRpc: "2.0", Method: "getAllGroups", Id: 1}, &resp)
	return resp.Result, err
}
