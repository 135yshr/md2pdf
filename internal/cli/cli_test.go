package cli

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

// testBuild is the build metadata every test in this package reports.
var testBuild = BuildInfo{Version: "1.2.3", Commit: "abc1234", Date: "2026-01-01"}

// testProgram returns a program invoked under name, writing to buffers the
// caller can assert on, with a diagnose stub so -doctor does not depend on what
// happens to be installed on the machine running the tests.
func testProgram(name string, report converter.Report) (*program, *bytes.Buffer, *bytes.Buffer) {
	var stdout, stderr bytes.Buffer
	p := &program{
		build:    testBuild,
		diagnose: func(*converter.Config) converter.Report { return report },
		stdout:   &stdout,
		stderr:   &stderr,
		name:     name,
		// Tests never read the process stdin, and a terminal there would make
		// the "-" cases depend on how the suite was started.
		defaultFormat:   converter.FormatPDF,
		stdinIsTerminal: false,
	}
	return p, &stdout, &stderr
}

// testProgramWithStdin returns a program invoked as md2pdf whose standard input
// is, or is not, a terminal.
func testProgramWithStdin(stdinIsTerminal bool) *program {
	p, _, _ := testProgram(defaultProgramName, converter.Report{})
	p.stdinIsTerminal = stdinIsTerminal
	return p
}

// parseFlags parses args exactly as the md2pdf binary does. Keeping the old
// free-function spelling lets every test written before the command line moved
// into this package go on doubling as the guard that md2pdf did not change.
func parseFlags(args []string) (*converter.Config, error) {
	p, _, _ := testProgram(defaultProgramName, converter.Report{})
	return p.parseFlags(args)
}

// readyReport is a diagnosis in which every format can run.
func readyReport() converter.Report {
	return converter.Report{
		Tools: []converter.ToolStatus{
			{Name: "Chromium", Path: "/usr/bin/chromium", Found: true, Purpose: "prints the PDF"},
		},
		Formats: []converter.FormatStatus{
			{Format: converter.FormatPDF, Ready: true},
			{Format: converter.FormatConsole, Ready: true},
		},
	}
}

// blockedReport is a diagnosis in which one format cannot run.
func blockedReport() converter.Report {
	return converter.Report{
		Tools: []converter.ToolStatus{
			{Name: "pandoc", Found: false, Purpose: "converts to DOCX", Hint: "brew install pandoc"},
		},
		Formats: []converter.FormatStatus{
			{Format: converter.FormatDOCX, Ready: false, Missing: []string{"pandoc"}},
			{Format: converter.FormatConsole, Ready: true},
		},
	}
}

func TestRunReportsMissingInputAndExitsOne(t *testing.T) {
	p, _, stderr := testProgram(defaultProgramName, converter.Report{})

	if code := p.run(nil); code != 1 {
		t.Errorf("run(nil) = %d, want 1", code)
	}
	if got := stderr.String(); !strings.HasPrefix(got, "md2pdf: at least one input Markdown file is required") {
		t.Errorf("stderr = %q, want it to start with the missing-input error", got)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Error("stderr does not contain the usage summary")
	}
}

func TestParseFlagsVersionFinishesWithExitZero(t *testing.T) {
	p, stdout, stderr := testProgram(defaultProgramName, converter.Report{})

	cfg, err := p.parseFlags([]string{"-version"})
	if cfg != nil {
		t.Errorf("Config = %+v, want nil for -version", cfg)
	}

	var done exitError
	if !errors.As(err, &done) {
		t.Fatalf("err = %v, want an exitError", err)
	}
	if done.code != 0 {
		t.Errorf("exit code = %d, want 0", done.code)
	}

	want := "md2pdf version 1.2.3 (commit: abc1234, built: 2026-01-01)\n"
	if got := stdout.String(); got != want {
		t.Errorf("stdout = %q, want %q", got, want)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr = %q, want empty", stderr.String())
	}
}

func TestParseFlagsDoctorReturnsAnExitCode(t *testing.T) {
	tests := []struct {
		name   string
		report converter.Report
		want   int
	}{
		{"every format ready", readyReport(), 0},
		{"a format is blocked", blockedReport(), 1},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, stdout, _ := testProgram(defaultProgramName, tc.report)

			cfg, err := p.parseFlags([]string{"-doctor"})
			if cfg != nil {
				t.Errorf("Config = %+v, want nil for -doctor", cfg)
			}

			var done exitError
			if !errors.As(err, &done) {
				t.Fatalf("err = %v, want an exitError", err)
			}
			if done.code != tc.want {
				t.Errorf("exit code = %d, want %d", done.code, tc.want)
			}
			if stdout.Len() == 0 {
				t.Error("stdout is empty, want the dependency report")
			}
		})
	}
}

func TestDescribeInputs(t *testing.T) {
	tests := []struct {
		name   string
		inputs []string
		want   string
	}{
		{"one file", []string{"doc.md"}, "doc.md"},
		{"several files", []string{"a.md", "b.md"}, "a.md, b.md"},
		{"standard input", []string{converter.StdinPath}, "standard input"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := describeInputs(tc.inputs); got != tc.want {
				t.Errorf("describeInputs(%q) = %q, want %q", tc.inputs, got, tc.want)
			}
		})
	}
}
