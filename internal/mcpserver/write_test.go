package mcpserver

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var writeToolNames = []string{
	"add_comment", "add_external_link", "add_task_link", "assign_task", "close_task", "create_task", "move_task", "open_task",
	"remove_external_link", "remove_task_link", "update_comment", "upload_files",
}

func connectWritable(t *testing.T, signature string) (*mcp.ClientSession, *fakeKanboard, string) {
	t.Helper()
	dir := t.TempDir()
	session, fake := connectOpts(t, Options{Writable: true, Signature: signature, UploadDir: dir, MaxUploadBytes: 100})
	return session, fake, dir
}

func TestWriteToolsOnlyWhenWritable(t *testing.T) {
	session, _, _ := connectWritable(t, "Claude")
	res, err := session.ListTools(context.Background(), nil)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, tool := range res.Tools {
		if !slices.Contains(writeToolNames, tool.Name) {
			continue
		}
		names = append(names, tool.Name)
		a := tool.Annotations
		if a == nil || a.ReadOnlyHint || a.DestructiveHint == nil {
			t.Errorf("%v must be marked as not read-only, with a destructive hint: %#v", tool.Name, a)
			continue
		}
		if strings.HasPrefix(tool.Name, "remove_") && !*a.DestructiveHint {
			t.Errorf("%v must be marked destructive", tool.Name)
		}
		if strings.HasPrefix(tool.Name, "add_") && *a.DestructiveHint {
			t.Errorf("%v only adds, it must not be marked destructive", tool.Name)
		}
	}
	slices.Sort(names)
	if !slices.Equal(names, writeToolNames) {
		t.Errorf("write tools = %v, want %v", names, writeToolNames)
	}
	// Without Writable they do not exist (TestListTools checks the full list).
}

func TestCreateTask(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "create_task", map[string]any{
		"project": "web", "title": " New ", "description": "Do it", "column": "ready", "category": "bug",
		"owner": "ann", "tags": []string{"agent", " "},
	})
	if res.IsError {
		t.Fatal(text)
	}
	if !strings.Contains(text, `"id":40`) || !strings.Contains(text, `"column":{"id":2,"name":"Ready"}`) {
		t.Errorf("unexpected result: %s", text)
	}
	got := fake.wrote("createTask")
	if len(got) != 1 {
		t.Fatalf("createTask called %d times", len(got))
	}
	p := got[0]
	want := map[string]any{"title": "New", "project_id": 7.0, "column_id": 2.0, "category_id": 3.0, "owner_id": 4.0,
		"creator_id": 9.0, "description": "Do it\n\n---\n*created by Claude*"}
	for k, v := range want {
		if p[k] != v {
			t.Errorf("createTask %v = %#v, want %#v", k, p[k], v)
		}
	}
	if _, ok := p["swimlane_id"]; ok {
		t.Errorf("swimlane_id was not given, it must be left to Kanboard: %v", p)
	}
	if tags, _ := p["tags"].([]any); len(tags) != 1 || tags[0] != "agent" {
		t.Errorf("tags = %#v, want [agent]", p["tags"])
	}
}

func TestCreateTaskWrongNames(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	for field, want := range map[string]string{
		"column":   `choose one of: "Backlog" (1), "Ready" (2)`,
		"category": `no category matches`,
		"owner":    `no project member matches`,
		"project":  `no active project matches`,
	} {
		args := map[string]any{"project": "web", "title": "x", field: "nope"}
		res, text := call(t, session, "create_task", args)
		if !res.IsError || !strings.Contains(text, want) {
			t.Errorf("%v: IsError=%v %q, want an error containing %q", field, res.IsError, text, want)
		}
	}
	if n := len(fake.wrote("createTask")); n != 0 {
		t.Errorf("createTask called %d times for invalid input", n)
	}
}

