package mcpserver

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

var writeToolNames = []string{
	"add_comment", "add_external_link", "add_task_link", "close_task", "create_task", "move_task", "open_task",
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
