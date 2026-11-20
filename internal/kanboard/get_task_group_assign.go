package kanboard

import (
	"encoding/json"
	"fmt"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// GetTaskGroupAssign returns the assigned group and the other assignees of a task.
// The procedure comes from the Group_assign plugin, version 1.8.4 or newer; without
// it (or with an older version) the error satisfies IsMethodNotFound. Returns nil
// (and no error) when the task does not exist.
func GetTaskGroupAssign(kbCfg config.KanboardConfig, taskId int) (*KbTaskGroupAssign, error) {
	// https://github.com/creecros/Group_assign/blob/master/Api/Procedure/GroupAssignTaskProcedures.php
	var resp KbResponse[json.RawMessage]
	if err := WebClientRpcCall(kbCfg, newRequest("getTaskGroupAssign", KbIdParam{Id: taskId}), &resp); err != nil {
		return nil, err
	}
	// An unknown task gives false.
	if r := string(resp.Result); r == "" || r == "false" || r == "null" {
		return nil, nil
	}
	var res KbTaskGroupAssign
	if err := json.Unmarshal(resp.Result, &res); err != nil {
		return nil, fmt.Errorf("getTaskGroupAssign: %w", err)
	}
	return &res, nil
}
