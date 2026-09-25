package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

func GetAllComments(kbCfg config.KanboardConfig, taskId int) ([]KbResponseComment, error) {
	// https://docs.kanboard.org/v1/api/comment_procedures/#getallcomments
	var resp KbResponse[[]KbResponseComment]
	err := WebClientRpcCall(kbCfg, newRequest("getAllComments", KbTaskIdParam{TaskId: taskId}), &resp)
	return resp.Result, err
}
