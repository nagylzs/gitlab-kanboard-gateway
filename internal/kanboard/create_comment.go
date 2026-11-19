package kanboard

import (
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
)

// CreateComment adds a comment and returns its id. With a personal token userId must
// be the logged-in user: Kanboard refuses (false) comments in another user's name.
func CreateComment(kbCfg config.KanboardConfig, taskId int, userId int, markdownContent string) (int, error) {
	// https://docs.kanboard.org/v1/api/comment_procedures/#createcomment
	return writeId(kbCfg, "createComment", KbCreateCommentParams{TaskId: taskId, UserId: userId, Content: markdownContent})
}
