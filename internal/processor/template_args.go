package processor

import (
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/webhooks"
)

type TemplateArgs struct {
	Event  webhooks.PushEvent
	Commit webhooks.Commit
	Task   kanboard.KbResponseTask
}
