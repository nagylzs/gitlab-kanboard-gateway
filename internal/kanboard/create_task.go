package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// KbCreateTaskParams are the createTask parameters this program uses. Zero values are
// left out, so Kanboard uses its defaults (first column, default swimlane, ...).
type KbCreateTaskParams struct {
	Title       string   `json:"title"`
	ProjectId   int      `json:"project_id"`
	Description string   `json:"description,omitempty"`
	ColumnId    int      `json:"column_id,omitempty"`
	SwimlaneId  int      `json:"swimlane_id,omitempty"`
	CategoryId  int      `json:"category_id,omitempty"`
	OwnerId     int      `json:"owner_id,omitempty"`
	CreatorId   int      `json:"creator_id,omitempty"`
	Tags        []string `json:"tags,omitempty"`
	ColorId     string   `json:"color_id,omitempty"` // default: the server's default color

	// Only for CreateTaskGroupAssign (Group_assign plugin).
	GroupId        int   `json:"group_id,omitempty"`
	OtherAssignees []int `json:"other_assignees,omitempty"`
}

// CreateTask returns the id of the new task. Kanboard refuses (false) an owner or
// creator that is not a member of the project, and a color id that is not in
// GetColorList; unknown tags are created.
func CreateTask(kbCfg config.KanboardConfig, params KbCreateTaskParams) (int, error) {
	// https://docs.kanboard.org/v1/api/task_procedures/#createtask
	return writeId(kbCfg, "createTask", params)
}

// CreateTaskGroupAssign is CreateTask with an assigned group and other assignees.
// The procedure comes from the Group_assign plugin; without it the error satisfies
// IsMethodNotFound. Version 1.8.4 refuses (false) a group that is not added to the
// project and other assignees who cannot be assigned in it.
func CreateTaskGroupAssign(kbCfg config.KanboardConfig, params KbCreateTaskParams) (int, error) {
	// https://github.com/creecros/Group_assign/blob/master/Api/Procedure/GroupAssignTaskProcedures.php
	return writeId(kbCfg, "createTaskGroupAssign", params)
}
