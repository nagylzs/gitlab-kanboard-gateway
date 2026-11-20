package taskedit

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
)

// The assigned group and the other assignees come from the Group_assign plugin
// (https://github.com/creecros/Group_assign). Changing them on an existing task needs
// version 1.8.4 or newer: older versions have no getTaskGroupAssign and
// patchTaskGroupAssign, and their updateTaskGroupAssign resets every field it is not
// given, so it is never used.

type AssignInput struct {
	Owner string // "me", a user id, name or username; "nobody" removes the assignee; empty: unchanged
	Group string // group id or name; "none" removes the group; empty: unchanged
	// Replaces the other assignees; an empty (non-nil) list removes them all; nil: unchanged.
	OtherAssignees []string
	// Change the other assignees one by one; cannot be combined with OtherAssignees.
	AddOtherAssignees    []string
	RemoveOtherAssignees []string
}

// AssignResult shows who the task is assigned to after the change.
type AssignResult struct {
	Id             int                `json:"id"`
	Url            string             `json:"url"`
	Title          string             `json:"title"`
	Owner          *taskdump.UserRef  `json:"owner"`
	AssignedGroup  *taskdump.NamedRef `json:"assigned_group"`
	OtherAssignees []taskdump.UserRef `json:"other_assignees"`
	Warnings       []string           `json:"warnings,omitempty"`
}

const (
	hintAssignable = "the assignee must be able to take tasks in the project (a member, not a viewer)"
	hintGroup      = "the group must be added to the project, and every other assignee must be able to take tasks in it " +
		"(a member, not a viewer)"
)

// AssignTask changes the assignee, the assigned group and/or the other assignees of a
// task; what is not given stays as it is. Everything is resolved and checked before
// the first change.
func (e *Editor) AssignTask(taskRef string, in AssignInput) (*AssignResult, error) {
	replace := in.OtherAssignees != nil
	oneByOne := len(in.AddOtherAssignees) > 0 || len(in.RemoveOtherAssignees) > 0
	owner, group := strings.TrimSpace(in.Owner), strings.TrimSpace(in.Group)
	if replace && oneByOne {
		return nil, errors.New("give other_assignees (the whole list) or add/remove_other_assignees, not both")
	}
	if owner == "" && group == "" && !replace && !oneByOne {
		return nil, errors.New("give owner, group, other_assignees, add_other_assignees or remove_other_assignees")
	}
	e.assignMu.Lock()
	defer e.assignMu.Unlock()
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}

	ownerId := -1 // unchanged
	if owner != "" {
		if strings.EqualFold(owner, "nobody") || owner == "0" {
			ownerId = 0
		} else {
			m, err := e.member(t.ProjectId, owner)
			if err != nil {
				return nil, err
			}
			ownerId = m.id
		}
	}

	patch := kanboard.KbPatchTaskGroupAssignParams{Id: t.Id}
	if group != "" || replace || oneByOne {
		// Also tells whether the server's plugin is new enough.
		ga, err := kanboard.GetTaskGroupAssign(e.cfg, t.Id)
		if err != nil {
			return nil, groupAssignErr(err, "changing the group or the other assignees")
		}
		if ga == nil {
			return nil, taskdump.ErrNotFound{TaskId: t.Id}
		}
		if group != "" {
			gid, err := e.groupId(group)
			if err != nil {
				return nil, err
			}
			if gid != ga.GroupId {
				patch.GroupId = &gid
			}
		}
		var list []int
		switch {
		case replace:
			ms, err := e.members(t.ProjectId, nonEmpty(in.OtherAssignees))
			if err != nil {
				return nil, err
			}
			list = ids(ms)
		case oneByOne:
			if list, err = e.changeOthers(t.ProjectId, ga, in.AddOtherAssignees, in.RemoveOtherAssignees); err != nil {
				return nil, err
			}
		}
		if (replace || oneByOne) && !sameIds(list, ga.OtherAssigneeIds) {
			patch.OtherAssignees = &list
		}
	}

	patched := false
	if patch.GroupId != nil || patch.OtherAssignees != nil {
		if err := kanboard.PatchTaskGroupAssign(e.cfg, patch); err != nil {
			return nil, refused(groupAssignErr(err, "changing the group or the other assignees"), hintGroup)
		}
		patched = true
	}
	if ownerId >= 0 && ownerId != t.OwnerId {
		if err := kanboard.UpdateTaskOwner(e.cfg, t.Id, ownerId); err != nil {
			err = refused(err, hintAssignable)
			if patched {
				return nil, fmt.Errorf("the group and the other assignees were changed, but not the assignee: %w", err)
			}
			return nil, err
		}
	}
	return e.assignResult(t.Id), nil
}

