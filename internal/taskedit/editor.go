// Package taskedit makes the changes that kanboard-mcp and kanboard-task offer when
// they are started with --writable: create tasks, add and edit comments, upload
// files, add and remove links, move, close and reopen tasks.
//
// Nothing here deletes a task, a comment or a file. Text written by an agent (a task
// description, a comment) gets a signature footer, so readers can tell it from text
// the human owner of the token wrote, and update_comment only edits comments that
// carry such a footer.
package taskedit

import (
	"errors"
	"fmt"
	"sync"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
)

type Options struct {
	// Signature is put below text written by the agent ("created by <Signature>").
	// Empty means no footer.
	Signature string
	// UploadFiles reads files only below this absolute directory. Empty disables it.
	UploadDir string
	// UploadFiles refuses larger files (0 = no limit).
	MaxUploadBytes int64
}

// Editor is safe for concurrent use.
type Editor struct {
	cfg  config.KanboardConfig
	opts Options

	mu     sync.Mutex
	userId int // cached result of UserId
}

func NewEditor(cfg config.KanboardConfig, opts Options) *Editor {
	return &Editor{cfg: cfg, opts: opts}
}

// UserId is the Kanboard user that comments are written as: the logged-in user with a
// personal token, Kanboard.UserId with the jsonrpc application token.
func (e *Editor) UserId() (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.userId > 0 {
		return e.userId, nil
	}
	if !e.cfg.IsUserAuth() {
		if e.cfg.UserId <= 0 {
			return 0, errors.New("Kanboard.UserId must be set in the config file to write with the jsonrpc application token " +
				"(comments are written in that user's name)")
		}
		e.userId = e.cfg.UserId
		return e.userId, nil
	}
	me, err := kanboard.GetMe(e.cfg)
	if err != nil {
		return 0, fmt.Errorf("getMe: %w", err)
	}
	if me == nil || me.Id <= 0 {
		return 0, errors.New("getMe returned no user")
	}
	e.userId = me.Id
	return e.userId, nil
}

// task loads a task; a missing task is an error.
func (e *Editor) task(ref string) (*kanboard.KbResponseTask, error) {
	id, err := taskdump.ParseTaskId(ref)
	if err != nil {
		return nil, err
	}
	t, err := kanboard.GetTask(e.cfg, id)
	if err != nil {
		if kanboard.IsForbidden(err) {
			// A personal token also gets 403 for tasks that do not exist.
			return nil, fmt.Errorf("task %v does not exist, or the Kanboard user of this server has no access to it: %w", id, err)
		}
		return nil, fmt.Errorf("getTask %v: %w", id, err)
	}
	if t == nil {
		return nil, taskdump.ErrNotFound{TaskId: id}
	}
	return t, nil
}

// refused adds a hint to an ErrRefused.
func refused(err error, hint string) error {
	var r kanboard.ErrRefused
	if errors.As(err, &r) && hint != "" {
		return fmt.Errorf("%w; %s", err, hint)
	}
	return err
}
