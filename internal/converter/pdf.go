package converter

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

// playwrightScript is the Python script template executed to print a PDF.
// It uses the Playwright sync API with Chromium.
const playwrightScript = `
from playwright.sync_api import sync_playwright
import sys

html_path = {{.HTMLPath | quote}}
pdf_path  = {{.PDFPath  | quote}}
page_size = {{.PageSize | quote}}
margin    = {
    "top":    {{.MarginTop    | quote}},
    "bottom": {{.MarginBottom | quote}},
    "left":   {{.MarginLeft   | quote}},
    "right":  {{.MarginRight  | quote}},
}

with sync_playwright() as p:
    browser = p.chromium.launch(args=["--no-sandbox", "--disable-setuid-sandbox"])
    page = browser.new_page()
    page.goto("file://" + html_path)
    page.wait_for_load_state("networkidle")
    page.evaluate("document.fonts.ready")
    page.pdf(
        path=pdf_path,
        format=page_size,
        margin=margin,
        print_background=True,
    )
    browser.close()

print("ok")
`

// scriptData holds the values interpolated into playwrightScript.
type scriptData struct {
	HTMLPath     string
	PDFPath      string
	PageSize     string
	MarginTop    string
	MarginBottom string
	MarginLeft   string
	MarginRight  string
}

// printPDF renders htmlPath to a PDF at pdfPath using a headless Chromium
// browser driven by the Playwright Python library.
//
// The function writes a small Python script to the working directory, executes
// it with the system `python3` interpreter, and removes the script afterwards.
func (c *Converter) printPDF(htmlPath, pdfPath string) error {
	// Resolve absolute path so the Python script can reference it reliably.
	absHTML, err := filepath.Abs(htmlPath)
	if err != nil {
		return fmt.Errorf("resolve html path: %w", err)
	}

	// Build the Python script from the template.
	scriptPath := filepath.Join(c.workDir, "print_pdf.py")
	if err := c.writePrintScript(scriptPath, absHTML, pdfPath); err != nil {
		return err
	}

	// Execute the script.
	python, err := c.findPython()
	if err != nil {
		return err
	}

	cmd := exec.Command(python, scriptPath) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("playwright script failed: %w\noutput: %s", err, out)
	}
	c.logf("  playwright output: %s", strings.TrimSpace(string(out)))
	return nil
}

// writePrintScript writes the rendered Python Playwright script to path.
func (c *Converter) writePrintScript(path, htmlPath, pdfPath string) error {
	// Register a custom "quote" function that wraps a string in Python quotes.
	funcMap := template.FuncMap{
		"quote": func(s string) string {
			s = strings.ReplaceAll(s, `\`, `\\`)
			s = strings.ReplaceAll(s, `"`, `\"`)
			return `"` + s + `"`
		},
	}
	tmpl, err := template.New("playwright").Funcs(funcMap).Parse(playwrightScript)
	if err != nil {
		return fmt.Errorf("parse playwright template: %w", err)
	}

	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("create script file: %w", err)
	}
	defer f.Close()

	data := scriptData{
		HTMLPath:     htmlPath,
		PDFPath:      pdfPath,
		PageSize:     c.cfg.PageSize,
		MarginTop:    c.cfg.MarginTop,
		MarginBottom: c.cfg.MarginBottom,
		MarginLeft:   c.cfg.MarginLeft,
		MarginRight:  c.cfg.MarginRight,
	}
	if err := tmpl.Execute(f, data); err != nil {
		return fmt.Errorf("execute playwright template: %w", err)
	}
	return nil
}

