package taskdump

import (
	"fmt"
	"math/rand/v2"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/nagylzs/gitlab-kanboard-gateway/internal/config"
	"github.com/nagylzs/gitlab-kanboard-gateway/internal/kanboard"
)

// SavedFile describes an attachment written by SaveAttachment.
type SavedFile struct {
	Path     string // absolute
	Size     int
	MimeType string
}

// SaveAttachment downloads attachment f of task taskId into
// <dir>/<taskId>/<fileId>-<sanitized name>, replacing an earlier copy. Attachments
// larger than maxBytes (0 = no limit) are skipped with an error.
//
// The name comes from whoever uploaded the attachment, so everything below dir is
// written through an os.Root: neither the name nor a symlink or junction inside dir
// can make it write outside dir.
func SaveAttachment(cfg config.KanboardConfig, dir string, maxBytes int64, taskId int, f kanboard.KbResponseTaskFile) (*SavedFile, error) {
	if maxBytes > 0 && f.Size > maxBytes {
		return nil, fmt.Errorf("skipped: size %v exceeds limit %v bytes", f.Size, maxBytes)
	}
	rel := filepath.Join(strconv.Itoa(taskId), fmt.Sprintf("%v-%v", f.Id, sanitizeFileName(f.Name)))
	if !filepath.IsLocal(rel) {
		return nil, fmt.Errorf("unsafe file name %q", f.Name)
	}
	data, err := kanboard.DownloadTaskFile(cfg, f.Id)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	root, err := os.OpenRoot(dir)
	if err != nil {
		return nil, err
	}
	defer root.Close()
	if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return nil, err
	}
	// A temporary name first, so that the final name never holds a partial file.
	tmp := fmt.Sprintf("%v.%08x.part", rel, rand.Uint32())
	if err := root.WriteFile(tmp, data, 0o644); err != nil {
		_ = root.Remove(tmp)
		return nil, err
	}
	if err := root.Rename(tmp, rel); err != nil {
		_ = root.Remove(tmp)
		return nil, err
	}
	path, err := filepath.Abs(filepath.Join(dir, rel))
	if err != nil {
		path = filepath.Join(dir, rel)
	}
	return &SavedFile{Path: path, Size: len(data), MimeType: ContentType(f.Name, data)}, nil
}

// ContentType guesses the MIME type of an attachment from the file extension, then
// from the content.
func ContentType(name string, data []byte) string {
	if t := mime.TypeByExtension(strings.ToLower(filepath.Ext(name))); t != "" {
		return t
	}
	return http.DetectContentType(data)
}

var unsafeFileChars = regexp.MustCompile(`[^\p{L}\p{N}._ -]+`)

// Longest sanitized name kept, in bytes; file systems allow 255 for the whole name,
// which also holds the "<fileId>-" prefix and the temporary ".part" suffix.
const maxFileNameBytes = 200

// sanitizeFileName makes an attachment name safe as a single file name on Windows and
// Unix: no path separators, no characters Windows forbids, no trailing dots or spaces
// (Windows drops them), and not too long. The "<fileId>-" prefix that callers add
// rules out reserved device names such as CON or NUL.
func sanitizeFileName(name string) string {
	// keep the whole name (Kanboard allows "/" in names), just make it safe
	name = unsafeFileChars.ReplaceAllString(name, "_")
	name = strings.Trim(name, ". ")
	if len(name) > maxFileNameBytes {
		ext := filepath.Ext(name)
		if len(ext) > 20 {
			ext = ""
		}
		stem := name[:len(name)-len(ext)]
		cut := maxFileNameBytes - len(ext)
		for cut > 0 && !utf8.RuneStart(stem[cut]) {
			cut--
		}
		name = strings.TrimRight(stem[:cut], ". ") + ext
	}
	if name == "" {
		return "file"
	}
	return name
}
