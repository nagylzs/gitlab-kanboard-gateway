package taskedit

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
)

// named is something an agent may refer to by id or by name.
type named struct {
	id   int
	name string
	alt  string // another accepted name (a project identifier, a username)
}

// pick resolves ref to one of items: an id, an exact (case-insensitive) name, or a
// part of exactly one name.
func pick(kind, ref string, items []named) (named, error) {
	ref = strings.TrimSpace(ref)
	if id, err := strconv.Atoi(ref); err == nil {
		for _, it := range items {
			if it.id == id {
				return it, nil
			}
		}
		return named{}, fmt.Errorf("no %v with id %v; %v", kind, id, choices(kind, items))
	}
	var exact, partial []named
	lref := strings.ToLower(ref)
	for _, it := range items {
		switch {
		case strings.EqualFold(it.name, ref) || (it.alt != "" && strings.EqualFold(it.alt, ref)):
			exact = append(exact, it)
		case strings.Contains(strings.ToLower(it.name), lref):
			partial = append(partial, it)
		}
	}
	matches := exact
	if len(matches) == 0 {
		matches = partial
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return named{}, fmt.Errorf("no %v matches %q; %v", kind, ref, choices(kind, items))
	}
	return named{}, fmt.Errorf("%q matches more than one %v (%v); give the id", ref, kind, list(matches))
}

func choices(kind string, items []named) string {
	if len(items) == 0 {
		return fmt.Sprintf("there is no %v to choose from", kind)
	}
	return "choose one of: " + list(items)
}

func list(items []named) string {
	s := make([]string, len(items))
	for i, it := range items {
		s[i] = fmt.Sprintf("%q (%v)", it.name, it.id)
	}
	return strings.Join(s, ", ")
}

// project resolves a project id or name among the projects this server may use.
func (e *Editor) project(ref string) (*kanboard.KbResponseProject, error) {
	if id, err := strconv.Atoi(strings.TrimSpace(ref)); err == nil {
		p, err := kanboard.GetProjectById(e.cfg, id)
		if err != nil {
			if kanboard.IsForbidden(err) {
				return nil, fmt.Errorf("project %v does not exist, or the Kanboard user of this server is not a member: %w", id, err)
			}
			return nil, fmt.Errorf("getProjectById %v: %w", id, err)
		}
		if p == nil {
			return nil, fmt.Errorf("project %v not found", id)
		}
		return p, nil
	}
	var projects []kanboard.KbResponseProject
	if e.cfg.IsUserAuth() {
		var err error
		if projects, err = kanboard.GetMyProjects(e.cfg); err != nil {
			return nil, fmt.Errorf("getMyProjects: %w", err)
		}
	} else {
		resp, err := kanboard.ListAllProjects(e.cfg)
		if err != nil {
			return nil, fmt.Errorf("getAllProjects: %w", err)
		}
		for _, p := range resp.Result {
			if p.IsActive {
				projects = append(projects, p)
			}
		}
	}
	items := make([]named, len(projects))
	for i, p := range projects {
		items[i] = named{id: int(p.Id), name: p.Name, alt: p.Identifier}
	}
	n, err := pick("active project", ref, items)
	if err != nil {
		return nil, err
	}
	for i := range projects {
		if int(projects[i].Id) == n.id {
			return &projects[i], nil
		}
	}
	panic("unreachable")
}

func (e *Editor) columns(projectId int) ([]named, error) {
	cols, err := kanboard.GetColumns(e.cfg, projectId)
	if err != nil {
		return nil, fmt.Errorf("getColumns: %w", err)
	}
	items := make([]named, len(cols))
	for i, c := range cols {
		items[i] = named{id: int(c.Id), name: c.Title}
	}
	return items, nil
}

func (e *Editor) swimlanes(projectId int) ([]named, error) {
	lanes, err := kanboard.GetActiveSwimlanes(e.cfg, projectId)
	if err != nil {
		return nil, fmt.Errorf("getActiveSwimlanes: %w", err)
	}
	items := make([]named, len(lanes))
	for i, l := range lanes {
		items[i] = named{id: int(l.Id), name: l.Name}
	}
	return items, nil
}

