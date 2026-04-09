package converter

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
	"strings"
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
	python, err := findPython()
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

// findPython returns the path to the Python 3 interpreter.
func findPython() (string, error) {
	for _, name := range []string{"python3", "python"} {
		if p, err := exec.LookPath(name); err == nil {
			return p, nil
		}
	}
	return "", fmt.Errorf("python3 not found in PATH; install Python 3 with the playwright package")
}
