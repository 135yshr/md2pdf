package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitFrontMatter(t *testing.T) {
	tests := []struct {
		name     string
		src      string
		wantMeta map[string]any
		wantBody string
	}{
		{
			name:     "mapping is stripped and parsed",
			src:      "---\ntitle: Report\nmarp: true\n---\n\n# A\n",
			wantMeta: map[string]any{"title": "Report", "marp": true},
			wantBody: "\n# A\n",
		},
		{
			name:     "dots close the block",
			src:      "---\ntitle: Report\n...\n# A\n",
			wantMeta: map[string]any{"title": "Report"},
			wantBody: "# A\n",
		},
		{
			name:     "CRLF line endings",
			src:      "---\r\ntitle: Report\r\n---\r\n# A\r\n",
			wantMeta: map[string]any{"title": "Report"},
			wantBody: "# A\r\n",
		},
		{
			name:     "trailing spaces on the delimiters",
			src:      "---  \ntitle: Report\n---\t\n# A\n",
			wantMeta: map[string]any{"title": "Report"},
			wantBody: "# A\n",
		},
		{
			name:     "empty block is stripped",
			src:      "---\n---\n# A\n",
			wantMeta: map[string]any{},
			wantBody: "# A\n",
		},
		{
			name:     "block closed at end of file",
			src:      "---\ntitle: Report\n---",
			wantMeta: map[string]any{"title": "Report"},
			wantBody: "",
		},
		{
			name:     "no front-matter",
			src:      "# A\n\n---\n\nB\n",
			wantMeta: nil,
			wantBody: "# A\n\n---\n\nB\n",
		},
		{
			name:     "unterminated block is ordinary Markdown",
			src:      "---\ntitle: Report\n\n# A\n",
			wantMeta: nil,
			wantBody: "---\ntitle: Report\n\n# A\n",
		},
		{
			name:     "block not on the first line is a thematic break",
			src:      "\n---\ntitle: Report\n---\n# A\n",
			wantMeta: nil,
			wantBody: "\n---\ntitle: Report\n---\n# A\n",
		},
		{
			name:     "four dashes is not a delimiter",
			src:      "----\ntitle: Report\n----\n# A\n",
			wantMeta: nil,
			wantBody: "----\ntitle: Report\n----\n# A\n",
		},
		{
			name:     "a scalar is not metadata",
			src:      "---\nSome text\n---\n# A\n",
			wantMeta: nil,
			wantBody: "---\nSome text\n---\n# A\n",
		},
		{
			name:     "a sequence is not metadata",
			src:      "---\n- a\n- b\n---\n# A\n",
			wantMeta: nil,
			wantBody: "---\n- a\n- b\n---\n# A\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			meta, body, err := splitFrontMatter([]byte(tt.src))
			if err != nil {
				t.Fatalf("splitFrontMatter: %v", err)
			}
			if string(body) != tt.wantBody {
				t.Errorf("body = %q, want %q", body, tt.wantBody)
			}
			if tt.wantMeta == nil {
				if meta != nil {
					t.Errorf("meta = %v, want none", meta)
				}
				return
			}
			if meta == nil {
				t.Fatalf("meta = nil, want %v", tt.wantMeta)
			}
			if len(meta) != len(tt.wantMeta) {
				t.Errorf("meta = %v, want %v", meta, tt.wantMeta)
			}
			for k, v := range tt.wantMeta {
				if meta[k] != v {
					t.Errorf("meta[%q] = %v, want %v", k, meta[k], v)
				}
			}
		})
	}
}

func TestSplitFrontMatter_InvalidYAMLIsAnError(t *testing.T) {
	_, _, err := splitFrontMatter([]byte("---\ntitle: [unclosed\n---\n# A\n"))
	if err == nil {
		t.Fatal("splitFrontMatter accepted invalid YAML")
	}
	if !strings.Contains(err.Error(), "front-matter") {
		t.Errorf("error %q does not say it is about the front-matter", err)
	}
}

func TestFrontMatterString(t *testing.T) {
	meta := frontMatter{"title": "Report", "count": 3, "empty": ""}
	if got := meta.String("title"); got != "Report" {
		t.Errorf(`String("title") = %q, want "Report"`, got)
	}
	if got := meta.String("count"); got != "" {
		t.Errorf(`String("count") = %q, want "" for a non-string`, got)
	}
	if got := meta.String("missing"); got != "" {
		t.Errorf(`String("missing") = %q, want ""`, got)
	}
	var none frontMatter
	if got := none.String("title"); got != "" {
		t.Errorf(`nil String("title") = %q, want ""`, got)
	}
}

// writeDoc writes content to a Markdown file in a fresh directory and returns
// its path.
func writeDoc(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.md")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}
	return path
}

const frontMatterDoc = "---\ntitle: Report\nauthor: someone-unique\n---\n\n# A\n\nBody.\n"

