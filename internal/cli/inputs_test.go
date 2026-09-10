package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

// writeMD creates a Markdown file in dir and returns its path.
func writeMD(t *testing.T, dir, name string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte("# "+name+"\n"), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}

func TestResolveInputs(t *testing.T) {
	dir := t.TempDir()
	a := writeMD(t, dir, "a.md")
	b := writeMD(t, dir, "b.md")
	notMD := filepath.Join(dir, "notes.txt")
	if err := os.WriteFile(notMD, []byte("hi\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	missing := filepath.Join(dir, "missing.md")

	tests := []struct {
		name       string
		args       []string
		format     string
		stdinIsTTY bool
		want       []string
		wantErrSub string
	}{
		{
			name:   "single file, pdf",
			args:   []string{a},
			format: converter.FormatPDF,
			want:   []string{a},
		},
		{
			name:   "single file, console",
			args:   []string{a},
			format: converter.FormatConsole,
			want:   []string{a},
		},
		{
			name:   "two files, console",
			args:   []string{a, b},
			format: converter.FormatConsole,
			want:   []string{a, b},
		},
		{
			name:   "same file twice is not deduplicated",
			args:   []string{a, a},
			format: converter.FormatConsole,
			want:   []string{a, a},
		},
		{
			name:   "stdin, console",
			args:   []string{"-"},
			format: converter.FormatConsole,
			want:   []string{"-"},
		},
		{
			name:   "stdin, pdf",
			args:   []string{"-"},
			format: converter.FormatPDF,
			want:   []string{"-"},
		},
		{
			name:       "no arguments",
			args:       nil,
			format:     converter.FormatConsole,
			wantErrSub: "input",
		},
		{
			name:       "two files with pdf is console-only",
			args:       []string{a, b},
			format:     converter.FormatPDF,
			wantErrSub: "console",
		},
		{
			name:       "two files with docx is console-only",
			args:       []string{a, b},
			format:     converter.FormatDOCX,
			wantErrSub: "console",
		},
		{
			name:       "missing file is named",
			args:       []string{a, missing, b},
			format:     converter.FormatConsole,
			wantErrSub: "missing.md",
		},
		{
			name:       "non-markdown file is named",
			args:       []string{a, notMD},
			format:     converter.FormatConsole,
			wantErrSub: "notes.txt",
		},
		{
			name:       "stdin mixed with a file",
			args:       []string{"-", a},
			format:     converter.FormatConsole,
			wantErrSub: "standard input",
		},
		{
			name:       "file mixed with stdin",
			args:       []string{a, "-"},
			format:     converter.FormatConsole,
			wantErrSub: "standard input",
		},
		{
			name:       "stdin twice",
			args:       []string{"-", "-"},
			format:     converter.FormatConsole,
			wantErrSub: "standard input",
		},
		{
			name:       "stdin from a terminal does not block",
			args:       []string{"-"},
			format:     converter.FormatConsole,
			stdinIsTTY: true,
			wantErrSub: "terminal",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := testProgramWithStdin(tc.stdinIsTTY).resolveInputs(tc.args, tc.format)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("resolveInputs(%v) = %v, want an error containing %q",
						tc.args, got, tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveInputs(%v) error = %v", tc.args, err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %v, want %v", got, tc.want)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("input %d = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestParseFlags_StdinRequiresOutputForFileFormats covers the criterion that a
// file format reading stdin has no filename to derive an output name from.
func TestParseFlags_StdinRequiresOutputForFileFormats(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{"pdf from stdin without -o", []string{"-format", "pdf", "-"}, true},
		{"docx from stdin without -o", []string{"-format", "docx", "-"}, true},
		{"default format from stdin without -o", []string{"-"}, true},
		{"pdf from stdin with -o", []string{"-format", "pdf", "-o", "out.pdf", "-"}, false},
		{"docx from stdin with -o", []string{"-format", "docx", "-o", "out.docx", "-"}, false},
		{"console from stdin needs no -o", []string{"-format", "console", "-"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%v) = %+v, want an error", tc.args, cfg)
				}
				if !strings.Contains(err.Error(), "-o") {
					t.Errorf("error does not mention -o: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) error = %v", tc.args, err)
			}
		})
	}
}

func TestParseFlags_MultipleInputs(t *testing.T) {
	dir := t.TempDir()
	a := writeMD(t, dir, "a.md")
	b := writeMD(t, dir, "b.md")

	cfg, err := parseFlags([]string{"-format", "console", a, b})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if len(cfg.InputFiles) != 2 || cfg.InputFiles[0] != a || cfg.InputFiles[1] != b {
		t.Errorf("InputFiles = %v, want [%s %s]", cfg.InputFiles, a, b)
	}
}

// TestParseFlags_SingleInputUnchanged is the regression guard: one path must
// behave exactly as before, including the derived output name.
func TestParseFlags_SingleInputUnchanged(t *testing.T) {
	dir := t.TempDir()
	input := writeMD(t, dir, "doc.md")

	tests := []struct {
		name       string
		args       []string
		wantFormat string
		wantOut    string
	}{
		{"default pdf", []string{input}, converter.FormatPDF, filepath.Join(dir, "doc.pdf")},
		{"docx", []string{"-format", "docx", input}, converter.FormatDOCX, filepath.Join(dir, "doc.docx")},
		{"console", []string{"-format", "console", input}, converter.FormatConsole, ""},
		{"explicit output", []string{"-o", filepath.Join(dir, "r.pdf"), input}, converter.FormatPDF, filepath.Join(dir, "r.pdf")},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseFlags(tc.args)
			if err != nil {
				t.Fatalf("parseFlags(%v): %v", tc.args, err)
			}
			if len(cfg.InputFiles) != 1 || cfg.InputFiles[0] != input {
				t.Errorf("InputFiles = %v, want [%s]", cfg.InputFiles, input)
			}
			if cfg.Format != tc.wantFormat {
				t.Errorf("Format = %q, want %q", cfg.Format, tc.wantFormat)
			}
			if cfg.OutputFile != tc.wantOut {
				t.Errorf("OutputFile = %q, want %q", cfg.OutputFile, tc.wantOut)
			}
		})
	}
}

func TestStdinUsable(t *testing.T) {
	if err := testProgramWithStdin(false).stdinUsable(); err != nil {
		t.Errorf("stdinUsable() with a pipe = %v, want nil", err)
	}
	err := testProgramWithStdin(true).stdinUsable()
	if err == nil {
		t.Fatal("stdinUsable() with a terminal = nil, want an error")
	}
	for _, want := range []string{"terminal", "standard input"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
