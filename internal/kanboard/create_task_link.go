package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// CreateTaskLink links two tasks and returns the id of the link as seen from taskId.
// Kanboard also creates the opposite link on the other task, and refuses (false) a
// link that already exists.
func CreateTaskLink(kbCfg config.KanboardConfig, taskId, oppositeTaskId, linkId int) (int, error) {
	// https://docs.kanboard.org/v1/api/internal_task_link_procedures/#createtasklink
	return writeId(kbCfg, "createTaskLink", struct {
		TaskId         int `json:"task_id"`
		OppositeTaskId int `json:"opposite_task_id"`
		LinkId         int `json:"link_id"`
	}{taskId, oppositeTaskId, linkId})
}
