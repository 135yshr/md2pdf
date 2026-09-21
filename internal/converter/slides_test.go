package converter

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseSlides_SplitsAtTopLevelThematicBreaks(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // a fragment each slide's HTML must contain
	}{
		{"dashes", "# One\n\n---\n\n# Two\n\n---\n\n# Three\n", []string{"One", "Two", "Three"}},
		{"asterisks", "# One\n\n***\n\n# Two\n", []string{"One", "Two"}},
		{"underscores", "# One\n\n___\n\n# Two\n", []string{"One", "Two"}},
		{"spaced dashes", "# One\n\n- - -\n\n# Two\n", []string{"One", "Two"}},
		{"no separator is one slide", "# Only\n\ntext\n", []string{"Only"}},
		{"consecutive separators make an empty slide", "# One\n\n---\n\n---\n\n# Three\n", []string{"One", "", "Three"}},
		{"a trailing separator makes an empty last slide", "# One\n\n---\n", []string{"One", ""}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseSlides([]byte(tt.src))
			if err != nil {
				t.Fatalf("parseSlides: %v", err)
			}
			if len(doc.slides) != len(tt.want) {
				t.Fatalf("got %d slides, want %d: %+v", len(doc.slides), len(tt.want), doc.slides)
			}
			for i, frag := range tt.want {
				got := doc.slides[i].html
				if frag == "" {
					if strings.TrimSpace(got) != "" {
						t.Errorf("slide %d = %q, want empty", i+1, got)
					}
					continue
				}
				if !strings.Contains(got, frag) {
					t.Errorf("slide %d = %q, want it to contain %q", i+1, got, frag)
				}
				if strings.Contains(got, "<hr") {
					t.Errorf("slide %d still holds the separator: %q", i+1, got)
				}
			}
		})
	}
}

func TestParseSlides_NestedBreaksDoNotSplit(t *testing.T) {
	tests := []struct {
		name string
		src  string
	}{
		{"fenced code", "# One\n\n```\n---\n```\n\ntext\n"},
		{"list item", "- a\n\n  ---\n\n- b\n"},
		{"blockquote", "> a\n>\n> ---\n>\n> b\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseSlides([]byte(tt.src))
			if err != nil {
				t.Fatalf("parseSlides: %v", err)
			}
			if len(doc.slides) != 1 {
				t.Errorf("got %d slides, want 1: %+v", len(doc.slides), doc.slides)
			}
		})
	}
}

func TestParseSlides_SetextHeadingIsNotASeparator(t *testing.T) {
	doc, err := parseSlides([]byte("Title\n---\n\nbody\n"))
	if err != nil {
		t.Fatalf("parseSlides: %v", err)
	}
	if len(doc.slides) != 1 || !strings.Contains(doc.slides[0].html, "<h2") {
		t.Errorf("a setext underline split the slide: %+v", doc.slides)
	}
}

func TestParseSlides_MermaidPlaceholdersSurviveTheSplit(t *testing.T) {
	src := "# One\n\n```mermaid\ngraph TD\n  A --> B\n```\n\n---\n\n```mermaid\ngraph TD\n  C --> D\n```\n"
	doc, err := parseSlides([]byte(src))
	if err != nil {
		t.Fatalf("parseSlides: %v", err)
	}
	if len(doc.mermaidBlocks) != 2 {
		t.Fatalf("got %d mermaid blocks, want 2", len(doc.mermaidBlocks))
	}
	for i, b := range doc.mermaidBlocks {
		comment := "<!--" + b.Placeholder + "-->"
		if !strings.Contains(doc.slides[i].html, comment) {
			t.Errorf("slide %d lacks placeholder %s: %q", i+1, comment, doc.slides[i].html)
		}
	}
}

func TestParseMarkdown_DocumentModeHasNoSlides(t *testing.T) {
	doc, err := parseMarkdown([]byte("# One\n\n---\n\n# Two\n"))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if doc.slides != nil {
		t.Errorf("document mode produced slides: %+v", doc.slides)
	}
	if !strings.Contains(doc.HTML, "<hr") {
		t.Errorf("document mode lost the thematic break: %q", doc.HTML)
	}
}

const threeSlides = "# One\n\n---\n\n# Two\n\n---\n\n# Three\n"

var sectionRE = regexp.MustCompile(`<section[ >]`)

// convertHTML runs the html pipeline on content with cfg and returns the page.
func convertHTML(t *testing.T, cfg *Config, content string) string {
	t.Helper()
	cfg.Format = FormatHTML
	input := writeDoc(t, content)
	out := filepath.Join(t.TempDir(), "out.html")
	c := newTestConverter(t, cfg)
	if err := c.Convert(t.Context(), []string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return readString(t, out)
}

func TestHTMLOutput_SlideModeFromFrontMatter(t *testing.T) {
	page := convertHTML(t, &Config{}, "---\nmarp: true\n---\n\n"+threeSlides)
	if got := len(sectionRE.FindAllString(page, -1)); got != 3 {
		t.Errorf("got %d <section> elements, want 3", got)
	}
	if strings.Contains(page, "<hr") {
		t.Error("slide mode still renders the separators as <hr>")
	}
}

func TestHTMLOutput_SlideModeFromConfig(t *testing.T) {
	page := convertHTML(t, &Config{Slides: true}, threeSlides)
	if got := len(sectionRE.FindAllString(page, -1)); got != 3 {
		t.Errorf("got %d <section> elements, want 3", got)
	}
}

func TestHTMLOutput_DocumentModeIsUnchanged(t *testing.T) {
	for name, content := range map[string]string{
		"no front-matter": threeSlides,
		"marp: false":     "---\nmarp: false\n---\n\n" + threeSlides,
		"marp as text":    "---\nmarp: \"yes\"\n---\n\n" + threeSlides,
	} {
		t.Run(name, func(t *testing.T) {
			page := convertHTML(t, &Config{}, content)
			if sectionRE.MatchString(page) {
				t.Error("document mode emitted <section> elements")
			}
			if strings.Count(page, "<hr") != 2 {
				t.Errorf("document mode should keep both <hr>, got %d", strings.Count(page, "<hr"))
			}
		})
	}
}

func TestPDFOutput_OnePagePerSlide(t *testing.T) {
	if _, err := chromiumPath(); err != nil {
		t.Skipf("no Chromium available: %v", err)
	}
	tests := []struct {
		name    string
		content string
		want    int
	}{
		{"three slides", "---\nmarp: true\n---\n\n" + threeSlides, 3},
		{"an empty slide still gets a page", "---\nmarp: true\n---\n\n# One\n\n---\n\n---\n\n# Three\n", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := writeDoc(t, tt.content)
			out := filepath.Join(t.TempDir(), "out.pdf")
			c := newTestConverter(t, &Config{Format: FormatPDF, PageSize: "A4"})
			if err := c.Convert(t.Context(), []string{input}, out); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("read pdf: %v", err)
			}
			if got := pdfPageCount(data); got != tt.want {
				t.Errorf("PDF has %d pages, want %d", got, tt.want)
			}
		})
	}
}

var pageObjectRE = regexp.MustCompile(`/Type /Page[^s]`)

// pdfPageCount counts the pages of a PDF Chromium printed. Skia writes page
// objects uncompressed, so counting them needs no PDF parser.
func pdfPageCount(data []byte) int {
	return len(pageObjectRE.FindAll(data, -1))
}
