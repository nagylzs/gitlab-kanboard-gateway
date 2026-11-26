package taskdump

import (
	"fmt"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
)

// ColorList is what kanboard-task --colors and the list_task_colors tool print.
type ColorList struct {
	Default string  `json:"default"` // color id of a task created without a color ("" if unknown)
	Colors  []Color `json:"colors"`  // in the order of the web UI's color menu
	// Non-fatal problems: the rest of the list is still valid.
	Warnings []string `json:"warnings,omitempty"`
}

type Color struct {
	Id   string `json:"id"`   // what tasks store (TaskDocument.Color)
	Name string `json:"name"` // as the web UI shows it, in the server's language
	// CSS colors; Kanboard knows them only for its built-in colors, not for colors
	// that a plugin adds.
	Background string `json:"background,omitempty"`
	Border     string `json:"border,omitempty"`
}

// ListColors returns the colors a task can have. Only a failing getColorList is an
// error.
func ListColors(cfg config.KanboardConfig) (*ColorList, error) {
	colors, err := kanboard.GetColorList(cfg)
	if err != nil {
		return nil, fmt.Errorf("getColorList: %w", err)
	}
	res := &ColorList{Colors: make([]Color, 0, len(colors))}
	props, err := kanboard.GetDefaultTaskColors(cfg)
	if err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("getDefaultTaskColors: %v (background and border are missing)", err))
	}
	if res.Default, err = kanboard.GetDefaultTaskColor(cfg); err != nil {
		res.Warnings = append(res.Warnings, fmt.Sprintf("getDefaultTaskColor: %v", err))
	}
	for _, c := range colors {
		p := props[c.Id]
		res.Colors = append(res.Colors, Color{Id: c.Id, Name: c.Name, Background: p.Background, Border: p.Border})
	}
	return res, nil
}

// Find returns the color with the given id.
func (l *ColorList) Find(id string) (Color, bool) {
	for _, c := range l.Colors {
		if c.Id == id {
			return c, true
		}
	}
	return Color{}, false
}
