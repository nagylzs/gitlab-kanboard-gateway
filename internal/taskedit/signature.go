package taskedit

import (
	"errors"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// The signature footer is the last part of a task description or comment written by
// the agent, separated from the text by a horizontal rule:
//
//	text
//
//	---
//	*created by Claude, modified by Claude*
//
// The blank line before "---" matters: right below a line of text it would turn that
// line into a heading.

// footer holds the signatures as they appear in the text (markdown-escaped).
type footer struct {
	created  string
	modified string
}

var reFooter = regexp.MustCompile(
	`(?s)^(?:(.*?)\s*\n)?[ \t]*---[ \t]*\r?\n\s*\*(?:created by (.+?))?(?:, )?(?:modified by (.+?))?\*\s*$`)

// splitFooter separates a signature footer from the text before it.
func splitFooter(text string) (body string, f footer, ok bool) {
	m := reFooter.FindStringSubmatch(text)
	if m == nil || (m[2] == "" && m[3] == "") {
		return text, footer{}, false
	}
	return m[1], footer{created: m[2], modified: m[3]}, true
}

// joinFooter appends footer f to body; an empty footer leaves body unchanged.
func joinFooter(body string, f footer) string {
	var parts []string
	if f.created != "" {
		parts = append(parts, "created by "+f.created)
	}
	if f.modified != "" {
		parts = append(parts, "modified by "+f.modified)
	}
	if len(parts) == 0 {
		return body
	}
	line := "*" + strings.Join(parts, ", ") + "*"
	body = strings.TrimRight(body, " \t\r\n")
	if body == "" {
		return "---\n" + line
	}
	return body + "\n\n---\n" + line
}

// signCreated appends the "created by" footer to text written by the agent. A footer
// the agent copied into the text itself is replaced.
func (e *Editor) signCreated(text string) string {
	body, _, _ := splitFooter(text)
	return joinFooter(body, footer{created: mdEscape(e.opts.Signature)})
}

// signModified returns the new text of an edited comment: the agent's text (without
// any footer it copied) with the old footer, extended by "modified by".
func (e *Editor) signModified(oldText, newText string) string {
	body, _, _ := splitFooter(newText)
	_, f, _ := splitFooter(oldText)
	f.modified = mdEscape(e.opts.Signature)
	return joinFooter(body, f)
}

// mdEscape keeps a signature from being read as markdown (and from ending the
// italic footer early).
func mdEscape(s string) string {
	var b strings.Builder
	for _, r := range s {
		if strings.ContainsRune("\\`*_[]<>", r) {
			b.WriteByte('\\')
		}
		b.WriteRune(r)
	}
	return b.String()
}

// ValidateSignature checks a --signature value: one line of at most 100 characters.
func ValidateSignature(s string) error {
	if utf8.RuneCountInString(s) > 100 {
		return errors.New("the signature must be at most 100 characters long")
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return errors.New("the signature must be a single line without control characters")
		}
	}
	if strings.TrimSpace(s) != s {
		return errors.New("the signature must not start or end with a space")
	}
	return nil
}
