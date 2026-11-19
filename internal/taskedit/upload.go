package taskedit

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
)

type UploadResult struct {
	TaskId    int            `json:"task_id"`
	Files     []UploadedFile `json:"files"`
	CommentId int            `json:"comment_id"`
	Comment   string         `json:"comment"` // as stored, with the file list and the signature footer
}

type UploadedFile struct {
	FileId    int    `json:"file_id"`
	Name      string `json:"name"`
	SizeBytes int    `json:"size_bytes"`
	Path      string `json:"path"` // the local file that was uploaded
}

// Kanboard decides by the extension whether an attachment is an image (and makes a
// thumbnail of it).
var imageExtensions = []string{".png", ".jpg", ".jpeg", ".gif", ".webp"}

type localFile struct {
	path string // absolute
	name string
	data []byte
}

// UploadFiles attaches local files to a task and posts one comment about them: the
// given text (may be empty), then the files, images shown inline, then the signature.
//
// Files are read only below Options.UploadDir, through an os.Root, so neither a path
// with ".." nor a symlink can make it read (and publish) a file elsewhere. Every file
// is checked before the first one is uploaded.
func (e *Editor) UploadFiles(taskRef string, paths []string, text string) (*UploadResult, error) {
	if e.opts.UploadDir == "" {
		return nil, errors.New("this server has no upload directory; start it with --upload-dir")
	}
	if len(paths) == 0 {
		return nil, errors.New("give at least one file")
	}
	t, err := e.task(taskRef)
	if err != nil {
		return nil, err
	}
	files, err := e.readUploads(paths)
	if err != nil {
		return nil, err
	}
	res := &UploadResult{TaskId: t.Id, Files: []UploadedFile{}}
	for _, f := range files {
		id, err := kanboard.CreateTaskFile(e.cfg, t.ProjectId, t.Id, f.name, f.data)
		if err != nil {
			err = fmt.Errorf("createTaskFile %q: %w", f.name, err)
			if len(res.Files) > 0 {
				err = fmt.Errorf("%w; already uploaded without a comment: %v", err, uploadedNames(res.Files))
			}
			return nil, err
		}
		res.Files = append(res.Files, UploadedFile{FileId: id, Name: f.name, SizeBytes: len(f.data), Path: f.path})
	}
	c, err := e.addComment(t.Id, e.signCreated(uploadComment(t.Id, text, res.Files)))
	if err != nil {
		return nil, fmt.Errorf("the files were uploaded (%v), but the comment failed: %w", uploadedNames(res.Files), err)
	}
	res.CommentId, res.Comment = c.CommentId, c.Content
	return res, nil
}

func (e *Editor) readUploads(paths []string) ([]localFile, error) {
	root, err := os.OpenRoot(e.opts.UploadDir)
	if err != nil {
		return nil, fmt.Errorf("upload directory: %w", err)
	}
	defer root.Close()
	files := make([]localFile, 0, len(paths))
	for _, p := range paths {
		rel, err := e.uploadRel(p)
		if err != nil {
			return nil, err
		}
		data, err := readFile(root, rel, e.opts.MaxUploadBytes)
		if err != nil {
			return nil, fmt.Errorf("%v: %w", p, err)
		}
		name := filepath.Base(rel)
		if isImage(name) && !strings.HasPrefix(http.DetectContentType(data), "image/") {
			// Kanboard would crash making a thumbnail of it.
			return nil, fmt.Errorf("%v: the name says image, but the content is not an image", p)
		}
		files = append(files, localFile{path: filepath.Join(e.opts.UploadDir, rel), name: name, data: data})
	}
	return files, nil
}

// uploadRel turns a path given by the agent (absolute, or relative to the upload
// directory) into a path relative to the upload directory.
func (e *Editor) uploadRel(p string) (string, error) {
	rel := strings.TrimSpace(p)
	if filepath.IsAbs(rel) {
		var err error
		if rel, err = filepath.Rel(e.opts.UploadDir, filepath.Clean(rel)); err != nil {
			return "", fmt.Errorf("%v: %w", p, err)
		}
	}
	if rel == "" || rel == "." || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("%v is not a file in the upload directory %v; only files below it can be uploaded "+
			"(copy the file there first)", p, e.opts.UploadDir)
	}
	return rel, nil
}

func readFile(root *os.Root, rel string, maxBytes int64) ([]byte, error) {
	f, err := root.Open(rel)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, errors.New("no such file in the upload directory")
		}
		if strings.Contains(err.Error(), "escapes") {
			return nil, errors.New("a symlink in the path points outside the upload directory")
		}
		return nil, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, errors.New("not a regular file")
	}
	if st.Size() == 0 {
		return nil, errors.New("the file is empty")
	}
	if maxBytes > 0 && st.Size() > maxBytes {
		return nil, fmt.Errorf("the file is %v bytes, more than the %v bytes this server uploads", st.Size(), maxBytes)
	}
	return io.ReadAll(f)
}

// uploadComment lists the uploaded files below text: images as inline images, other
// files as download links. Both use the relative URLs of the Kanboard web UI, which
// get_task turns into absolute ones (and into attachment_ids).
func uploadComment(taskId int, text string, files []UploadedFile) string {
	var b strings.Builder
	if text = strings.TrimSpace(text); text != "" {
		b.WriteString(text)
		b.WriteString("\n\n")
	} else if len(files) == 1 {
		b.WriteString("New file:\n\n")
	} else {
		b.WriteString("New files:\n\n")
	}
	for i, f := range files {
		if i > 0 {
			b.WriteString("\n\n")
		}
		action, prefix := "download", ""
		if isImage(f.Name) {
			action, prefix = "image", "!"
		}
		fmt.Fprintf(&b, "%s[%s](?controller=FileViewerController&action=%s&task_id=%v&file_id=%v)",
			prefix, mdLinkText(f.Name), action, taskId, f.FileId)
	}
	return b.String()
}

func isImage(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range imageExtensions {
		if ext == e {
			return true
		}
	}
	return false
}

func mdLinkText(s string) string {
	return strings.NewReplacer(`\`, `\\`, `[`, `\[`, `]`, `\]`).Replace(s)
}

func uploadedNames(files []UploadedFile) string {
	s := make([]string, len(files))
	for i, f := range files {
		s[i] = fmt.Sprintf("%q (file %v)", f.Name, f.FileId)
	}
	return strings.Join(s, ", ")
}