// changeOthers returns the other assignees after adding and removing users.
func (e *Editor) changeOthers(projectId int, ga *kanboard.KbTaskGroupAssign, add, remove []string) ([]int, error) {
	list := slices.Clone(ga.OtherAssigneeIds)
	if refs := nonEmpty(add); len(refs) > 0 {
		ms, err := e.members(projectId, refs)
		if err != nil {
			return nil, err
		}
		for _, m := range ms {
			if !slices.Contains(list, m.id) {
				list = append(list, m.id)
			}
		}
	}
	current := make([]named, len(ga.OtherAssignees))
	for i, u := range ga.OtherAssignees {
		current[i] = named{id: u.Id, name: u.Name, alt: u.Username}
	}
	for _, ref := range nonEmpty(remove) {
		ref, err := e.resolveMe(ref)
		if err != nil {
			return nil, err
		}
		if u, err := pick("other assignee", ref, current); err == nil {
			list = slices.DeleteFunc(list, func(id int) bool { return id == u.id })
			continue
		}
		// Someone who is not among the other assignees: nothing to remove. A name
		// that matches nobody is a mistake, though.
		if _, err := e.member(projectId, ref); err != nil {
			return nil, err
		}
	}
	if list == nil {
		list = []int{}
	}
	return list, nil
}

// groupId resolves an assigned group: "none" (or 0) is no group, otherwise a group id
// or name. Listing the groups is admin-only, so a non-admin personal token can only
// give the id. Whether the group is added to the project is checked by the plugin.
func (e *Editor) groupId(ref string) (int, error) {
	ref = strings.TrimSpace(ref)
	if strings.EqualFold(ref, "none") || ref == "0" {
		return 0, nil
	}
	groups, err := kanboard.GetAllGroups(e.cfg)
	if kanboard.IsForbidden(err) {
		if id, err := strconv.Atoi(ref); err == nil && id > 0 {
			return id, nil
		}
		return 0, fmt.Errorf("only an administrator can look up groups by name (getAllGroups is admin-only); "+
			"give the group id instead of %q", ref)
	}
	if err != nil {
		return 0, fmt.Errorf("getAllGroups: %w", err)
	}
	items := make([]named, len(groups))
	for i, g := range groups {
		items[i] = named{id: g.Id, name: g.Name}
	}
	g, err := pick("group", ref, items)
	return g.id, err
}

// groupAssignErr explains a missing plugin procedure.
func groupAssignErr(err error, what string) error {
	if kanboard.IsMethodNotFound(err) {
		return fmt.Errorf("%v needs the Group_assign plugin, version 1.8.4 or newer, on the Kanboard server: %w", what, err)
	}
	return err
}

// assignResult reads back who the task is assigned to, as get_task shows it.
func (e *Editor) assignResult(taskId int) *AssignResult {
	doc, err := taskdump.NewDumper(e.cfg, taskdump.Options{
		Fields: []string{"owner", "assigned_group", "other_assignees"},
	}).Dump(taskId)
	if err != nil {
		// The change was made; only the summary is missing.
		return &AssignResult{Id: taskId, Warnings: []string{fmt.Sprintf("the result could not be read back: %v", err)}}
	}
	others := doc.OtherAssignees
	if others == nil {
		others = []taskdump.UserRef{}
	}
	return &AssignResult{Id: doc.Id, Url: doc.Url, Title: doc.Title, Owner: doc.Owner,
		AssignedGroup: doc.AssignedGroup, OtherAssignees: others, Warnings: doc.Warnings}
}

// nonEmpty returns the trimmed, non-empty strings.
func nonEmpty(refs []string) []string {
	var res []string
	for _, r := range refs {
		if r = strings.TrimSpace(r); r != "" {
			res = append(res, r)
		}
	}
	return res
}

// ids returns the ids of the users, without duplicates, in order.
func ids(users []named) []int {
	res := []int{}
	for _, u := range users {
		if !slices.Contains(res, u.id) {
			res = append(res, u.id)
		}
	}
	return res
}

func sameIds(a, b []int) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)
	return slices.Equal(a, b)
}