func TestComments(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "add_comment", map[string]any{"task": "#KB1", "content": "Done."})
	if res.IsError {
		t.Fatal(text)
	}
	c := fake.wrote("createComment")
	if len(c) != 1 || c[0]["user_id"] != 9.0 || c[0]["task_id"] != 1.0 || c[0]["content"] != "Done.\n\n---\n*created by Claude*" {
		t.Errorf("createComment params: %v", c)
	}

	// The agent copied the old footer into the new text; it is not doubled.
	res, text = call(t, session, "update_comment", map[string]any{"comment_id": 20, "content": "new\n\n---\n*created by Claude*"})
	if res.IsError {
		t.Fatal(text)
	}
	u := fake.wrote("updateComment")
	if len(u) != 1 || u[0]["content"] != "new\n\n---\n*created by Claude, modified by Claude*" {
		t.Errorf("updateComment params: %v", u)
	}

	for id, want := range map[int]string{21: "no signature footer", 22: `written by "Ann"`, 23: "not found"} {
		res, text := call(t, session, "update_comment", map[string]any{"comment_id": id, "content": "x"})
		if !res.IsError || !strings.Contains(text, want) {
			t.Errorf("comment %v: IsError=%v %q, want an error containing %q", id, res.IsError, text, want)
		}
	}
	if n := len(fake.wrote("updateComment")); n != 1 {
		t.Errorf("updateComment called %d times, want 1", n)
	}
}

func TestUpdateCommentNeedsSignature(t *testing.T) {
	session, fake, _ := connectWritable(t, "")
	res, text := call(t, session, "update_comment", map[string]any{"comment_id": 20, "content": "x"})
	if !res.IsError || !strings.Contains(text, "needs a signature") {
		t.Errorf("IsError=%v %q", res.IsError, text)
	}
	// Without a signature comments get no footer.
	call(t, session, "add_comment", map[string]any{"task": "1", "content": "plain"})
	if c := fake.wrote("createComment"); len(c) != 1 || c[0]["content"] != "plain" {
		t.Errorf("createComment params: %v", c)
	}
}

func TestUploadFiles(t *testing.T) {
	session, fake, dir := connectWritable(t, "Claude")
	png := append([]byte("\x89PNG\r\n\x1a\n"), make([]byte, 24)...)
	must(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	must(t, os.WriteFile(filepath.Join(dir, "shot.png"), png, 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "sub", "run.log"), []byte("ok"), 0o644))

	res, text := call(t, session, "upload_files", map[string]any{
		"task": "1", "files": []string{"shot.png", filepath.Join(dir, "sub", "run.log")}, "comment": "See:",
	})
	if res.IsError {
		t.Fatal(text)
	}
	files := fake.wrote("createTaskFile")
	if len(files) != 2 || files[0]["filename"] != "shot.png" || files[1]["filename"] != "run.log" ||
		files[0]["project_id"] != 7.0 || files[0]["blob"] != base64.StdEncoding.EncodeToString(png) {
		t.Fatalf("createTaskFile params: %v", files)
	}
	c := fake.wrote("createComment")
	want := "See:\n\n![shot.png](?controller=FileViewerController&action=image&task_id=1&file_id=51)\n\n" +
		"[run.log](?controller=FileViewerController&action=download&task_id=1&file_id=52)\n\n---\n*created by Claude*"
	if len(c) != 1 || c[0]["content"] != want {
		t.Errorf("comment = %q,\nwant      %q", c[0]["content"], want)
	}
}

