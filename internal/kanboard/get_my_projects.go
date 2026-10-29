package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetMyProjects returns the active projects the logged-in user is a member of. It works
// only with a personal token: the application token gets an error.
func GetMyProjects(kbCfg config.KanboardConfig) ([]KbResponseProject, error) {
	// https://docs.kanboard.org/v1/api/me_procedures/#getmyprojects
	var resp KbResponse[[]KbResponseProject]
	err := WebClientRpcCall(kbCfg, newRequest("getMyProjects", struct{}{}), &resp)
	return resp.Result, err
}

// GetMyProjectsList returns id -> name of every project (active and inactive) the
// logged-in user is a member of. Personal token only, like GetMyProjects.
func GetMyProjectsList(kbCfg config.KanboardConfig) (map[int]string, error) {
	// https://docs.kanboard.org/v1/api/me_procedures/#getmyprojectslist
	var resp KbResponse[map[int]string]
	err := WebClientRpcCall(kbCfg, newRequest("getMyProjectsList", struct{}{}), &resp)
	return resp.Result, err
}
