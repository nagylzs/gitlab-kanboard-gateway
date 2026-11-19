package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetColumns returns the columns of a project, left to right.
func GetColumns(kbCfg config.KanboardConfig, projectId int) ([]KbResponseColumn, error) {
	// https://docs.kanboard.org/v1/api/column_procedures/#getcolumns
	var resp KbResponse[[]KbResponseColumn]
	err := WebClientRpcCall(kbCfg, newRequest("getColumns", KbProjectIdParam{ProjectId: int32(projectId)}), &resp)
	return resp.Result, err
}
