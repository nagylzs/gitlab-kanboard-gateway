package taskedit

import (
	"errors"
	"fmt"
	"strings"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
)

// ColorRef names a task color.
type ColorRef struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

// ColorResult shows the color of a task after SetTaskColor.
type ColorResult struct {
	Id            int      `json:"id"`
	Url           string   `json:"url"`
	Title         string   `json:"title"`
	Color         ColorRef `json:"color"`
	PreviousColor ColorRef `json:"previous_color"`
	Warnings      []string `json:"warnings,omitempty"`
}

// Where an agent or a user can see the colors.
const colorsTool = "list_task_colors (kanboard-task --colors)"

// SetTaskColor changes the color of a task.
func (e *Editor) SetTaskColor(taskRef, color string) (*ColorResult, error) {
	if strings.TrimSpace(color) == "" {
		return nil, errors.New("give a color: its id or name, as " + colorsTool + " shows them")
	}
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	colors, err := taskdump.ListColors(e.cfg)
	if err != nil {
		return nil, err
	}
	c, err := findColor(color, colors.Colors)
	if err != nil {
		return nil, err
	}
	res := &ColorResult{Id: t.Id, Url: t.Url, Title: t.Title,
		Color: ColorRef{Id: c.Id, Name: c.Name}, PreviousColor: colorRef(colors, t.ColorId)}
	if t.ColorId == c.Id {
		return res, nil
	}
	if err := kanboard.UpdateTaskColor(e.cfg, t.Id, c.Id); err != nil {
		return nil, refused(err, "the color must be one of those that "+colorsTool+" shows")
	}
	after, err := kanboard.GetTask(e.cfg, t.Id)
	if err != nil || after == nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("the color was changed, but it could not be read back: %v", err))
	} else if w := colorChanged(c.Id, after.ColorId); w != "" {
		res.Color = colorRef(colors, after.ColorId)
		res.Warnings = append(res.Warnings, w)
	}
	return res, nil
}

// colorChanged explains a task whose color is not the one it was given.
func colorChanged(want, got string) string {
	if got == want || got == "" {
		return ""
	}
	return fmt.Sprintf("the task was given the color %v, but now it has %v: an automatic action of the project "+
		"probably changed it", want, got)
}

func colorRef(colors *taskdump.ColorList, id string) ColorRef {
	c, _ := colors.Find(id)
	return ColorRef{Id: id, Name: c.Name}
}

// findColor resolves a task color: its id ("deep_orange"), its name as the web UI
// shows it ("Deep Orange", or translated), the id written with spaces or hyphens
// ("deep orange"), or else a part of exactly one id or name. Case does not matter.
func findColor(ref string, colors []taskdump.Color) (taskdump.Color, error) {
	ref = strings.TrimSpace(ref)
	key := colorKey(ref)
	if key == "" {
		return taskdump.Color{}, errors.New("give a color: its id or name, as " + colorsTool + " shows them")
	}
	var byName, partial []taskdump.Color
	for _, c := range colors {
		switch {
		case colorKey(c.Id) == key:
			return c, nil
		case strings.EqualFold(c.Name, ref) || colorKey(c.Name) == key:
			byName = append(byName, c)
		case strings.Contains(colorKey(c.Id), key) || strings.Contains(colorKey(c.Name), key):
			partial = append(partial, c)
		}
	}
	matches := byName
	if len(matches) == 0 {
		matches = partial
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		ids := make([]string, len(colors))
		for i, c := range colors {
			ids[i] = c.Id
		}
		return taskdump.Color{}, fmt.Errorf("no color matches %q; the color ids are: %v; %v shows their names",
			ref, strings.Join(ids, ", "), colorsTool)
	}
	s := make([]string, len(matches))
	for i, c := range matches {
		s[i] = fmt.Sprintf("%v (%q)", c.Id, c.Name)
	}
	return taskdump.Color{}, fmt.Errorf("%q matches more than one color: %v; give the id", ref, strings.Join(s, ", "))
}

// colorKey turns a color name into the form of a color id: lower case, words joined
// with underscores.
func colorKey(s string) string {
	s = strings.NewReplacer("_", " ", "-", " ").Replace(strings.ToLower(s))
	return strings.Join(strings.Fields(s), "_")
}