func TestHTMLOutput_FrontMatterIsNotRendered(t *testing.T) {
	input := writeDoc(t, frontMatterDoc)
	out := filepath.Join(t.TempDir(), "doc.html")
	c := newTestConverter(t, &Config{Format: FormatHTML})
	if err := c.Convert(t.Context(), []string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	page := readString(t, out)
	body := page[strings.Index(page, "<body>"):]
	for _, unwanted := range []string{"<hr", "someone-unique", "title: Report"} {
		if strings.Contains(body, unwanted) {
			t.Errorf("body contains %q:\n%s", unwanted, body)
		}
	}
	if !strings.Contains(page, "<title>Report</title>") {
		t.Errorf("front-matter title was not used as <title>:\n%s", page[:strings.Index(page, "<body>")])
	}
}

func TestHTMLOutput_TitleFallsBackToFirstHeading(t *testing.T) {
	input := writeDoc(t, "---\nauthor: x\n---\n\n# Heading Title\n")
	out := filepath.Join(t.TempDir(), "doc.html")
	c := newTestConverter(t, &Config{Format: FormatHTML})
	if err := c.Convert(t.Context(), []string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if page := readString(t, out); !strings.Contains(page, "<title>Heading Title</title>") {
		t.Errorf("want the first heading as <title>, got:\n%s", page[:strings.Index(page, "<body>")])
	}
}

func TestHTMLOutput_TitleIsEscaped(t *testing.T) {
	input := writeDoc(t, "---\ntitle: \"</title><script>x</script>\"\n---\n\n# A\n")
	out := filepath.Join(t.TempDir(), "doc.html")
	c := newTestConverter(t, &Config{Format: FormatHTML})
	if err := c.Convert(t.Context(), []string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	if page := readString(t, out); strings.Contains(page, "<script>") {
		t.Errorf("front-matter title was written unescaped:\n%s", page[:strings.Index(page, "<body>")])
	}
}

func TestConvert_InvalidFrontMatterNamesTheFile(t *testing.T) {
	input := writeDoc(t, "---\ntitle: [unclosed\n---\n# A\n")
	c := newTestConverter(t, &Config{Format: FormatHTML})
	err := c.Convert(t.Context(), []string{input}, filepath.Join(t.TempDir(), "doc.html"))
	if err == nil {
		t.Fatal("Convert accepted invalid front-matter YAML")
	}
	if !strings.Contains(err.Error(), input) {
		t.Errorf("error %q does not name the input file", err)
	}
}

func TestConsoleOutput_FrontMatterIsNotRendered(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatConsole, MermaidRender: MermaidRenderSource})
	out := filepath.Join(t.TempDir(), "out.txt")
	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("create output: %v", err)
	}
	defer f.Close()

	c.stdin = strings.NewReader(frontMatterDoc)
	if err := c.renderConsole(t.Context(), []string{StdinPath}, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}
	got := readString(t, out)
	if strings.Contains(got, "someone-unique") {
		t.Errorf("console output contains the front-matter:\n%s", got)
	}
	if !strings.Contains(got, "Body.") {
		t.Errorf("console output lost the body:\n%s", got)
	}
}

func TestDOCXOutput_FrontMatterIsNotPassedToPandoc(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	// The stub copies the Markdown it is given to the -o path, so the "docx"
	// holds exactly what pandoc would have read.
	pandoc := filepath.Join(dir, "pandoc")
	stub := "#!/bin/sh\nin=\"$1\"; out=\"\"\n" +
		"while [ $# -gt 0 ]; do if [ \"$1\" = \"-o\" ]; then out=\"$2\"; fi; shift; done\n" +
		"[ -n \"$out\" ] && [ -f \"$in\" ] && cp \"$in\" \"$out\"\nexit 0\n"
	if err := os.WriteFile(pandoc, []byte(stub), 0o755); err != nil { //nolint:gosec // an executable stub is the point
		t.Fatalf("write pandoc stub: %v", err)
	}

	input := writeDoc(t, frontMatterDoc)
	out := filepath.Join(dir, "doc.docx")
	c := newTestConverter(t, &Config{Format: FormatDOCX, PandocPath: pandoc})
	if err := c.Convert(t.Context(), []string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	got := readString(t, out)
	if strings.Contains(got, "someone-unique") {
		t.Errorf("pandoc was given the front-matter:\n%s", got)
	}
	if !strings.Contains(got, "Body.") {
		t.Errorf("pandoc was not given the body:\n%s", got)
	}
}

func TestParseMarkdown_MermaidFrontmatterIsUntouched(t *testing.T) {
	src := "# A\n\n```mermaid\n---\ntitle: Flow\n---\ngraph TD\n  A --> B\n```\n"
	meta, body, err := splitFrontMatter([]byte(src))
	if err != nil {
		t.Fatalf("splitFrontMatter: %v", err)
	}
	if meta != nil || string(body) != src {
		t.Fatalf("document front-matter handling touched a diagram's own frontmatter")
	}
	doc, err := parseMarkdown(body)
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if len(doc.mermaidBlocks) != 1 || !strings.Contains(doc.mermaidBlocks[0].Source, "title: Flow") {
		t.Errorf("diagram source lost its frontmatter: %+v", doc.mermaidBlocks)
	}
}

func readString(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}
