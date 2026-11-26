package taskedit

import (
	"strings"
	"testing"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/taskdump"
)

func TestFindColor(t *testing.T) {
	// As on a Hungarian server with a plugin that adds colors.
	colors := []taskdump.Color{
		{Id: "yellow", Name: "Sárga"}, {Id: "orange", Name: "Narancssárga"}, {Id: "deep_orange", Name: "Sötét narancs"},
		{Id: "lime", Name: "Citrus"}, {Id: "lime_green", Name: "Lime Green"}, {Id: "blood_red", Name: "Blood Red"},
		{Id: "white_on_blood_red", Name: "White on Blood Red"}, {Id: "red", Name: "Piros"},
	}
	for ref, want := range map[string]string{
		"red":            "red",
		" Red ":          "red",
		"piros":          "red",
		"deep orange":    "deep_orange",
		"Deep-Orange":    "deep_orange",
		"SÖTÉT NARANCS":  "deep_orange",
		"lime":           "lime", // the id wins over a part of "lime_green"
		"citrus":         "lime",
		"lime green":     "lime_green",
		"white on blood": "white_on_blood_red",
		"sárga":          "yellow", // the whole name wins over a part of "Narancssárga"
	} {
		c, err := findColor(ref, colors)
		if err != nil || c.Id != want {
			t.Errorf("findColor(%q) = %q, %v; want %q", ref, c.Id, err, want)
		}
	}
	for ref, wantErr := range map[string]string{
		"blood":   "matches more than one color",
		"narancs": "matches more than one color",
		"green_x": "no color matches",
		"  ":      "give a color",
	} {
		if c, err := findColor(ref, colors); err == nil || !strings.Contains(err.Error(), wantErr) {
			t.Errorf("findColor(%q) = %q, %v; want an error with %q", ref, c.Id, err, wantErr)
		}
	}
}
