package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

func TestParseFlagsDefaultFormatFollowsTheProgramName(t *testing.T) {
	dir := t.TempDir()
	doc := writeMD(t, dir, "doc.md")
	other := writeMD(t, dir, "other.md")

	tests := []struct {
		name       string
		program    string
		args       []string
		wantFormat string
		wantOut    string
		wantInputs int
		wantErrSub string
	}{
		{
			name:       "mdview renders to the terminal",
			program:    consoleProgramName,
			args:       []string{doc},
			wantFormat: converter.FormatConsole,
			wantOut:    "",
			wantInputs: 1,
		},
		{
			name:       "md2pdf still writes a PDF",
			program:    defaultProgramName,
			args:       []string{doc},
			wantFormat: converter.FormatPDF,
			wantOut:    filepath.Join(dir, "doc.pdf"),
			wantInputs: 1,
		},
		{
			name:       "a pdf output path wins over the name",
			program:    consoleProgramName,
			args:       []string{"-o", filepath.Join(dir, "report.pdf"), doc},
			wantFormat: converter.FormatPDF,
			wantOut:    filepath.Join(dir, "report.pdf"),
			wantInputs: 1,
		},
		{
			name:       "an explicit format wins over the name",
			program:    consoleProgramName,
			args:       []string{"-format", "docx", "-o", filepath.Join(dir, "x.docx"), doc},
			wantFormat: converter.FormatDOCX,
			wantOut:    filepath.Join(dir, "x.docx"),
			wantInputs: 1,
		},
		{
			name:       "mdview takes several documents without a format flag",
			program:    consoleProgramName,
			args:       []string{doc, other},
			wantFormat: converter.FormatConsole,
			wantOut:    "",
			wantInputs: 2,
		},
		{
			name:       "mdview reads standard input without needing -o",
			program:    consoleProgramName,
			args:       []string{converter.StdinPath},
			wantFormat: converter.FormatConsole,
			wantOut:    "",
			wantInputs: 1,
		},
		{
			name:       "an output path naming no format is refused",
			program:    consoleProgramName,
			args:       []string{"-o", filepath.Join(dir, "notes.txt"), doc},
			wantErrSub: "-format",
		},
		{
			name:       "md2pdf still refuses several documents",
			program:    defaultProgramName,
			args:       []string{doc, other},
			wantErrSub: "console",
		},
		{
			name:       "md2pdf still requires -o for standard input",
			program:    defaultProgramName,
			args:       []string{converter.StdinPath},
			wantErrSub: "-o is required",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, _, _ := testProgram(tc.program, converter.Report{})
			p.defaultFormat = defaultFormat(tc.program)

			cfg, err := p.parseFlags(tc.args)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("parseFlags(%v) as %s = %+v, want an error containing %q",
						tc.args, tc.program, cfg, tc.wantErrSub)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseFlags(%v) as %s: %v", tc.args, tc.program, err)
			}
			if cfg.Format != tc.wantFormat {
				t.Errorf("Format = %q, want %q", cfg.Format, tc.wantFormat)
			}
			if cfg.OutputFile != tc.wantOut {
				t.Errorf("OutputFile = %q, want %q", cfg.OutputFile, tc.wantOut)
			}
			if len(cfg.InputFiles) != tc.wantInputs {
				t.Errorf("InputFiles = %v, want %d of them", cfg.InputFiles, tc.wantInputs)
			}
		})
	}
}

func TestVersionLineNamesTheInvokedProgram(t *testing.T) {
	p, stdout, _ := testProgram(consoleProgramName, converter.Report{})

	if _, err := p.parseFlags([]string{"-version"}); err == nil {
		t.Fatal("parseFlags(-version) = nil error, want an exitError")
	}
	want := "mdview version 1.2.3 (commit: abc1234, built: 2026-01-01)\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
}

func TestStdinErrorNamesTheInvokedProgram(t *testing.T) {
	p, _, _ := testProgram(consoleProgramName, converter.Report{})
	p.stdinIsTerminal = true

	err := p.stdinUsable()
	if err == nil {
		t.Fatal("stdinUsable() with a terminal = nil, want an error")
	}
	if !strings.Contains(err.Error(), "cat doc.md | mdview -") {
		t.Errorf("error %q does not suggest the invoked program", err)
	}
	if strings.Contains(err.Error(), defaultProgramName) {
		t.Errorf("error %q names md2pdf, but the program was invoked as mdview", err)
	}
}

func TestPrintUsageForMDView(t *testing.T) {
	p, _, stderr := testProgram(consoleProgramName, converter.Report{})
	p.defaultFormat = defaultFormat(consoleProgramName)

	p.printUsage()

	got := stderr.String()
	if !strings.HasPrefix(got, "\nUsage:\n  mdview [options] <input.md>...\n") {
		t.Errorf("usage does not open with the mdview synopsis:\n%s", got)
	}
	for _, want := range []string{
		"default: console;",
		"\nExamples:\n  mdview document.md\n",
		"  -mermaid-render <mode>",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
	// The command lines an mdview user is shown must be mdview's. The prose is
	// allowed to mention md2pdf — it says which program this is — so only an
	// indented command line counts.
	if strings.Contains(got, "\n  "+defaultProgramName+" ") {
		t.Errorf("usage shows md2pdf command lines:\n%s", got)
	}
	if strings.Contains(got, "%!") {
		t.Errorf("usage contains a formatting error:\n%s", got)
	}
}

func TestPrintUsageForAnUnknownNameLooksLikeMD2PDF(t *testing.T) {
	p, _, stderr := testProgram("mdcat", converter.Report{})
	p.defaultFormat = defaultFormat("mdcat")

	p.printUsage()

	got := stderr.String()
	if !strings.HasPrefix(got, "\nUsage:\n  mdcat [options] <input.md>\n") {
		t.Errorf("usage does not open with the pdf synopsis under the invoked name:\n%s", got)
	}
	if !strings.Contains(got, "default: pdf;") {
		t.Error("usage does not report pdf as the default format")
	}
}
