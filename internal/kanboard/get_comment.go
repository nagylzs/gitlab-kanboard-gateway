package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// GetComment returns nil (and no error) when the comment does not exist. With a
// personal token a missing comment is a 403 error instead.
func GetComment(kbCfg config.KanboardConfig, commentId int) (*KbResponseComment, error) {
	// https://docs.kanboard.org/v1/api/comment_procedures/#getcomment
	var resp KbResponse[*KbResponseComment]
	err := WebClientRpcCall(kbCfg, newRequest("getComment", KbCommentIdParam{CommentId: commentId}), &resp)
	return resp.Result, err
}
