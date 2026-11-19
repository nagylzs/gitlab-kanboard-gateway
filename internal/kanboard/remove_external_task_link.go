package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// RemoveExternalTaskLink removes a URL from a task; false when the task has no such link.
func RemoveExternalTaskLink(kbCfg config.KanboardConfig, taskId, linkId int) error {
	// https://docs.kanboard.org/v1/api/external_task_link_procedures/#removeexternaltasklink
	return writeOk(kbCfg, "removeExternalTaskLink", struct {
		TaskId int `json:"task_id"`
		LinkId int `json:"link_id"`
	}{taskId, linkId})
}
