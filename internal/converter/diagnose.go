package converter

import (
	"os"
	"os/exec"
)

// ToolStatus is one external dependency's availability.
type ToolStatus struct {
	// Name is how the dependency is referred to in messages.
	Name string
	// Path is where it was found, empty when it was not.
	Path string
	// Found reports whether it is usable.
	Found bool
	// Purpose says what stops working without it.
	Purpose string
	// Hint is how to install it, set when it is missing.
	Hint string
	// Optional marks a dependency whose absence degrades output rather than
	// preventing it, so it never blocks a format.
	Optional bool
	// Conditional marks a dependency needed only for certain documents, so it
	// does not block a format either. When says which documents.
	Conditional bool
	// When describes the circumstances a conditional dependency is needed in.
	When string
}

// FormatStatus is whether one output format can run on this machine.
type FormatStatus struct {
	// Format is the -format value.
	Format string
	// Ready reports whether everything the format needs is present.
	Ready bool
	// Missing names the dependencies that block it.
	Missing []string
}

// Report is the result of inspecting the environment for the tools md2pdf
// drives.
type Report struct {
	Tools   []ToolStatus
	Formats []FormatStatus
}

// Ready reports whether every output format can run for a document that needs
// nothing conditional. A missing optional dependency, such as the CJK font, and
// a missing conditional one, such as mmdc, do not make the report unready —
// verified behavior: a document with no Mermaid blocks prints to PDF with no
// mmdc installed at all.
func (r Report) Ready() bool {
	for _, f := range r.Formats {
		if !f.Ready {
			return false
		}
	}
	return true
}

// diagnoseDeps are the environment lookups Diagnose performs, injected so the
// report can be tested without depending on what the host has installed.
type diagnoseDeps struct {
	chromium   func() (string, error)
	lookPath   func(string) (string, error)
	fontExists func(string) bool
}

// Diagnose inspects the environment and reports which dependencies are present
// and which output formats can run.
//
// It resolves each dependency the same way the conversion pipeline does —
// ChromiumPath for the browser, the configured path or PATH for mmdc and
// pandoc — so the report cannot claim a readiness the pipeline would not
// deliver.
func Diagnose(cfg *Config) Report {
	return diagnose(cfg, diagnoseDeps{
		chromium: ChromiumPath,
		lookPath: exec.LookPath,
		fontExists: func(path string) bool {
			if path == "" {
				return false
			}
			info, err := os.Stat(path)
			return err == nil && info.Mode().IsRegular()
		},
	})
}

// Tool names as they appear in the report, and as the format requirement lists
// refer to them.
const (
	toolChromium = "Chromium"
	toolMmdc     = "mmdc"
	toolPandoc   = "pandoc"
	toolCJKFont  = "Noto CJK font"
)

// diagnose builds the report from injected lookups.
func diagnose(cfg *Config, deps diagnoseDeps) Report {
	browser, browserErr := deps.chromium()
	mmdc := probeTool(deps.lookPath, resolveMmdcName(cfg))
	pandoc := probeTool(deps.lookPath, resolvePandocName(cfg))

	tools := []ToolStatus{
		{
			Name:    toolChromium,
			Path:    browser,
			Found:   browserErr == nil,
			Purpose: "printing PDFs, and rendering Mermaid diagrams via mmdc",
			Hint:    "brew install --cask chromium (Google Chrome also works), or set CHROME_PATH",
		},
		{
			Name:        toolMmdc,
			Path:        mmdc,
			Found:       mmdc != "",
			Purpose:     "Mermaid diagrams in pdf, html and docx output",
			Hint:        "brew install mermaid-cli, or npm install -g @mermaid-js/mermaid-cli",
			Conditional: true,
			When:        "documents containing Mermaid diagrams",
		},
		{
			Name:    toolPandoc,
			Path:    pandoc,
			Found:   pandoc != "",
			Purpose: "-format docx",
			Hint:    "brew install pandoc, or apt install pandoc",
		},
		{
			Name:     toolCJKFont,
			Path:     cfg.FontRegular,
			Found:    deps.fontExists(cfg.FontRegular),
			Purpose:  "Japanese text in pdf and html output",
			Hint:     "brew install --cask font-noto-sans-cjk, or apt install fonts-noto-cjk",
			Optional: true,
		},
	}

	return Report{Tools: tools, Formats: formatStatuses(tools)}
}

// formatRequirements lists what each output format needs **unconditionally**.
//
// The mmdc entry is deliberately absent: it is only reached when a document actually
// contains a Mermaid block, and a document without one converts with no mmdc
// installed. Saying "pdf not ready" because mmdc is missing would overstate the
// problem. It is reported as conditional instead.
//
// HTML requires nothing: it stops before the browser, and only touches one
// through mmdc when there are diagrams to rasterise. PDF needs the browser
// because that is what prints it.
var formatRequirements = map[string][]string{
	FormatConsole: {},
	FormatHTML:    {},
	FormatPDF:     {toolChromium},
	FormatDOCX:    {toolPandoc},
}

// formatOrder is the order formats appear in the report.
var formatOrder = []string{FormatPDF, FormatHTML, FormatDOCX, FormatConsole}

// formatStatuses works out which formats can run given the tools found.
func formatStatuses(tools []ToolStatus) []FormatStatus {
	found := make(map[string]bool, len(tools))
	for _, tool := range tools {
		found[tool.Name] = tool.Found || tool.Optional || tool.Conditional
	}

	statuses := make([]FormatStatus, 0, len(formatOrder))
	for _, format := range formatOrder {
		var missing []string
		for _, need := range formatRequirements[format] {
			if !found[need] {
				missing = append(missing, need)
			}
		}
		statuses = append(statuses, FormatStatus{
			Format:  format,
			Ready:   len(missing) == 0,
			Missing: missing,
		})
	}
	return statuses
}

// probeTool returns where name resolves, or an empty string when it does not.
func probeTool(lookPath func(string) (string, error), name string) string {
	if name == "" {
		return ""
	}
	resolved, err := lookPath(name)
	if err != nil {
		return ""
	}
	return resolved
}

// resolveMmdcName is the mmdc binary to probe: the configured path, or the
// default name.
func resolveMmdcName(cfg *Config) string {
	if cfg.MmdcPath != "" {
		return cfg.MmdcPath
	}
	return "mmdc"
}

// resolvePandocName is the pandoc binary to probe: the configured path, or the
// first default candidate, matching findPandoc's own search.
func resolvePandocName(cfg *Config) string {
	if cfg.PandocPath != "" {
		return cfg.PandocPath
	}
	if len(pandocDefaultPaths) > 0 {
		return pandocDefaultPaths[0]
	}
	return "pandoc"
}
