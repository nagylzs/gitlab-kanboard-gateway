package kanboard

import (
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

func CreateComment(kbCfg config.KanboardConfig, taskId int, userId int, markdownContent string) (KbCreateCommentResponse, error) {
	req := KbCreateCommentRequest{
		JsonRpc: "2.0",
		Method:  "createComment",
		Id:      1,
		Params:  KbCreateCommentParams{TaskId: taskId, UserId: userId, Content: markdownContent},
	}
	var resp KbCreateCommentResponse
	err := WebClientRpcCall(kbCfg, req, &resp)
	if err != nil {
		return resp, err
	}
	return resp, err
}
