package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// MoveTaskPosition moves a task to a column and swimlane of its project. position
// starts at 1 (top of the column); a position past the end puts the task at the bottom.
func MoveTaskPosition(kbCfg config.KanboardConfig, projectId, taskId, columnId, position, swimlaneId int) error {
	// https://docs.kanboard.org/v1/api/task_procedures/#movetaskposition
	return writeOk(kbCfg, "moveTaskPosition", struct {
		ProjectId  int `json:"project_id"`
		TaskId     int `json:"task_id"`
		ColumnId   int `json:"column_id"`
		Position   int `json:"position"`
		SwimlaneId int `json:"swimlane_id"`
	}{projectId, taskId, columnId, position, swimlaneId})
}
