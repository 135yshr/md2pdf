package converter

import (
	"os"
	"strings"
	"testing"
)

func TestBuildCSS_ContainsFontFace(t *testing.T) {
	c := &Converter{
		cfg: &Config{
			FontRegular: "/fonts/NotoSansCJK-Regular.ttc",
			FontBold:    "/fonts/NotoSansCJK-Bold.ttc",
			FontMedium:  "/fonts/NotoSansCJK-Medium.ttc",
		},
	}
	css := c.buildCSS()
	for _, want := range []string{
		"@font-face",
		"Noto Sans JP",
		"font-weight: 400",
		"font-weight: 700",
		"font-weight: 500",
		"NotoSansCJK-Regular.ttc",
	} {
		if !strings.Contains(css, want) {
			t.Errorf("buildCSS() missing %q", want)
		}
	}
}

func TestBuildCSS_NoFontFaceWhenEmpty(t *testing.T) {
	c := &Converter{cfg: &Config{}}
	css := c.buildCSS()
	if strings.Contains(css, "@font-face") {
		t.Error("expected no @font-face when font paths are empty")
	}
}

func TestBuildHTML_CreateFile(t *testing.T) {
	dir := t.TempDir()
	dest := dir + "/out.html"

	c := &Converter{
		cfg:     &Config{FontRegular: "", FontBold: "", FontMedium: ""},
		workDir: dir,
	}
	doc := &parsedDoc{
		HTML: "<h1>Test</h1><p>Hello world</p>",
	}

	if err := c.buildHTML(doc, dest); err != nil {
		t.Fatalf("buildHTML() error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	html := string(data)
	for _, want := range []string{
		"<!DOCTYPE html>",
		"<title>Test</title>",
		"Hello world",
		"body {",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("buildHTML() output missing %q", want)
		}
	}
}

func TestBuildHTML_InjectsSVG(t *testing.T) {
	dir := t.TempDir()
	dest := dir + "/out.html"

	c := &Converter{cfg: &Config{}, workDir: dir}
	block := &mermaidBlock{
		Source:      "flowchart TD\n  A --> B\n",
		SVGContent:  `<svg><text>diagram</text></svg>`,
		Placeholder: "MERMAID_PLACEHOLDER_0",
	}
	doc := &parsedDoc{
		HTML:          "<h1>Flow</h1><!--MERMAID_PLACEHOLDER_0-->",
		mermaidBlocks: []*mermaidBlock{block},
	}

	if err := c.buildHTML(doc, dest); err != nil {
		t.Fatalf("buildHTML() error: %v", err)
	}

	data, err := os.ReadFile(dest)
	if err != nil {
		t.Fatalf("ReadFile() error: %v", err)
	}
	html := string(data)
	if !strings.Contains(html, "diagram-wrapper") {
		t.Error("expected .diagram-wrapper in output")
	}
	if !strings.Contains(html, "<svg>") {
		t.Error("expected SVG content in output")
	}
	if strings.Contains(html, "MERMAID_PLACEHOLDER_0") {
		t.Error("placeholder should have been replaced")
	}
}
