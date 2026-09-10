package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

func TestResolveFormat_HTML(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		output  string
		want    string
		wantErr bool
	}{
		{"explicit html", "html", "", converter.FormatHTML, false},
		{"html with matching extension", "html", "out.html", converter.FormatHTML, false},
		{"inferred from .html extension", "", "out.html", converter.FormatHTML, false},
		{"inferred from .htm extension", "", "out.htm", converter.FormatHTML, false},
		{"html conflicts with .pdf output", "html", "out.pdf", "", true},
		{"pdf conflicts with .html output", "pdf", "out.html", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFormat(tc.format, tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveFormat(%q, %q) = %q, want an error", tc.format, tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFormat(%q, %q) error = %v", tc.format, tc.output, err)
			}
			if got != tc.want {
				t.Errorf("resolveFormat(%q, %q) = %q, want %q", tc.format, tc.output, got, tc.want)
			}
		})
	}
}

func TestParseFlags_HTMLOutputDefaultsToInputName(t *testing.T) {
	dir := t.TempDir()
	input := writeMD(t, dir, "doc.md")

	cfg, err := parseFlags([]string{"-format", "html", input})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if cfg.Format != converter.FormatHTML {
		t.Errorf("Format = %q, want html", cfg.Format)
	}
	if want := filepath.Join(dir, "doc.html"); cfg.OutputFile != want {
		t.Errorf("OutputFile = %q, want %q", cfg.OutputFile, want)
	}
}

func TestParseFlags_HTMLFromStdinRequiresOutput(t *testing.T) {
	if _, err := parseFlags([]string{"-format", "html", "-"}); err == nil {
		t.Fatal("expected an error: html from stdin has no filename to derive -o from")
	}
	if _, err := parseFlags([]string{"-format", "html", "-o", "out.html", "-"}); err != nil {
		t.Errorf("html from stdin with -o should be accepted: %v", err)
	}
}

func TestParseFlags_CSSFlagIsRepeatable(t *testing.T) {
	dir := t.TempDir()
	input := writeMD(t, dir, "doc.md")
	first := filepath.Join(dir, "base.css")
	second := filepath.Join(dir, "client.css")
	for _, p := range []string{first, second} {
		if err := os.WriteFile(p, []byte("body{}\n"), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	cfg, err := parseFlags([]string{"-css", first, "-css", second, input})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if len(cfg.CSSFiles) != 2 || cfg.CSSFiles[0] != first || cfg.CSSFiles[1] != second {
		t.Errorf("CSSFiles = %v, want [%s %s] in that order", cfg.CSSFiles, first, second)
	}
}

func TestParseFlags_CSSMissingFileIsAnError(t *testing.T) {
	dir := t.TempDir()
	input := writeMD(t, dir, "doc.md")
	missing := filepath.Join(dir, "nope.css")

	_, err := parseFlags([]string{"-css", missing, input})
	if err == nil {
		t.Fatal("expected an error for a missing stylesheet")
	}
	if !strings.Contains(err.Error(), "nope.css") {
		t.Errorf("error %q does not name the missing file", err)
	}
}

// TestParseFlags_NoCSSLeavesConfigEmpty is the regression guard for the default
// path: without -css nothing about the configuration changes.
func TestParseFlags_NoCSSLeavesConfigEmpty(t *testing.T) {
	dir := t.TempDir()
	input := writeMD(t, dir, "doc.md")

	cfg, err := parseFlags([]string{input})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}
	if len(cfg.CSSFiles) != 0 {
		t.Errorf("CSSFiles = %v, want empty", cfg.CSSFiles)
	}
	if cfg.Format != converter.FormatPDF {
		t.Errorf("Format = %q, want pdf", cfg.Format)
	}
}
