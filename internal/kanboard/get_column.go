package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetColumn returns nil (and no error) when the column does not exist.
func GetColumn(kbCfg config.KanboardConfig, columnId int) (*KbResponseColumn, error) {
	// https://docs.kanboard.org/v1/api/column_procedures/#getcolumn
	var resp KbResponse[*KbResponseColumn]
	err := WebClientRpcCall(kbCfg, newRequest("getColumn", KbColumnIdParam{ColumnId: columnId}), &resp)
	return resp.Result, err
}
