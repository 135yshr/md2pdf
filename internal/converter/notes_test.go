package converter

import (
	"strings"
	"testing"
)

func TestParseSlides_CollectsPresenterNotes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string // notes per slide
	}{
		{
			"a note on slide 2 only",
			"# One\n\n---\n\n# Two\n\n<!-- Mention the benchmark -->\n\n---\n\n# Three\n",
			[]string{"", "Mention the benchmark", ""},
		},
		{
			"two comments join with a blank line, in source order",
			"<!-- first -->\n\n# One\n\n<!-- second -->\n",
			[]string{"first\n\nsecond"},
		},
		{
			"a multi-line comment keeps its line breaks",
			"# One\n\n<!--\nline one\nline two\n-->\n",
			[]string{"line one\nline two"},
		},
		{
			"a directive is not a note",
			"<!-- _class: lead -->\n\n# One\n",
			[]string{""},
		},
		{
			"an empty comment adds nothing",
			"# One\n\n<!-- -->\n\n<!--\n\n-->\n",
			[]string{""},
		},
		{
			"a comment in a code block is code",
			"# One\n\n```html\n<!-- not a note -->\n```\n",
			[]string{""},
		},
		{
			"a note with a colon that is not a directive",
			"# One\n\n<!-- TODO: shorten this -->\n",
			[]string{"TODO: shorten this"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc, err := parseSlides([]byte(tt.src), nil)
			if err != nil {
				t.Fatalf("parseSlides: %v", err)
			}
			if len(doc.slides) != len(tt.want) {
				t.Fatalf("got %d slides, want %d", len(doc.slides), len(tt.want))
			}
			for i, want := range tt.want {
				if got := doc.slides[i].notes; got != want {
					t.Errorf("slide %d notes = %q, want %q", i+1, got, want)
				}
			}
		})
	}
}

func TestParseSlides_NotesAreRemovedFromTheSlide(t *testing.T) {
	doc, err := parseSlides([]byte("# One\n\n<!-- secret speaker note -->\n"), nil)
	if err != nil {
		t.Fatalf("parseSlides: %v", err)
	}
	if strings.Contains(doc.slides[0].html, "secret speaker note") {
		t.Errorf("the note is still in the slide HTML: %q", doc.slides[0].html)
	}
}

func TestParseMarkdown_DocumentModeKeepsComments(t *testing.T) {
	doc, err := parseMarkdown([]byte("# One\n\n<!-- kept as before -->\n"))
	if err != nil {
		t.Fatalf("parseMarkdown: %v", err)
	}
	if !strings.Contains(doc.HTML, "<!-- kept as before -->") {
		t.Errorf("document mode changed how comments render: %q", doc.HTML)
	}
}
