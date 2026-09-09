package converter

import (
	"errors"
	"os/exec"
	"strings"
	"testing"
)

// stubDeps builds a diagnoseDeps where the named tools resolve and everything
// else is absent.
func stubDeps(available map[string]string, chromium string, font bool) diagnoseDeps {
	return diagnoseDeps{
		chromium: func() (string, error) {
			if chromium == "" {
				return "", errors.New("no Chromium executable found")
			}
			return chromium, nil
		},
		lookPath: func(name string) (string, error) {
			if p, ok := available[name]; ok {
				return p, nil
			}
			return "", exec.ErrNotFound
		},
		fontExists: func(string) bool { return font },
	}
}

// toolByName finds a tool in the report, failing when it is absent.
func toolByName(t *testing.T, r Report, name string) ToolStatus {
	t.Helper()
	for _, tool := range r.Tools {
		if strings.EqualFold(tool.Name, name) {
			return tool
		}
	}
	t.Fatalf("report has no tool named %q", name)
	return ToolStatus{}
}

// formatByName finds a format in the report, failing when it is absent.
func formatByName(t *testing.T, r Report, format string) FormatStatus {
	t.Helper()
	for _, f := range r.Formats {
		if f.Format == format {
			return f
		}
	}
	t.Fatalf("report has no format %q", format)
	return FormatStatus{}
}

func TestDiagnose_EverythingPresent(t *testing.T) {
	deps := stubDeps(map[string]string{"mmdc": "/bin/mmdc", "pandoc": "/bin/pandoc"}, "/bin/chrome", true)
	r := diagnose(&Config{}, deps)

	for _, name := range []string{"Chromium", "mmdc", "pandoc"} {
		if tool := toolByName(t, r, name); !tool.Found {
			t.Errorf("%s reported missing", name)
		}
	}
	for _, f := range r.Formats {
		if !f.Ready {
			t.Errorf("format %s reported not ready: missing %v", f.Format, f.Missing)
		}
	}
	if !r.Ready() {
		t.Error("Ready() is false with every dependency present")
	}
}

// TestDiagnose_ConsoleIsAlwaysReady covers the format that needs no external
// tool at all.
func TestDiagnose_ConsoleIsAlwaysReady(t *testing.T) {
	r := diagnose(&Config{}, stubDeps(nil, "", false))

	console := formatByName(t, r, FormatConsole)
	if !console.Ready {
		t.Errorf("console reported not ready on a bare machine: missing %v", console.Missing)
	}
	if len(console.Missing) != 0 {
		t.Errorf("console lists requirements %v, want none", console.Missing)
	}
}

func TestDiagnose_MissingToolsBlockTheRightFormats(t *testing.T) {
	tests := []struct {
		name         string
		available    map[string]string
		chromium     string
		wantNotReady []string
		wantStillOK  []string
	}{
		{
			name:         "nothing installed",
			wantNotReady: []string{FormatPDF, FormatDOCX},
			// html stops before the browser, and console needs nothing.
			wantStillOK: []string{FormatConsole, FormatHTML},
		},
		{
			name:         "only pandoc missing",
			available:    map[string]string{"mmdc": "/bin/mmdc"},
			chromium:     "/bin/chrome",
			wantNotReady: []string{FormatDOCX},
			wantStillOK:  []string{FormatConsole, FormatPDF, FormatHTML},
		},
		{
			name:         "only the browser missing",
			available:    map[string]string{"mmdc": "/bin/mmdc", "pandoc": "/bin/pandoc"},
			wantNotReady: []string{FormatPDF},
			wantStillOK:  []string{FormatConsole, FormatHTML, FormatDOCX},
		},
		{
			// Verified against the binary: a document with no Mermaid blocks
			// prints to PDF with no mmdc installed.
			name:         "only mmdc missing blocks nothing",
			available:    map[string]string{"pandoc": "/bin/pandoc"},
			chromium:     "/bin/chrome",
			wantNotReady: nil,
			wantStillOK:  []string{FormatConsole, FormatPDF, FormatHTML, FormatDOCX},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := diagnose(&Config{}, stubDeps(tc.available, tc.chromium, true))
			for _, format := range tc.wantNotReady {
				if f := formatByName(t, r, format); f.Ready {
					t.Errorf("%s reported ready but should not be", format)
				}
			}
			for _, format := range tc.wantStillOK {
				if f := formatByName(t, r, format); !f.Ready {
					t.Errorf("%s reported not ready: missing %v", format, f.Missing)
				}
			}
		})
	}
}

