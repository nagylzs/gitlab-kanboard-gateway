package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// RemoveTaskLink removes an internal link, together with its opposite on the other task.
func RemoveTaskLink(kbCfg config.KanboardConfig, taskLinkId int) error {
	// https://docs.kanboard.org/v1/api/internal_task_link_procedures/#removetasklink
	return writeOk(kbCfg, "removeTaskLink", KbTaskLinkIdParam{TaskLinkId: taskLinkId})
}