func TestUploadFilesStaysInDirectory(t *testing.T) {
	session, fake, dir := connectWritable(t, "Claude")
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	must(t, os.WriteFile(secret, []byte("secret"), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "big.txt"), make([]byte, 101), 0o644))
	must(t, os.WriteFile(filepath.Join(dir, "fake.png"), []byte("not an image"), 0o644))
	cases := map[string]string{
		secret:                                "not a file in the upload directory",
		"../" + filepath.Base(outside) + "/x": "not a file in the upload directory",
		"big.txt":                             "more than the 100 bytes",
		"fake.png":                            "not an image",
		"missing.txt":                         "no such file",
		".":                                   "not a file in the upload directory",
	}
	if err := os.Symlink(secret, filepath.Join(dir, "link.txt")); err == nil {
		cases["link.txt"] = "outside the upload directory"
	}
	for path, want := range cases {
		res, text := call(t, session, "upload_files", map[string]any{"task": "1", "files": []string{path}})
		if !res.IsError || !strings.Contains(text, want) {
			t.Errorf("%v: IsError=%v %q, want an error containing %q", path, res.IsError, text, want)
		}
	}
	// A bad file anywhere in the list stops the whole upload before it starts.
	must(t, os.WriteFile(filepath.Join(dir, "ok.txt"), []byte("ok"), 0o644))
	call(t, session, "upload_files", map[string]any{"task": "1", "files": []string{"ok.txt", secret}})
	if n := len(fake.wrote("createTaskFile")); n != 0 {
		t.Errorf("createTaskFile called %d times", n)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

// writeCalls returns the writing procedures that were called, in order.
func (f *fakeKanboard) writeCalls() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	var res []string
	for _, c := range f.calls {
		if !strings.HasPrefix(c, "get") && c != "searchTasks" {
			res = append(res, c)
		}
	}
	return res
}

func TestCreateTaskGroupAssign(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "create_task", map[string]any{
		"project": "web", "title": "Shared", "owner": "ann", "group": "devs", "other_assignees": []string{"ann", "me", "Ann"},
	})
	if res.IsError {
		t.Fatal(text)
	}
	if fake.called("createTask") {
		t.Error("createTask was called instead of createTaskGroupAssign")
	}
	got := fake.wrote("createTaskGroupAssign")
	if len(got) != 1 {
		t.Fatalf("createTaskGroupAssign called %d times", len(got))
	}
	if p := got[0]; p["group_id"] != 2.0 || fmt.Sprint(p["other_assignees"]) != "[4 9]" || p["owner_id"] != 4.0 {
		t.Errorf("createTaskGroupAssign params = %v, want group_id 2, other_assignees [4 9], owner_id 4", p)
	}
	if !strings.Contains(text, `"assigned_group":{"id":2,"name":"Devs"}`) || !strings.Contains(text, `"username":"agent"`) {
		t.Errorf("the result should show the group and the other assignees: %s", text)
	}
}

func TestCreateTaskWithoutGroupAssignPlugin(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	fake.noGroupAssign.Store(true)
	res, text := call(t, session, "create_task", map[string]any{"project": "web", "title": "x", "other_assignees": []string{"ann"}})
	if !res.IsError || !strings.Contains(text, "Group_assign plugin") || !strings.Contains(text, "no task was created") {
		t.Errorf("want an error about the missing plugin, got %s", text)
	}
	if fake.called("createTask") {
		t.Error("the task must not be created without the group fields")
	}
}

func TestAssignTaskOneByOne(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	// Task 3: group Devs (2), other assignees ann (4) and agent (9), no owner.
	res, text := call(t, session, "assign_task", map[string]any{
		"task": "#3", "owner": "Ann", "add_other_assignees": []string{"ann"}, "remove_other_assignees": []string{"agent"},
	})
	if res.IsError {
		t.Fatal(text)
	}
	if calls := fake.writeCalls(); !slices.Equal(calls, []string{"patchTaskGroupAssign", "updateTask"}) {
		t.Errorf("write calls = %v, want patchTaskGroupAssign then updateTask", calls)
	}
	patch := fake.wrote("patchTaskGroupAssign")[0]
	if _, ok := patch["group_id"]; ok || fmt.Sprint(patch["other_assignees"]) != "[4]" {
		t.Errorf("patch = %v, want only other_assignees [4]", patch)
	}
	if u := fake.wrote("updateTask")[0]; u["id"] != 3.0 || u["owner_id"] != 4.0 || len(u) != 2 {
		t.Errorf("updateTask = %v, want only id 3 and owner_id 4", u)
	}
	if !strings.HasPrefix(text, `{"id":3,`) || !strings.Contains(text, `"other_assignees":[`) {
		t.Errorf("unexpected result: %s", text)
	}
}