// TestDiagnose_MissingToolNamesItselfInTheFormat checks the report says which
// tool blocks a format, not merely that it is blocked.
func TestDiagnose_MissingToolNamesItselfInTheFormat(t *testing.T) {
	deps := stubDeps(map[string]string{"mmdc": "/bin/mmdc"}, "/bin/chrome", true)
	r := diagnose(&Config{}, deps)

	docx := formatByName(t, r, FormatDOCX)
	if docx.Ready {
		t.Fatal("docx reported ready without pandoc")
	}
	if len(docx.Missing) != 1 || !strings.EqualFold(docx.Missing[0], "pandoc") {
		t.Errorf("docx missing = %v, want [pandoc]", docx.Missing)
	}
}

// TestDiagnose_MmdcIsReportedAsConditional pins that a missing mmdc is described
// as affecting only documents with diagrams, rather than blocking a format.
func TestDiagnose_MmdcIsReportedAsConditional(t *testing.T) {
	r := diagnose(&Config{}, stubDeps(map[string]string{"pandoc": "/bin/pandoc"}, "/bin/chrome", true))

	mmdc := toolByName(t, r, "mmdc")
	if mmdc.Found {
		t.Fatal("mmdc unexpectedly found")
	}
	if !mmdc.Conditional {
		t.Error("mmdc is not marked conditional, so the report would claim pdf is unusable")
	}
	if mmdc.When == "" {
		t.Error("mmdc is conditional but does not say when it is needed")
	}
	if !r.Ready() {
		t.Error("Ready() is false only because mmdc is missing")
	}
}

// TestDiagnose_EveryToolCarriesAPurposeAndHint checks the report is actionable:
// a missing tool must say what it is for and how to get it.
func TestDiagnose_EveryToolCarriesAPurposeAndHint(t *testing.T) {
	r := diagnose(&Config{}, stubDeps(nil, "", false))

	for _, tool := range r.Tools {
		if tool.Purpose == "" {
			t.Errorf("%s has no purpose", tool.Name)
		}
		if !tool.Found && tool.Hint == "" {
			t.Errorf("%s is missing but offers no install hint", tool.Name)
		}
	}
}

// TestDiagnose_FontDoesNotBlockAnyFormat pins the deliberate choice that a
// missing CJK font degrades rendering rather than preventing it.
func TestDiagnose_FontDoesNotBlockAnyFormat(t *testing.T) {
	deps := stubDeps(map[string]string{"mmdc": "/bin/mmdc", "pandoc": "/bin/pandoc"}, "/bin/chrome", false)
	r := diagnose(&Config{}, deps)

	for _, f := range r.Formats {
		if !f.Ready {
			t.Errorf("format %s blocked by a missing font: missing %v", f.Format, f.Missing)
		}
	}
	if !r.Ready() {
		t.Error("Ready() is false only because a font is missing")
	}
}

// TestDiagnose_ReportsConfiguredPaths checks an explicitly configured tool is
// the one probed, so -doctor reflects the flags in use.
func TestDiagnose_ReportsConfiguredPaths(t *testing.T) {
	deps := stubDeps(map[string]string{"/custom/mmdc": "/custom/mmdc"}, "/bin/chrome", true)
	r := diagnose(&Config{MmdcPath: "/custom/mmdc"}, deps)

	mmdc := toolByName(t, r, "mmdc")
	if !mmdc.Found {
		t.Fatal("a configured mmdc path was not probed")
	}
	if mmdc.Path != "/custom/mmdc" {
		t.Errorf("mmdc path = %q, want /custom/mmdc", mmdc.Path)
	}
}
