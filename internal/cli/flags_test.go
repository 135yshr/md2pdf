package cli

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
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
		{"console format", "console", "", "console", false},
		{"console is case-insensitive", "Console", "", "console", false},
		{"term is an alias for console", "term", "", "console", false},
		{"terminal is an alias for console", "terminal", "", "console", false},
		{"console rejects an output path", "console", "out.txt", "", true},
		{"console rejects a pdf output path", "console", "out.pdf", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFormat(tc.format, tc.output, converter.FormatPDF)
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

// TestResolveFormatWithConsoleDefault covers the program that renders to the
// terminal unless told otherwise: an explicit -format wins, an -o extension
// wins over the name, and an -o path that names no format is an error rather
// than a silently discarded flag.
func TestResolveFormatWithConsoleDefault(t *testing.T) {
	tests := []struct {
		name    string
		format  string
		output  string
		want    string
		wantErr bool
	}{
		{"no flags renders to the terminal", "", "", "console", false},
		{"a pdf output path wins over the name", "", "out.pdf", "pdf", false},
		{"an html output path wins over the name", "", "out.html", "html", false},
		{"a docx output path wins over the name", "", "out.docx", "docx", false},
		{"an output path naming no format errors", "", "notes.txt", "", true},
		{"an explicit format wins", "docx", "out.docx", "docx", false},
		{"an explicit pdf wins with no output path", "pdf", "", "pdf", false},
		{"an explicit console still rejects an output path", "console", "out.txt", "", true},
		{"a conflicting flag and extension still errors", "pdf", "out.docx", "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveFormat(tc.format, tc.output, converter.FormatConsole)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveFormat(%q, %q, console) expected error, got %q", tc.format, tc.output, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveFormat(%q, %q, console): %v", tc.format, tc.output, err)
			}
			if got != tc.want {
				t.Errorf("resolveFormat(%q, %q, console) = %q, want %q", tc.format, tc.output, got, tc.want)
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

func TestParseFlags_ConsoleFormat(t *testing.T) {
	input := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(input, []byte("# hi"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	t.Run("defaults", func(t *testing.T) {
		cfg, err := parseFlags([]string{"-format", "console", input})
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if cfg.Format != "console" {
			t.Errorf("Format = %q, want console", cfg.Format)
		}
		if cfg.OutputFile != "" {
			t.Errorf("OutputFile = %q, want empty for console output", cfg.OutputFile)
		}
		if cfg.ConsoleWidth != 0 {
			t.Errorf("ConsoleWidth = %d, want 0 (follow terminal)", cfg.ConsoleWidth)
		}
		if cfg.ConsoleStyle != "" {
			t.Errorf("ConsoleStyle = %q, want empty (auto)", cfg.ConsoleStyle)
		}
		if !cfg.ConsolePager {
			t.Error("ConsolePager = false, want true by default")
		}
	})

	t.Run("explicit options", func(t *testing.T) {
		cfg, err := parseFlags([]string{
			"-format", "term", "-width", "100", "-style", "dark", "-pager=false", input,
		})
		if err != nil {
			t.Fatalf("parseFlags: %v", err)
		}
		if cfg.Format != "console" {
			t.Errorf("Format = %q, want console", cfg.Format)
		}
		if cfg.ConsoleWidth != 100 {
			t.Errorf("ConsoleWidth = %d, want 100", cfg.ConsoleWidth)
		}
		if cfg.ConsoleStyle != "dark" {
			t.Errorf("ConsoleStyle = %q, want dark", cfg.ConsoleStyle)
		}
		if cfg.ConsolePager {
			t.Error("ConsolePager = true, want false")
		}
	})

	t.Run("rejects an unknown style", func(t *testing.T) {
		_, err := parseFlags([]string{"-format", "console", "-style", "darkk", input})
		if err == nil {
			t.Fatal("expected an error for an unknown style, got nil")
		}
		if !strings.Contains(err.Error(), "unknown console style") {
			t.Errorf("error = %v, want it to mention the unknown style", err)
		}
	})

	t.Run("rejects a negative width", func(t *testing.T) {
		if _, err := parseFlags([]string{"-format", "console", "-width", "-10", input}); err == nil {
			t.Fatal("expected an error for a negative width, got nil")
		}
	})

	t.Run("rejects an output path", func(t *testing.T) {
		_, err := parseFlags([]string{"-format", "console", "-o", "out.txt", input})
		if err == nil {
			t.Fatal("expected an error when -o is combined with console, got nil")
		}
		if !strings.Contains(err.Error(), "-o") {
			t.Errorf("error = %v, want it to mention -o", err)
		}
	})
}

func TestParseFlags_MermaidRenderModes(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(input, []byte("# T\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr bool
	}{
		{"default is empty", []string{"-format", "console", input}, "", false},
		{"auto", []string{"-format", "console", "-mermaid-render", "auto", input}, "auto", false},
		{"image", []string{"-format", "console", "-mermaid-render", "image", input}, "image", false},
		{"source", []string{"-format", "console", "-mermaid-render", "source", input}, "source", false},
		{"ascii", []string{"-format", "console", "-mermaid-render", "ascii", input}, "ascii", false},
		{"unknown value", []string{"-format", "console", "-mermaid-render", "nope", input}, "", true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%v) = %+v, want an error", tc.args, cfg)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) error = %v", tc.args, err)
			}
			if cfg.MermaidRender != tc.want {
				t.Errorf("MermaidRender = %q, want %q", cfg.MermaidRender, tc.want)
			}
		})
	}
}

// TestParseFlags_CollectionFontCarriesLocalFaceNames wires the two halves
// together: finding the collection is not enough, the Config also has to carry
// the face names, or every weight silently resolves to the collection's first
// face (Thin) and the PDF renders with no bold.
func TestParseFlags_CollectionFontCarriesLocalFaceNames(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(input, []byte("# hi"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	font := filepath.Join(dir, "NotoSansCJK.ttc")
	if err := os.WriteFile(font, []byte("font"), 0o644); err != nil {
		t.Fatalf("write font: %v", err)
	}

	cfg, err := parseFlags([]string{"-font", font, input})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	// A weightless collection reuses one file for all three weights, so the
	// local names are the only thing telling them apart.
	for _, tc := range []struct {
		weight string
		got    []string
		path   string
	}{
		{"Regular", cfg.FontLocalRegular, cfg.FontRegular},
		{"Bold", cfg.FontLocalBold, cfg.FontBold},
		{"Medium", cfg.FontLocalMedium, cfg.FontMedium},
	} {
		if tc.path != font {
			t.Errorf("%s weight resolved to %q, want the collection %q", tc.weight, tc.path, font)
		}
		want := []string{"Noto Sans CJK JP " + tc.weight, "NotoSansCJKjp-" + tc.weight}
		if !slices.Equal(tc.got, want) {
			t.Errorf("%s local face names = %v, want %v", tc.weight, tc.got, want)
		}
	}
}

// TestParseFlags_PerWeightFontNeedsNoLocalNames is the other side: a per-weight
// install addresses one face by path already, so nothing is added.
func TestParseFlags_PerWeightFontNeedsNoLocalNames(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(input, []byte("# hi"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}
	for _, name := range []string{"NotoSansCJKjp-Regular.otf", "NotoSansCJKjp-Bold.otf"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("font"), 0o644); err != nil {
			t.Fatalf("write font: %v", err)
		}
	}

	cfg, err := parseFlags([]string{"-font", filepath.Join(dir, "NotoSansCJKjp-Regular.otf"), input})
	if err != nil {
		t.Fatalf("parseFlags: %v", err)
	}

	if cfg.FontLocalRegular != nil || cfg.FontLocalBold != nil || cfg.FontLocalMedium != nil {
		t.Errorf("per-weight font gained local face names: %v / %v / %v",
			cfg.FontLocalRegular, cfg.FontLocalBold, cfg.FontLocalMedium)
	}
	if want := filepath.Join(dir, "NotoSansCJKjp-Bold.otf"); cfg.FontBold != want {
		t.Errorf("FontBold = %q, want the sibling %q", cfg.FontBold, want)
	}
}

// TestParseFlags_MermaidScale covers the -mermaid-scale flag, including the
// combinations that can never do anything. A flag that is silently ignored is
// worse than one that says it cannot apply, so text art and source output
// reject it rather than accepting it and doing nothing.
func TestParseFlags_MermaidScale(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(input, []byte("# T\n"), 0o644); err != nil {
		t.Fatalf("write input: %v", err)
	}

	tests := []struct {
		name    string
		args    []string
		want    float64
		wantErr bool
	}{
		{"default is unset", []string{"-format", "console", input}, 0, false},
		{"half", []string{"-format", "console", "-mermaid-scale", "0.5", input}, 0.5, false},
		{"double", []string{"-format", "console", "-mermaid-scale", "2", input}, 2, false},
		{"with an explicit image mode",
			[]string{"-format", "console", "-mermaid-render", "image", "-mermaid-scale", "0.5", input}, 0.5, false},
		{"out of range", []string{"-format", "console", "-mermaid-scale", "99", input}, 0, true},
		{"negative", []string{"-format", "console", "-mermaid-scale", "-1", input}, 0, true},
		// Text art has no scale: a character is a character.
		{"rejected with ascii",
			[]string{"-format", "console", "-mermaid-render", "ascii", "-mermaid-scale", "0.5", input}, 0, true},
		{"rejected with source",
			[]string{"-format", "console", "-mermaid-render", "source", "-mermaid-scale", "0.5", input}, 0, true},
		// Only console output draws inline images at all.
		{"rejected outside console output",
			[]string{"-format", "html", "-mermaid-scale", "0.5", "-o", filepath.Join(dir, "o.html"), input}, 0, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfg, err := parseFlags(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseFlags(%v) = %+v, want an error", tc.args, cfg)
				}
				if !strings.Contains(err.Error(), "mermaid-scale") {
					t.Errorf("error %q does not name the flag", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) error = %v", tc.args, err)
			}
			if cfg.MermaidScale != tc.want {
				t.Errorf("MermaidScale = %v, want %v", cfg.MermaidScale, tc.want)
			}
		})
	}
}
