package cli

import (
	"path/filepath"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

func TestProgramName(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want string
	}{
		{"an absolute path", []string{"/usr/local/bin/md2pdf"}, "md2pdf"},
		{"a relative path", []string{"./mdview"}, "mdview"},
		{"a bare name", []string{"mdview"}, "mdview"},
		{"a Windows executable", []string{"mdview.exe"}, "mdview"},
		{"a shouted Windows executable", []string{"MDVIEW.EXE"}, "MDVIEW"},
		{"a suffixed name", []string{"/opt/bin/mdview-2"}, "mdview-2"},
		{"no argv at all", nil, defaultProgramName},
		{"an empty argv[0]", []string{""}, defaultProgramName},
		{"a path of separators", []string{string(filepath.Separator)}, defaultProgramName},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := programName(tc.argv); got != tc.want {
				t.Errorf("programName(%q) = %q, want %q", tc.argv, got, tc.want)
			}
		})
	}
}

func TestDefaultFormat(t *testing.T) {
	tests := []struct {
		name    string
		program string
		want    string
	}{
		{"md2pdf writes a PDF", "md2pdf", converter.FormatPDF},
		{"mdview renders to the terminal", "mdview", converter.FormatConsole},
		{"the name is matched without case", "MdView", converter.FormatConsole},
		{"a suffixed name is not mdview", "mdview-2", converter.FormatPDF},
		{"an unrelated name keeps the pdf default", "mdcat", converter.FormatPDF},
		{"an empty name keeps the pdf default", "", converter.FormatPDF},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := defaultFormat(tc.program); got != tc.want {
				t.Errorf("defaultFormat(%q) = %q, want %q", tc.program, got, tc.want)
			}
		})
	}
}
