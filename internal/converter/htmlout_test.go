package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestHTMLOutput_WritesDocument covers the criterion that html format
// stops after the HTML stage and emits it to the output path.
func TestHTMLOutput_WritesDocument(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	out := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(input, []byte("# Title\n\nBody text.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := newTestConverter(t, &Config{Format: FormatHTML})
	if err := c.Convert([]string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	page := string(data)
	for _, want := range []string{"<!DOCTYPE html>", "<title>Title</title>", "Body text.", "<style>", "box-sizing"} {
		if !strings.Contains(page, want) {
			t.Errorf("HTML output missing %q", want)
		}
	}
}

// TestHTMLOutput_SkipsChromium checks the Playwright stage is never
// reached, which is the whole point of the format.
func TestHTMLOutput_SkipsChromium(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(input, []byte("# T\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// An unusable page size would fail the print stage loudly if it ran, so a
	// clean return proves the pipeline stopped before the browser.
	c := newTestConverter(t, &Config{
		Format:   FormatHTML,
		PageSize: "definitely-not-a-paper-size",
	})
	if err := c.Convert([]string{input}, filepath.Join(dir, "doc.html")); err != nil {
		t.Fatalf("Convert must not touch the PDF pipeline: %v", err)
	}
}

// TestHTMLOutput_AppliesCustomCSS covers composability: -format html with
// -css must produce HTML carrying the custom rules.
func TestHTMLOutput_AppliesCustomCSS(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	css := filepath.Join(dir, "brand.css")
	out := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(input, []byte("# T\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(css, []byte("body { color: rebeccapurple; }\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := newTestConverter(t, &Config{Format: FormatHTML, CSSFiles: []string{css}})
	if err := c.Convert([]string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "rebeccapurple") {
		t.Error("custom CSS missing from the HTML output")
	}
}

// TestHTMLOutput_PreservesImagePaths pins the agreed behavior: image
// paths are emitted as written, so they resolve relative to the output file.
func TestHTMLOutput_PreservesImagePaths(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	out := filepath.Join(dir, "doc.html")
	if err := os.WriteFile(input, []byte("# T\n\n![shot](./assets/img.png)\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := newTestConverter(t, &Config{Format: FormatHTML})
	if err := c.Convert([]string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "./assets/img.png") {
		t.Errorf("image path was rewritten; want it preserved verbatim:\n%s", data)
	}
}

// TestHTMLOutput_RejectsMultipleInputs keeps html on the same
// single-document contract as pdf and docx.
func TestHTMLOutput_RejectsMultipleInputs(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatHTML})
	err := c.Convert([]string{"a.md", "b.md"}, filepath.Join(t.TempDir(), "out.html"))
	if err == nil {
		t.Fatal("expected an error for multiple inputs with html format")
	}
	if !strings.Contains(err.Error(), "single input") {
		t.Errorf("unexpected error: %v", err)
	}
}

// makeSVGWritingMmdc returns the path to a stub mmdc that writes a recognizable
// SVG to whatever -o names, so the Mermaid stage can run without the real CLI.
func makeSVGWritingMmdc(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "mmdc")
	script := `#!/bin/sh
out=""
prev=""
for a in "$@"; do
  if [ "$prev" = "-o" ]; then out="$a"; fi
  prev="$a"
done
[ -n "$out" ] || exit 2
printf '<svg id="stub-diagram" xmlns="http://www.w3.org/2000/svg"></svg>' > "$out"
exit 0
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write mmdc stub: %v", err)
	}
	return path
}

// TestHTMLOutput_InlinesMermaidSVG covers the criterion that HTML output
// carries the diagram inline, like the PDF pipeline it shares the stage with.
func TestHTMLOutput_InlinesMermaidSVG(t *testing.T) {
	skipOnWindows(t)

	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	out := filepath.Join(dir, "doc.html")
	md := "# T\n\n```mermaid\nflowchart LR\n  A --> B\n```\n"
	if err := os.WriteFile(input, []byte(md), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := newTestConverter(t, &Config{
		Format:   FormatHTML,
		MmdcPath: makeSVGWritingMmdc(t),
		// A non-empty value short-circuits Chromium detection, which this test
		// has no need for.
		PuppeteerConfig: filepath.Join(dir, "puppeteer.json"),
	})
	if err := c.Convert([]string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	page := string(data)
	if !strings.Contains(page, `id="stub-diagram"`) {
		t.Errorf("diagram SVG was not inlined into the HTML:\n%s", page)
	}
	if !strings.Contains(page, "diagram-wrapper") {
		t.Error("diagram wrapper missing from the HTML")
	}
	if strings.Contains(page, "flowchart LR") {
		t.Error("Mermaid source leaked into the HTML alongside the diagram")
	}
}
