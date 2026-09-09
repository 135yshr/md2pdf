package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestConvert_HTMLFormatWritesDocument covers the criterion that html format
// stops after the HTML stage and emits it to the output path.
func TestConvert_HTMLFormatWritesDocument(t *testing.T) {
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

// TestConvert_HTMLFormatSkipsChromium checks the Playwright stage is never
// reached, which is the whole point of the format.
func TestConvert_HTMLFormatSkipsChromium(t *testing.T) {
	dir := t.TempDir()
	input := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(input, []byte("# T\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	// An unusable interpreter would fail the PDF stage loudly if it ran.
	c := newTestConverter(t, &Config{
		Format:     FormatHTML,
		PythonPath: filepath.Join(dir, "definitely-not-python"),
	})
	if err := c.Convert([]string{input}, filepath.Join(dir, "doc.html")); err != nil {
		t.Fatalf("Convert must not touch the PDF pipeline: %v", err)
	}
}

// TestConvert_HTMLFormatAppliesCustomCSS covers composability: -format html with
// -css must produce HTML carrying the custom rules.
func TestConvert_HTMLFormatAppliesCustomCSS(t *testing.T) {
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

// TestConvert_HTMLFormatPreservesImagePaths pins the agreed behaviour: image
// paths are emitted as written, so they resolve relative to the output file.
func TestConvert_HTMLFormatPreservesImagePaths(t *testing.T) {
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

// TestConvert_HTMLFormatRejectsMultipleInputs keeps html on the same
// single-document contract as pdf and docx.
func TestConvert_HTMLFormatRejectsMultipleInputs(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatHTML})
	err := c.Convert([]string{"a.md", "b.md"}, filepath.Join(t.TempDir(), "out.html"))
	if err == nil {
		t.Fatal("expected an error for multiple inputs with html format")
	}
	if !strings.Contains(err.Error(), "single input") {
		t.Errorf("unexpected error: %v", err)
	}
}

// makeSVGWritingMmdc returns the path to a stub mmdc that writes a recognisable
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

// TestConvert_HTMLFormatInlinesMermaidSVG covers the criterion that HTML output
// carries the diagram inline, like the PDF pipeline it shares the stage with.
func TestConvert_HTMLFormatInlinesMermaidSVG(t *testing.T) {
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
