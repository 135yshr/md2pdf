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
	css, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS: %v", err)
	}
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
	css, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS: %v", err)
	}
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

func TestBuildHTML_InjectsImageWhenImagePathSet(t *testing.T) {
	dir := t.TempDir()
	dest := dir + "/out.html"

	c := &Converter{cfg: &Config{}, workDir: dir}
	block := &mermaidBlock{
		Source:      "flowchart TD\n  A --> B\n",
		ImagePath:   "diagram_0.png",
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
	if !strings.Contains(html, `<img src="diagram_0.png"`) {
		t.Errorf("expected <img> referencing the PNG, got: %s", html)
	}
	if strings.Contains(html, "MERMAID_PLACEHOLDER_0") {
		t.Error("placeholder should have been replaced")
	}
}

// TestBuildCSS_AppendsCustomCSSAfterBuiltIn covers the documented precedence:
// built-in stylesheet first, user CSS after it, so the user's rules win.
func TestBuildCSS_AppendsCustomCSSAfterBuiltIn(t *testing.T) {
	dir := t.TempDir()
	custom := dir + "/brand.css"
	if err := os.WriteFile(custom, []byte("body { color: rebeccapurple; }\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := &Converter{cfg: &Config{CSSFiles: []string{custom}}}
	css, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS: %v", err)
	}

	iBuiltIn := strings.Index(css, "box-sizing")
	iCustom := strings.Index(css, "rebeccapurple")
	if iBuiltIn < 0 {
		t.Fatal("built-in stylesheet missing")
	}
	if iCustom < 0 {
		t.Fatalf("custom CSS missing:\n%s", css)
	}
	if iCustom < iBuiltIn {
		t.Error("custom CSS came before the built-in stylesheet, so it would lose the cascade")
	}
}

// TestBuildCSS_UnchangedWithoutCustomCSS is the regression guard: with no -css
// the stylesheet must be exactly what it was before this feature.
func TestBuildCSS_UnchangedWithoutCustomCSS(t *testing.T) {
	c := &Converter{cfg: &Config{FontRegular: "/fonts/R.ttc"}}
	css, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS: %v", err)
	}
	want := fontFace("Noto Sans JP", 400, "/fonts/R.ttc") + baseCSS
	if css != want {
		t.Errorf("stylesheet drifted with no -css given.\n got: %q\nwant: %q", css, want)
	}
}

// TestBuildCSS_MultipleFilesInOrder covers layering several stylesheets, where
// the last one given has to win.
func TestBuildCSS_MultipleFilesInOrder(t *testing.T) {
	dir := t.TempDir()
	first := dir + "/base.css"
	second := dir + "/client.css"
	if err := os.WriteFile(first, []byte("body { color: red; } /* FIRST */\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(second, []byte("body { color: blue; } /* SECOND */\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := &Converter{cfg: &Config{CSSFiles: []string{first, second}}}
	css, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS: %v", err)
	}

	iFirst := strings.Index(css, "FIRST")
	iSecond := strings.Index(css, "SECOND")
	if iFirst < 0 || iSecond < 0 {
		t.Fatalf("both stylesheets must be present:\n%s", css)
	}
	if iFirst > iSecond {
		t.Error("stylesheets are out of argument order, so the wrong one would win")
	}
}

// TestBuildCSS_MissingFileIsAnError checks a typo in -css surfaces as an error
// naming the file rather than silently producing an unstyled document.
func TestBuildCSS_MissingFileIsAnError(t *testing.T) {
	missing := t.TempDir() + "/nope.css"
	c := &Converter{cfg: &Config{CSSFiles: []string{missing}}}

	_, err := c.buildCSS()
	if err == nil {
		t.Fatal("expected an error for a missing stylesheet")
	}
	if !strings.Contains(err.Error(), "nope.css") {
		t.Errorf("error %q does not name the missing file", err)
	}
}

// TestBuildCSS_EmptyFileIsAccepted checks an empty stylesheet contributes
// nothing and is not treated as a failure.
func TestBuildCSS_EmptyFileIsAccepted(t *testing.T) {
	dir := t.TempDir()
	empty := dir + "/empty.css"
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := &Converter{cfg: &Config{}}
	base, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS: %v", err)
	}

	c.cfg.CSSFiles = []string{empty}
	got, err := c.buildCSS()
	if err != nil {
		t.Fatalf("buildCSS with an empty stylesheet: %v", err)
	}
	if got != base {
		t.Errorf("an empty stylesheet changed the output:\n got: %q\nwant: %q", got, base)
	}
}

// TestBuildCSS_RejectsStyleTagBreakout guards the inline <style> block. The page
// is assembled with text/template, which does not escape, so a stylesheet
// containing "</style" would otherwise close the block and inject markup.
func TestBuildCSS_RejectsStyleTagBreakout(t *testing.T) {
	for _, content := range []string{
		"</style><script>alert(1)</script>",
		"body{}\n</STYLE >\n",
		"a{}</style\t>",
	} {
		dir := t.TempDir()
		path := dir + "/evil.css"
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}

		c := &Converter{cfg: &Config{CSSFiles: []string{path}}}
		_, err := c.buildCSS()
		if err == nil {
			t.Errorf("stylesheet %q was accepted but can escape the <style> block", content)
			continue
		}
		if !strings.Contains(err.Error(), "</style") {
			t.Errorf("error %q does not explain what is wrong", err)
		}
	}
}