// findPython returns a Python 3 interpreter that can import the playwright
// package. When c.cfg.PythonPath is set (via the -python flag or the
// MD2PDF_PYTHON env var), it is used directly after a precheck. Otherwise
// candidates are probed in order — first "python3" and "python" on PATH,
// then well-known absolute locations (active virtualenv, pyenv shims,
// Homebrew, system Python) — and the first interpreter that passes
// `python -c "import playwright"` is selected. Probing absolute paths in
// addition to PATH covers the common case where md2pdf is launched from a
// context (GUI, minimal shell) that does not inherit the user's interactive
// shell PATH and would otherwise fall back to a Python without playwright.
func (c *Converter) findPython() (string, error) {
	if explicit := c.cfg.PythonPath; explicit != "" {
		if err := canImportPlaywright(explicit); err != nil {
			return "", fmt.Errorf("python at %q cannot import playwright: %w", explicit, err)
		}
		c.logf("  python: %s (user-specified)", explicit)
		return explicit, nil
	}

	var failures []string
	seen := make(map[string]bool)
	for _, p := range pythonCandidates() {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		// Skip nonexistent paths silently so unrelated absent locations
		// (e.g. pyenv shim on a Homebrew-only machine) don't pollute the
		// failure list returned to the user.
		if _, err := os.Stat(p); err != nil {
			continue
		}
		if perr := canImportPlaywright(p); perr != nil {
			failures = append(failures, fmt.Sprintf("%s (%v)", p, perr))
			continue
		}
		c.logf("  python: %s (auto-detected)", p)
		return p, nil
	}

	if len(failures) == 0 {
		return "", errors.New("no Python 3 interpreter found; install Python 3 with the playwright package, or pass -python / set MD2PDF_PYTHON")
	}
	return "", fmt.Errorf(
		"no Python interpreter can import playwright: %s; "+
			"install playwright (`pip install playwright`) for the right interpreter, "+
			"or pass -python / set MD2PDF_PYTHON to the interpreter that has it",
		strings.Join(failures, "; "),
	)
}

// pythonCandidates returns the ordered list of interpreter paths findPython
// probes during auto-detection. PATH-resolved interpreters come first to
// preserve the fast path for normal shell invocations; well-known absolute
// locations follow as a safety net for contexts where PATH is minimal.
func pythonCandidates() []string {
	var out []string
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			out = append(out, p)
		}
	}
	return append(out, extraPythonCandidatesFn()...)
}

// extraPythonCandidatesFn produces the absolute-path candidate list. It is a
// var rather than a function literal so tests can swap it without exporting
// internal layout.
var extraPythonCandidatesFn = defaultExtraPythonCandidates

// defaultExtraPythonCandidates returns absolute interpreter paths that exist
// on common macOS / Linux developer setups. The list is ordered from "most
// likely to reflect user intent" (active virtualenv) to "system fallback".
func defaultExtraPythonCandidates() []string {
	var paths []string
	// Active virtualenv: strongest signal of where pip just installed things.
	if v := os.Getenv("VIRTUAL_ENV"); v != "" {
		paths = append(paths,
			filepath.Join(v, "bin", "python3"),
			filepath.Join(v, "bin", "python"),
		)
	}
	// pyenv shims, including anyenv-managed pyenv.
	if home, err := os.UserHomeDir(); err == nil {
		paths = append(paths,
			filepath.Join(home, ".pyenv", "shims", "python3"),
			filepath.Join(home, ".anyenv", "envs", "pyenv", "shims", "python3"),
		)
	}
	// Homebrew (Apple Silicon, Intel/Linux) and system Python.
	paths = append(paths,
		"/opt/homebrew/bin/python3",
		"/usr/local/bin/python3",
		"/usr/bin/python3",
	)
	return paths
}

// canImportPlaywright runs `python -c "import playwright"` against the given
// interpreter and returns nil only when the import succeeds. On failure the
// returned error carries the last non-empty line of the interpreter's output,
// which is typically the ModuleNotFoundError or other concrete reason.
func canImportPlaywright(python string) error {
	cmd := exec.Command(python, "-c", "import playwright") //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err == nil {
		return nil
	}
	if msg := lastNonEmptyLine(string(out)); msg != "" {
		return errors.New(msg)
	}
	return fmt.Errorf("execute python: %w", err)
}

// lastNonEmptyLine returns the last non-empty trimmed line of s, or "" when s
// contains no such line. Used to extract the salient final line of a Python
// traceback (e.g. "ModuleNotFoundError: No module named 'playwright'").
func lastNonEmptyLine(s string) string {
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if line := strings.TrimSpace(lines[i]); line != "" {
			return line
		}
	}
	return ""
}
