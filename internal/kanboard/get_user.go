package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetUser returns nil (and no error) when the user does not exist (e.g. id 0 = unassigned).
func GetUser(kbCfg config.KanboardConfig, userId int) (*KbResponseUser, error) {
	// https://docs.kanboard.org/v1/api/user_procedures/#getuser
	var resp KbResponse[*KbResponseUser]
	err := WebClientRpcCall(kbCfg, newRequest("getUser", KbUserIdParam{UserId: userId}), &resp)
	return resp.Result, err
}
