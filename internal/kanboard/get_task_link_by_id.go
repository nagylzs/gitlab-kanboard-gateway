package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetTaskLinkById returns nil (and no error) when the link does not exist. With a
// personal token a missing link is a 403 error instead.
func GetTaskLinkById(kbCfg config.KanboardConfig, taskLinkId int) (*KbResponseTaskLinkById, error) {
	// https://docs.kanboard.org/v1/api/internal_task_link_procedures/#gettasklinkbyid
	var resp KbResponse[*KbResponseTaskLinkById]
	err := WebClientRpcCall(kbCfg, newRequest("getTaskLinkById", KbTaskLinkIdParam{TaskLinkId: taskLinkId}), &resp)
	return resp.Result, err
}