func TestAssignTaskReplace(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "assign_task", map[string]any{"task": "3", "group": "none", "other_assignees": []string{}})
	if res.IsError {
		t.Fatal(text)
	}
	got := fake.wrote("patchTaskGroupAssign")
	if len(got) != 1 {
		t.Fatalf("patchTaskGroupAssign called %d times", len(got))
	}
	if p := got[0]; p["group_id"] != 0.0 || fmt.Sprint(p["other_assignees"]) != "[]" {
		t.Errorf("patch = %v, want group_id 0 and other_assignees []", p)
	}
	if fake.called("updateTask") {
		t.Error("the owner was not given, updateTask must not be called")
	}
}

func TestAssignTaskNothingToChange(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	res, text := call(t, session, "assign_task", map[string]any{
		"task": "3", "group": "Devs", "add_other_assignees": []string{"4"}, "remove_other_assignees": []string{"me"},
	})
	if res.IsError {
		t.Fatal(text)
	}
	// "me" (9, agent) is an other assignee, so only that changes.
	if p := fake.wrote("patchTaskGroupAssign"); len(p) != 1 || fmt.Sprint(p[0]["other_assignees"]) != "[4]" {
		t.Errorf("patch = %v, want only other_assignees [4]", p)
	}

	session, fake, _ = connectWritable(t, "Claude")
	res, text = call(t, session, "assign_task", map[string]any{"task": "3", "group": "2", "add_other_assignees": []string{"Agent"}})
	if res.IsError {
		t.Fatal(text)
	}
	if calls := fake.writeCalls(); len(calls) != 0 {
		t.Errorf("nothing changes, but %v was called", calls)
	}
}

func TestAssignTaskOwnerWithoutPlugin(t *testing.T) {
	session, fake, _ := connectWritable(t, "Claude")
	fake.noGroupAssign.Store(true)
	res, text := call(t, session, "assign_task", map[string]any{"task": "1", "owner": "me"})
	if res.IsError {
		t.Fatal(text)
	}
	if u := fake.wrote("updateTask"); len(u) != 1 || u[0]["owner_id"] != 9.0 {
		t.Errorf("updateTask = %v, want owner_id 9", u)
	}
	if fake.called("getTaskGroupAssign") {
		t.Error("only the owner changes; the plugin is not needed")
	}
}

func TestAssignTaskRefusedBeforeAnyChange(t *testing.T) {
	cases := []struct {
		name    string
		old     bool
		args    map[string]any
		wantErr string
	}{
		{"old plugin", true, map[string]any{"task": "3", "owner": "ann", "add_other_assignees": []string{"me"}}, "1.8.4 or newer"},
		{"nothing given", false, map[string]any{"task": "3"}, "give owner, group"},
		{"list and one by one", false, map[string]any{"task": "3", "other_assignees": []string{"ann"},
			"add_other_assignees": []string{"me"}}, "not both"},
		{"unknown user", false, map[string]any{"task": "3", "owner": "ann", "remove_other_assignees": []string{"zed"}}, "Agent"},
		{"unknown group", false, map[string]any{"task": "3", "owner": "ann", "group": "qa"}, "Ops"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			session, fake, _ := connectWritable(t, "Claude")
			fake.oldGroupAssign.Store(c.old)
			res, text := call(t, session, "assign_task", c.args)
			if !res.IsError || !strings.Contains(text, c.wantErr) {
				t.Errorf("want an error with %q, got %s", c.wantErr, text)
			}
			if calls := fake.writeCalls(); len(calls) != 0 {
				t.Errorf("nothing may change, but %v was called", calls)
			}
		})
	}
}
