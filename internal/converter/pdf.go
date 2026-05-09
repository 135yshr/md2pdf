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
// "python3" and "python" are probed on PATH and the first interpreter that
// passes `python -c "import playwright"` is selected. Returning an error
// before invoking the print script lets the caller surface which interpreter
// failed, instead of a generic ModuleNotFoundError from deep inside the
// Playwright script.
func (c *Converter) findPython() (string, error) {
	if explicit := c.cfg.PythonPath; explicit != "" {
		if err := canImportPlaywright(explicit); err != nil {
			return "", fmt.Errorf("python at %q cannot import playwright: %w", explicit, err)
		}
		c.logf("  python: %s (user-specified)", explicit)
		return explicit, nil
	}

	var failures []string
	for _, name := range []string{"python3", "python"} {
		p, err := exec.LookPath(name)
		if err != nil {
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
		return "", errors.New("python3 not found in PATH; install Python 3 with the playwright package")
	}
	return "", fmt.Errorf(
		"no Python interpreter on PATH can import playwright: %s; "+
			"install playwright (`pip install playwright`) for the right interpreter, "+
			"or pass -python / set MD2PDF_PYTHON to the interpreter that has it",
		strings.Join(failures, "; "),
	)
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
