package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetActiveSwimlanes returns the active swimlanes of a project, top to bottom.
func GetActiveSwimlanes(kbCfg config.KanboardConfig, projectId int) ([]KbResponseSwimlane, error) {
	// https://docs.kanboard.org/v1/api/swimlane_procedures/#getactiveswimlanes
	var resp KbResponse[[]KbResponseSwimlane]
	err := WebClientRpcCall(kbCfg, newRequest("getActiveSwimlanes", KbProjectIdParam{ProjectId: int32(projectId)}), &resp)
	return resp.Result, err
}
