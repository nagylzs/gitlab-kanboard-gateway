package taskdump

import (
	"fmt"
	"log/slog"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
)

type SearchOptions struct {
	Query string
	// Project ids or (case-insensitive, substring) names/identifiers. Empty means all
	// active projects. Explicitly selected projects are searched even if inactive.
	Projects []string
	// Also search inactive projects when Projects is empty.
	IncludeInactive bool
	// Maximum number of hits returned (0 = no limit). Total still counts all hits.
	Limit int
	// Number of projects searched in parallel.
	Workers int
}

// ErrNoProject is returned by Search when a --project filter matches no project.
type ErrNoProject struct{ Filter string }

func (e ErrNoProject) Error() string { return fmt.Sprintf("no project matches %q", e.Filter) }

const excerptRunes = 300

// Search runs searchTasks in every selected project and merges the hits, newest
// modification first. A failure in one project is recorded in Warnings; an error is
// returned only if the project list cannot be loaded or every project search failed.
func Search(cfg config.KanboardConfig, opts SearchOptions) (*SearchResult, error) {
	projects, err := kanboard.ListAllProjects(cfg)
	if err != nil {
		return nil, fmt.Errorf("getAllProjects: %w", err)
	}
	selected, err := selectProjects(projects.Result, opts)
	if err != nil {
		return nil, err
	}

	res := &SearchResult{
		Query:            opts.Query,
		ProjectsSearched: len(selected),
		Tasks:            []SearchHit{},
		FetchedAt:        time.Now().Format(time.RFC3339),
	}
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		failed int
		sem    = make(chan struct{}, max(opts.Workers, 1))
	)
	for _, p := range selected {
		wg.Add(1)
		sem <- struct{}{}
		go func(p kanboard.KbResponseProject) {
			defer wg.Done()
			defer func() { <-sem }()
			rows, err := kanboard.SearchTasks(cfg, int(p.Id), opts.Query)
			mu.Lock()
			defer mu.Unlock()
			if err != nil {
				msg := fmt.Sprintf("searchTasks in project %v (%v): %v", p.Id, p.Name, err)
				slog.Warn(msg)
				res.Warnings = append(res.Warnings, msg)
				failed++
				return
			}
			for _, r := range rows {
				res.Tasks = append(res.Tasks, searchHit(r, p, cfg.ApiUrl))
			}
		}(p)
	}
	wg.Wait()
	if failed > 0 && failed == len(selected) {
		return nil, fmt.Errorf("searchTasks failed in every project: %v", res.Warnings[0])
	}
	sort.Strings(res.Warnings)

	sort.SliceStable(res.Tasks, func(i, j int) bool {
		a, b := derefStr(res.Tasks[i].Dates.Modified), derefStr(res.Tasks[j].Dates.Modified)
		if a != b {
			return a > b // RFC3339 in the same zone sorts lexically
		}
		return res.Tasks[i].Id > res.Tasks[j].Id
	})
	res.Total = len(res.Tasks)
	if opts.Limit > 0 && len(res.Tasks) > opts.Limit {
		res.Tasks = res.Tasks[:opts.Limit]
		res.Truncated = true
	}
	return res, nil
}

func selectProjects(all []kanboard.KbResponseProject, opts SearchOptions) ([]kanboard.KbResponseProject, error) {
	if len(opts.Projects) == 0 {
		var out []kanboard.KbResponseProject
		for _, p := range all {
			if p.IsActive || opts.IncludeInactive {
				out = append(out, p)
			}
		}
		return out, nil
	}
	seen := make(map[int32]bool)
	var out []kanboard.KbResponseProject
	for _, f := range opts.Projects {
		f = strings.TrimSpace(f)
		id, isId := strconv.Atoi(f)
		lf := strings.ToLower(f)
		matched := false
		for _, p := range all {
			var ok bool
			if isId == nil {
				ok = int(p.Id) == id
			} else {
				ok = strings.Contains(strings.ToLower(p.Name), lf) ||
					(p.Identifier != "" && strings.EqualFold(p.Identifier, f))
			}
			if ok {
				matched = true
				if !seen[p.Id] {
					seen[p.Id] = true
					out = append(out, p)
				}
			}
		}
		if !matched {
			return nil, ErrNoProject{Filter: f}
		}
	}
	return out, nil
}

func searchHit(r kanboard.KbResponseSearchTask, p kanboard.KbResponseProject, apiUrl string) SearchHit {
	h := SearchHit{
		Id:        r.Id,
		Url:       taskUrl(apiUrl, p.Url.Board, r.Id, r.ProjectId),
		Title:     r.Title,
		Status:    openClosed(r.IsActive),
		Project:   NamedRef{Id: r.ProjectId, Name: r.ProjectName},
		Column:    r.ColumnName,
		Swimlane:  derefStr(r.SwimlaneName),
		Category:  derefStr(r.CategoryName),
		Priority:  r.Priority,
		Reference: r.Reference,
		Dates: SearchDates{
			Created:   tsPtr(r.DateCreation),
			Modified:  tsPtr(r.DateModification),
			Due:       tsPtr(r.DateDue),
			Completed: tsPtr(r.DateCompleted),
		},
		TimeTracking: TimeTracking{SpentHours: r.TimeSpent, EstimatedHours: r.TimeEstimated},
		Counts: SearchCounts{
			Comments:      r.NbComments,
			Attachments:   r.NbFiles,
			Subtasks:      r.NbSubtasks,
			SubtasksDone:  r.NbCompletedSubtasks,
			Links:         r.NbLinks,
			ExternalLinks: r.NbExternalLinks,
		},
		DescriptionExcerpt: excerpt(r.Description, excerptRunes),
	}
	if h.Project.Name == "" {
		h.Project.Name = p.Name
	}
	h.Assignee = userFromParts(r.OwnerId, r.AssigneeUsername, r.AssigneeName)
	return h
}

// taskUrl builds the browser URL of a task. searchTasks does not return it, so it is
// derived the way the server builds it: pretty /task/<id> URLs when the project's
// board URL uses them, the controller query string otherwise.
func taskUrl(apiUrl, boardUrl string, taskId, projectId int) string {
	base := appBaseUrl(apiUrl)
	if strings.Contains(boardUrl, "/board/") {
		return fmt.Sprintf("%vtask/%v", base, taskId)
	}
	return fmt.Sprintf("%v?controller=TaskViewController&action=show&task_id=%v&project_id=%v", base, taskId, projectId)
}

var (
	reWhitespace = regexp.MustCompile(`\s+`)
	reImgTag     = regexp.MustCompile(`(?i)<img\b[^>]*>`)
)

func excerpt(s string, n int) string {
	s = reImgTag.ReplaceAllString(s, "[image]")
	s = strings.TrimSpace(reWhitespace.ReplaceAllString(s, " "))
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return strings.TrimSpace(string(r[:n])) + "…"
}
