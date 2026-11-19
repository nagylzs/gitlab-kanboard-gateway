package kanboard

import "github.com/nagylzs/gitlab-kanboard-gateway/internal/config"

// UpdateComment replaces the text of a comment. With a personal token Kanboard
// refuses (false) to change another user's comment.
func UpdateComment(kbCfg config.KanboardConfig, commentId int, content string) error {
	// https://docs.kanboard.org/v1/api/comment_procedures/#updatecomment
	return writeOk(kbCfg, "updateComment", struct {
		Id      int    `json:"id"`
		Content string `json:"content"`
	}{commentId, content})
}
