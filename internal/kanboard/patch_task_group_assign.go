package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// A nil field is left out, and the plugin then keeps that value.
type KbPatchTaskGroupAssignParams struct {
	Id             int    `json:"id"`
	GroupId        *int   `json:"group_id,omitempty"`        // 0 = no group
	OtherAssignees *[]int `json:"other_assignees,omitempty"` // replaces the list; empty = none
}

// PatchTaskGroupAssign changes the assigned group and/or the other assignees of a
// task, nothing else. The procedure comes from the Group_assign plugin 1.8.4 or
// newer; without it the error satisfies IsMethodNotFound. The plugin refuses (false)
// a group that is not added to the project and users who cannot be assigned in it.
func PatchTaskGroupAssign(kbCfg config.KanboardConfig, params KbPatchTaskGroupAssignParams) error {
	// https://github.com/creecros/Group_assign/blob/master/Api/Procedure/GroupAssignTaskProcedures.php
	return writeOk(kbCfg, "patchTaskGroupAssign", params)
}
