package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveFormat(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		output  string
		want    string
		wantErr bool
	}{
		{"default is pdf", "", "", "pdf", false},
		{"infer pdf from extension", "", "out.pdf", "pdf", false},
		{"infer docx from extension", "", "out.docx", "docx", false},
		{"infer is case-insensitive", "", "out.DOCX", "docx", false},
		{"explicit flag wins over empty ext", "docx", "", "docx", false},
		{"explicit flag normalized", "DOCX", "", "docx", false},
		{"flag and matching extension", "docx", "out.docx", "docx", false},
		{"unknown extension defaults to pdf", "", "out.txt", "pdf", false},
		{"unsupported format errors", "rtf", "", "", true},
		{"conflicting flag and extension errors", "pdf", "out.docx", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFormat(tc.format, tc.output)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveFormat(%q, %q) expected error, got %q", tc.format, tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFormat(%q, %q): %v", tc.format, tc.output, err)
			}
			if got != tc.want {
				t.Errorf("resolveFormat(%q, %q) = %q, want %q", tc.format, tc.output, got, tc.want)
			}
		})
	}
}

func TestParseFlags_DefaultOutputExtensionFollowsFormat(t *testing.T) {
	input := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(input, []byte("# hi"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	t.Run("pdf default", func(t *testing.T) {
		cfg, err := parseFlags([]string{input})
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if cfg.Format != "pdf" {
			t.Errorf("Format = %q, want pdf", cfg.Format)
		}
		if !strings.HasSuffix(cfg.OutputFile, ".pdf") {
			t.Errorf("OutputFile = %q, want .pdf suffix", cfg.OutputFile)
		}
	})

	t.Run("docx via format flag", func(t *testing.T) {
		cfg, err := parseFlags([]string{"-format", "docx", input})
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if cfg.Format != "docx" {
			t.Errorf("Format = %q, want docx", cfg.Format)
		}
		if !strings.HasSuffix(cfg.OutputFile, ".docx") {
			t.Errorf("OutputFile = %q, want .docx suffix", cfg.OutputFile)
		}
	})

	t.Run("docx inferred from output extension", func(t *testing.T) {
		out := filepath.Join(t.TempDir(), "report.docx")
		cfg, err := parseFlags([]string{"-o", out, input})
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if cfg.Format != "docx" {
			t.Errorf("Format = %q, want docx", cfg.Format)
		}
		if cfg.OutputFile != out {
			t.Errorf("OutputFile = %q, want %q", cfg.OutputFile, out)
		}
	})

	t.Run("pandoc path passthrough", func(t *testing.T) {
		cfg, err := parseFlags([]string{"-format", "docx", "-pandoc", "/custom/pandoc", input})
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if cfg.PandocPath != "/custom/pandoc" {
			t.Errorf("PandocPath = %q, want %q", cfg.PandocPath, "/custom/pandoc")
		}
	})
}
