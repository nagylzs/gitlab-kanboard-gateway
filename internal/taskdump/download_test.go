package taskdump

import (
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

func TestSanitizeFileName(t *testing.T) {
	cases := map[string]string{
		"claude-kanboard.docx":       "claude-kanboard.docx",
		"Árvíztűrő tükörfúrógép.pdf": "Árvíztűrő tükörfúrógép.pdf",
		`a<b>c:d"e/f\g|h?i*j.txt`:    "a_b_c_d_e_f_g_h_i_j.txt",
		"../../etc/passwd":           "_.._etc_passwd",
		`..\..\Windows\win.ini`:      "_.._Windows_win.ini",
		"report.pdf. . ":             "report.pdf",
		"tab\there\x00.txt":          "tab_here_.txt",
		" ... ":                      "file",
	}
	for in, want := range cases {
		if got := sanitizeFileName(in); got != want {
			t.Errorf("sanitizeFileName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSanitizeFileNameLong(t *testing.T) {
	for _, in := range []string{
		strings.Repeat("é", 300) + ".docx",                       // cut inside two-byte runes
		strings.Repeat("x", 300),                                 // no extension
		strings.Repeat("y", 250) + "." + strings.Repeat("z", 30), // "extension" too long to keep
	} {
		got := sanitizeFileName(in)
		if len(got) > maxFileNameBytes || !utf8.ValidString(got) || !filepath.IsLocal(got) {
			t.Errorf("sanitizeFileName(%.20q...) = %q (%d bytes)", in, got, len(got))
		}
		if strings.HasSuffix(in, ".docx") && !strings.HasSuffix(got, ".docx") {
			t.Errorf("extension lost: %q", got)
		}
	}
}
