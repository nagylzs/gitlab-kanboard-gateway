package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestListTaskColors(t *testing.T) {
	session, fake := connect(t)
	for _, args := range []map[string]any{{}, nil} {
		res, text := call(t, session, "list_task_colors", args)
		if res.IsError {
			t.Fatal(text)
		}
		var got struct {
			Default string `json:"default"`
			Colors  []struct {
				Id, Name, Background, Border string
			} `json:"colors"`
		}
		must(t, json.Unmarshal([]byte(text), &got))
		if got.Default != "yellow" {
			t.Errorf("default = %q, want yellow", got.Default)
		}
		var ids []string
		for _, c := range got.Colors {
			ids = append(ids, c.Id)
		}
		// The server's order, not sorted.
		if strings.Join(ids, " ") != "yellow red white_on_black deep_orange orange" {
			t.Errorf("color ids = %v", ids)
		}
		if c := got.Colors[1]; c.Name != "Piros" || c.Background != "rgb(255, 187, 187)" || c.Border == "" {
			t.Errorf("red = %+v, want the translated name and the CSS colors", c)
		}
		if c := got.Colors[2]; c.Name != "White on Black" || c.Background != "" {
			t.Errorf("a plugin's color has no CSS colors: %+v", c)
		}
	}
	if calls := fake.writeCalls(); len(calls) != 0 {
		t.Errorf("listing colors called %v", calls)
	}
}

func TestCreateTaskColor(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "create_task", map[string]any{"project": "web", "title": "x", "color": "sötét Narancs"})
	if res.IsError {
		t.Fatal(text)
	}
	if p := fake.wrote("createTask"); len(p) != 1 || p[0]["color_id"] != "deep_orange" {
		t.Errorf("createTask = %v, want color_id deep_orange", p)
	}
	if !strings.Contains(text, `"color":"deep_orange"`) || strings.Contains(text, "warnings") {
		t.Errorf("unexpected result: %s", text)
	}

	// Without a color Kanboard chooses it.
	session, fake, _ = connectWritable(t, "Claude")
	res, text = call(t, session, "create_task", map[string]any{"project": "web", "title": "x"})
	if res.IsError {
		t.Fatal(text)
	}
	if p := fake.wrote("createTask"); len(p) != 1 || p[0]["color_id"] != nil {
		t.Errorf("createTask = %v, want no color_id", p)
	}
	if !strings.Contains(text, `"color":"yellow"`) || fake.called("getColorList") {
		t.Errorf("want the default color, without looking up the colors: %s", text)
	}

	// With a group, the plugin's procedure gets the color.
	session, fake, _ = connectWritable(t, "Claude")
	res, text = call(t, session, "create_task", map[string]any{"project": "web", "title": "x", "group": "Devs", "color": "Red"})
	if res.IsError {
		t.Fatal(text)
	}
	if p := fake.wrote("createTaskGroupAssign"); len(p) != 1 || p[0]["color_id"] != "red" {
		t.Errorf("createTaskGroupAssign = %v, want color_id red", p)
	}
}

func TestCreateTaskColorAutomaticAction(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	fake.setActionColor("orange")
	res, text := call(t, session, "create_task", map[string]any{"project": "web", "title": "x", "color": "red"})
	if res.IsError {
		t.Fatal(text)
	}
	if !strings.Contains(text, `"color":"orange"`) || !strings.Contains(text, "automatic action") {
		t.Errorf("want the actual color and a warning: %s", text)
	}
}

func TestSetTaskColor(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "set_task_color", map[string]any{"task": "#KB1", "color": "piros"})
	if res.IsError {
		t.Fatal(text)
	}
	if u := fake.wrote("updateTask"); len(u) != 1 || u[0]["id"] != 1.0 || u[0]["color_id"] != "red" || len(u[0]) != 2 {
		t.Errorf("updateTask = %v, want only id 1 and color_id red", u)
	}
	want := `"color":{"id":"red","name":"Piros"},"previous_color":{"id":"yellow","name":"Sárga"}}`
	if !strings.HasPrefix(text, `{"id":1,`) || !strings.HasSuffix(text, want) {
		t.Errorf("result = %s, want it to end with %s", text, want)
	}

	// The same color again changes nothing.
	res, text = call(t, session, "set_task_color", map[string]any{"task": "1", "color": "RED"})
	if res.IsError {
		t.Fatal(text)
	}
	if n := len(fake.wrote("updateTask")); n != 1 {
		t.Errorf("updateTask called %d times, want 1", n)
	}
	if !strings.Contains(text, `"previous_color":{"id":"red","name":"Piros"}`) {
		t.Errorf("unexpected result: %s", text)
	}

	res, text = call(t, session, "set_task_color", map[string]any{"task": "1", "color": "white-on-black"})
	if res.IsError || !strings.Contains(text, `"color":{"id":"white_on_black","name":"White on Black"}`) {
		t.Errorf("a plugin's color: %s", text)
	}
}

func TestSetTaskColorAutomaticAction(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	fake.setActionColor("orange")
	res, text := call(t, session, "set_task_color", map[string]any{"task": "1", "color": "red"})
	if res.IsError {
		t.Fatal(text)
	}
	if !strings.Contains(text, `"color":{"id":"orange","name":"Narancssárga"}`) || !strings.Contains(text, "automatic action") {
		t.Errorf("want the actual color and a warning: %s", text)
	}
}

func TestColorRefusedBeforeAnyChange(t *testing.T) {
	cases := []struct {
		tool    string
		args    map[string]any
		wantErr string
	}{
		{"set_task_color", map[string]any{"task": "1", "color": "nope"}, "the color ids are: yellow, red, white_on_black"},
		{"set_task_color", map[string]any{"task": "1", "color": "narancs"}, `matches more than one color: deep_orange ("Sötét narancs"), orange`},
		{"set_task_color", map[string]any{"task": "1", "color": " "}, "give a color"},
		{"set_task_color", map[string]any{"task": "99", "color": "red"}, "not found"},
		{"create_task", map[string]any{"project": "web", "title": "x", "color": "nope"}, "no color matches"},
	}
	for _, c := range cases {
		t.Run(c.wantErr, func(t *testing.T) {
			session, fake, _ := connectWritable(t, "Claude")
			res, text := call(t, session, c.tool, c.args)
			if !res.IsError || !strings.Contains(text, c.wantErr) {
				t.Errorf("want an error with %q, got %s", c.wantErr, text)
			}
			if calls := fake.writeCalls(); len(calls) != 0 {
				t.Errorf("nothing may change, but %v was called", calls)
			}
		})
	}
}
