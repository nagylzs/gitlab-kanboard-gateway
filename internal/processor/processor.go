package processor

import (
	"bytes"
	"fmt"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/signal"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/webhooks"
	"log/slog"
	"strconv"
	"text/template"
	"time"
)

type Processor struct {
	cfg         *config.KanboardConfig
	template    *template.Template
	projects    []kanboard.KbResponseProject
	tasks       map[int]kanboard.KbResponseTask
	lastFetched time.Time
	lastElapsed time.Duration

	afterRefresh []webhooks.PushEvent
}

func NewProcessor(cfg *config.KanboardConfig, template *template.Template) *Processor {
	return &Processor{
		cfg:          cfg,
		template:     template,
		projects:     make([]kanboard.KbResponseProject, 0),
		tasks:        make(map[int]kanboard.KbResponseTask),
		afterRefresh: make([]webhooks.PushEvent, 0),
	}
}

func (p *Processor) Run() {
	for !signal.IsStopping() {
		p.refreshCache()
		p.runOnce() // this may block for one second
	}
}

func (p *Processor) refreshCache() {
	elapsed := time.Since(p.lastFetched)
	timeout := p.cfg.DefRefreshInterval
	if len(p.afterRefresh) > 0 {
		timeout = p.cfg.MinRefreshInterval
	}
	minTimeout := p.lastElapsed * time.Duration(10)
	if minTimeout > timeout {
		timeout = minTimeout
		slog.Warn(fmt.Sprintf("cache refresh took %v to complete the last time, limiting refresh interval to %v",
			p.lastElapsed.String(), minTimeout))
		slog.Warn("consider increasing MinRefreshInterval and/or DefRefreshInterval")
	}
	if elapsed < timeout {
		return
	}

	start := time.Now()
	projects, err := kanboard.ListAllProjects(*p.cfg)
	elapsed = time.Since(start)
	if err != nil {
		slog.Error(fmt.Sprintf("could not list projects: %v", err))
		return
	}
	slog.Info(fmt.Sprintf("reloaded %v project(s) in %v", len(projects.Result), elapsed.String()))
	p.projects = projects.Result

	taskFetchElapsed := time.Duration(0)
	tasks := make(map[int]kanboard.KbResponseTask)
	for _, project := range p.projects {
		start2 := time.Now()
		ptsk, err2 := kanboard.GetAllTasks(*p.cfg, project.Id)
		elapsed = time.Since(start2)
		taskFetchElapsed += elapsed
		if err2 != nil {
			slog.Error(
				"could not list tasks for project",
				"project_id", project.Id,
				"project_name", project.Name,
				"error", err2.Error(),
			)
			continue
		}
		for _, task := range ptsk.Result {
			tasks[task.Id] = task
		}
	}
	slog.Info(fmt.Sprintf("reloaded %v tasks(s) in %v", len(tasks), taskFetchElapsed.String()))
	p.tasks = tasks

	p.lastFetched = time.Now()
	p.lastElapsed = time.Since(start)

	// Now we add the ones that are failed because referenced to unknown tasks.
	if len(p.afterRefresh) > 0 {
		slog.Info(fmt.Sprintf("retrying %v event(s)", len(p.afterRefresh)))
		p.afterRefresh = make([]webhooks.PushEvent, 0)
	}
	for _, event := range p.afterRefresh {
		event.CanRetry = false
		// TODO: this might block forever if the queue is full, but this is not likely (queue length is 1000)
		webhooks.PushQueue <- event
	}
}

type ReferenceFound struct {
	Commit webhooks.Commit
	Task   kanboard.KbResponseTask
}

func (p *Processor) runOnce() {
	var event webhooks.PushEvent
	select {
	case event = <-webhooks.PushQueue:
	case <-time.After(1 * time.Second):
		return
	}

	want := false
	for _, pattern := range p.cfg.RefPatterns {
		if pattern.MatchString(event.Ref) {
			want = true
			break
		}
	}
	if !want {
		slog.Debug(fmt.Sprintf("skipping event with not wanted ref %v", event.Ref))
		return
	}

	references := make([]ReferenceFound, 0)

	for _, commit := range event.Commits {
		for _, pattern := range p.cfg.TaskRefPatterns {
			for _, match := range pattern.FindAllStringSubmatch(commit.Message, -1) {
				if len(match) < 1 {
					slog.Error("Error in regexp match, should have a capturing group for the task id")
					signal.Stop(1)
					return
				}
				taskId, err := strconv.Atoi(match[1])
				if err != nil {
					slog.Error("Error in regexp match, task id capturing group must be an integer number \\d+")
					signal.Stop(1)
					return
				}
				task, ok := p.tasks[taskId]
				if !ok {
					p.retryEvent(commit, taskId, event)
					return
				}
				slog.Info(
					"found reference",
					"commit", commit.Id,
					"author", commit.Author,
					"taskId", taskId,
					"taskTitle", task.Title)
				references = append(references, ReferenceFound{Commit: commit, Task: task})
			}
		}
	}

	for _, reference := range references {
		var tpl bytes.Buffer
		err := p.template.Execute(&tpl, TemplateArgs{
			Event:  event,
			Commit: reference.Commit,
			Task:   reference.Task,
		})
		var markdown string
		if err != nil {
			slog.Warn("error executing template: %w, fallback to default commit message", err)
			markdown = fmt.Sprintf(
				`%v (%v) pushed commit [%v](%v "%v") to %v`,
				event.UserName, event.UserUserName,
				reference.Commit.Id, reference.Commit.Url, reference.Commit.Title,
				event.Ref,
			)
			if reference.Commit.Author.Name != event.UserUserName {
				markdown += fmt.Sprintf("  (author: %v %v)",
					reference.Commit.Author.Name, reference.Commit.Author.Email)
			}
		} else {
			markdown = tpl.String()
		}
		_, err = kanboard.CreateComment(*p.cfg, reference.Task.Id, p.cfg.UserId, markdown)
		if err != nil {
			slog.Info(
				"could not create comment",
				"commit", reference.Commit.Id,
				"author", reference.Commit.Author,
				"taskId", reference.Task.Id,
				"taskTitle", reference.Task.Title,
				"error", err.Error())
		}
	}

}

func (p *Processor) retryEvent(commit webhooks.Commit, taskId int, event webhooks.PushEvent) {
	if event.CanRetry {
		slog.Warn(
			"found reference, but there is no such task, will try later",
			"commit", commit.Id,
			"author", commit.Author,
			"taskId", taskId,
		)
		p.afterRefresh = append(p.afterRefresh, event)
	}
}