func (e *Editor) categories(projectId int) ([]named, error) {
	cats, err := kanboard.GetAllCategories(e.cfg, projectId)
	if err != nil {
		return nil, fmt.Errorf("getAllCategories: %w", err)
	}
	items := make([]named, len(cats))
	for i, c := range cats {
		items[i] = named{id: int(c.Id), name: c.Name}
	}
	return items, nil
}

// member resolves a project member: "me", a user id, a display name or a username.
// Usernames are looked up only when the display names do not match, and only where
// getUser is allowed (application token, admins).
func (e *Editor) member(projectId int, ref string) (named, error) {
	users, err := kanboard.GetProjectUsers(e.cfg, projectId)
	if err != nil {
		return named{}, fmt.Errorf("getProjectUsers: %w", err)
	}
	items := make([]named, 0, len(users))
	for id, name := range users {
		items = append(items, named{id: id, name: name})
	}
	slices.SortFunc(items, func(a, b named) int { return a.id - b.id })
	if strings.EqualFold(strings.TrimSpace(ref), "me") {
		me, err := e.UserId()
		if err != nil {
			return named{}, err
		}
		ref = strconv.Itoa(me)
	}
	if n, err := pick("project member", ref, items); err == nil {
		return n, nil
	} else if _, isId := strconv.Atoi(strings.TrimSpace(ref)); isId == nil {
		return named{}, err
	}
	for i := range items {
		u, err := kanboard.GetUser(e.cfg, items[i].id)
		if err != nil {
			break // forbidden for non-admin personal tokens: names only
		}
		if u != nil {
			items[i].alt = u.Username
		}
	}
	return pick("project member", ref, items)
}

// isMember reports whether user userId is a member of the project.
func (e *Editor) isMember(projectId, userId int) (bool, error) {
	users, err := kanboard.GetProjectUsers(e.cfg, projectId)
	if err != nil {
		return false, fmt.Errorf("getProjectUsers: %w", err)
	}
	_, ok := users[userId]
	return ok, nil
}

// defaultLinks are the relations of a new Kanboard installation. getAllLinks (and
// every other link type procedure) is admin-only for personal tokens, so a non-admin
// user gets these; AddTaskLink then checks the label of the link it created.
var defaultLinks = []named{
	{id: 1, name: "relates to"}, {id: 2, name: "blocks"}, {id: 3, name: "is blocked by"},
	{id: 4, name: "duplicates"}, {id: 5, name: "is duplicated by"}, {id: 6, name: "is a child of"},
	{id: 7, name: "is a parent of"}, {id: 8, name: "targets milestone"}, {id: 9, name: "is a milestone of"},
	{id: 10, name: "fixes"}, {id: 11, name: "is fixed by"},
}

// linkType resolves a relation label. verified is false when the id comes from
// defaultLinks instead of the server.
func (e *Editor) linkType(ref string) (rel named, verified bool, err error) {
	links, err := kanboard.GetAllLinks(e.cfg)
	if err != nil && !kanboard.IsForbidden(err) {
		return named{}, false, fmt.Errorf("getAllLinks: %w", err)
	}
	items := defaultLinks
	if err == nil {
		items = make([]named, len(links))
		for i, l := range links {
			items[i] = named{id: l.Id, name: l.Label}
		}
	}
	// Exact matches only: a relation in the wrong direction ("blocks" instead of
	// "is blocked by") is worse than an error.
	ref = strings.TrimSpace(ref)
	for _, it := range items {
		if strings.EqualFold(it.name, ref) {
			return it, err == nil, nil
		}
	}
	return named{}, false, fmt.Errorf("unknown relation %q; %v", ref, choices("relation", items))
}

func nameOf(items []named, id int) string {
	for _, it := range items {
		if it.id == id {
			return it.name
		}
	}
	return ""
}
