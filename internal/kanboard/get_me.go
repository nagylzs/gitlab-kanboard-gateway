package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetMe returns the logged-in user. It works only with a personal token (or a user's
// password): the application token gets an error.
func GetMe(kbCfg config.KanboardConfig) (*KbResponseUser, error) {
	// https://docs.kanboard.org/v1/api/me_procedures/#getme
	var resp KbResponse[*KbResponseUser]
	err := WebClientRpcCall(kbCfg, newRequest("getMe", struct{}{}), &resp)
	return resp.Result, err
}
