package cli

import (
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

func TestRunDoctor_ReportsReadyWhenNothingIsMissing(t *testing.T) {
	report := converter.Report{
		Tools: []converter.ToolStatus{
			{Name: "Chromium", Path: "/bin/chrome", Found: true, Purpose: "printing PDFs"},
		},
		Formats: []converter.FormatStatus{
			{Format: converter.FormatPDF, Ready: true},
			{Format: converter.FormatConsole, Ready: true},
		},
	}

	var out strings.Builder
	if ok := runDoctor(&out, report); !ok {
		t.Error("runDoctor reported not ready with every format ready")
	}
	text := out.String()
	for _, want := range []string{"Chromium", "ok", "/bin/chrome", "pdf", "ready"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "cannot run") {
		t.Errorf("output warns about failures when there are none:\n%s", text)
	}
}

func TestRunDoctor_ReportsWhatBlocksAFormat(t *testing.T) {
	report := converter.Report{
		Tools: []converter.ToolStatus{
			{Name: "pandoc", Found: false, Purpose: "-format docx", Hint: "brew install pandoc"},
		},
		Formats: []converter.FormatStatus{
			{Format: converter.FormatDOCX, Ready: false, Missing: []string{"pandoc"}},
			{Format: converter.FormatConsole, Ready: true},
		},
	}

	var out strings.Builder
	if ok := runDoctor(&out, report); ok {
		t.Error("runDoctor reported ready with a blocked format")
	}
	text := out.String()
	for _, want := range []string{"pandoc", "missing", "brew install pandoc", "not ready", "cannot run"} {
		if !strings.Contains(text, want) {
			t.Errorf("output missing %q:\n%s", want, text)
		}
	}
}

// TestRunDoctor_QualifiesConditionalAndOptionalTools checks a missing tool that
// does not block a format says so, rather than reading like a hard failure.
func TestRunDoctor_QualifiesConditionalAndOptionalTools(t *testing.T) {
	report := converter.Report{
		Tools: []converter.ToolStatus{
			{
				Name: "mmdc", Found: false, Purpose: "Mermaid diagrams",
				Hint: "brew install mermaid-cli", Conditional: true,
				When: "documents containing Mermaid diagrams",
			},
			{
				Name: "Noto CJK font", Found: false, Purpose: "Japanese text",
				Hint: "brew install --cask font-noto-sans-cjk-jp", Optional: true,
			},
		},
		Formats: []converter.FormatStatus{{Format: converter.FormatPDF, Ready: true}},
	}

	var out strings.Builder
	if ok := runDoctor(&out, report); !ok {
		t.Error("a conditional or optional tool must not make the report unready")
	}
	text := out.String()
	if !strings.Contains(text, "only needed for documents containing Mermaid diagrams") {
		t.Errorf("conditional tool is not qualified:\n%s", text)
	}
	if !strings.Contains(text, "output still works without it") {
		t.Errorf("optional tool is not qualified:\n%s", text)
	}
}
