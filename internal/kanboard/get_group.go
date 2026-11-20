package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetGroup returns nil (and no error) when the group does not exist. It is
// admin-only: a personal token of a non-admin user gets an error that satisfies
// IsForbidden.
func GetGroup(kbCfg config.KanboardConfig, groupId int) (*KbResponseGroup, error) {
	// https://docs.kanboard.org/v1/api/group_procedures/#getgroup
	var resp KbResponse[*KbResponseGroup]
	err := WebClientRpcCall(kbCfg, newRequest("getGroup", KbGroupIdParam{GroupId: groupId}), &resp)
	return resp.Result, err
}
