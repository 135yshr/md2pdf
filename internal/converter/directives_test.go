package converter

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestParseDirectiveComment(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		want   map[string]any
		wantOK bool
	}{
		{"spot class", "<!-- _class: lead -->", map[string]any{"_class": "lead"}, true},
		{"local class", "<!-- class: lead -->\n", map[string]any{"class": "lead"}, true},
		{"multi-line", "<!--\nclass: lead\npaginate: true\n-->", map[string]any{"class": "lead", "paginate": true}, true},
		{"an unimplemented Marp directive is still one", "<!-- paginate: true -->", map[string]any{"paginate": true}, true},
		{"prose is a note", "<!-- Mention the benchmark -->", nil, false},
		{"an unknown key is a note", "<!-- hello: world -->", nil, false},
		{"a known and an unknown key is a note", "<!--\nclass: lead\nhello: world\n-->", nil, false},
		{"empty is not a directive", "<!-- -->", nil, false},
		{"text after the comment", "<!-- class: lead --> trailing", nil, false},
		{"two comments", "<!-- class: lead --><!-- x -->", nil, false},
		{"not a comment", "<div>class: lead</div>", nil, false},
		{"invalid YAML is a note", "<!-- class: [lead -->", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseDirectiveComment(tt.raw)
			if ok != tt.wantOK {
				t.Fatalf("parseDirectiveComment(%q) ok = %v, want %v", tt.raw, ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if len(got) != len(tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("got[%q] = %v, want %v", k, got[k], v)
				}
			}
		})
	}
}

func TestClassValue(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"single", "lead", "lead"},
		{"space separated", "lead  invert", "lead invert"},
		{"YAML sequence", []any{"lead", "invert"}, "lead invert"},
		{"unusable", 3, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := strings.Join(classValue(tt.in), " "); got != tt.want {
				t.Errorf("classValue(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

var sectionTagRE = regexp.MustCompile(`<section class="([^"]*)"`)

// slideClasses returns the class attribute of every slide in page, in order,
// without the "slide" class every one of them has.
func slideClasses(t *testing.T, page string) []string {
	t.Helper()
	var out []string
	for _, m := range sectionTagRE.FindAllStringSubmatch(page, -1) {
		out = append(out, strings.TrimSpace(strings.TrimPrefix(m[1], "slide")))
	}
	return out
}

func TestHTMLOutput_ClassDirective(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{
			"spot class on slide 1 only",
			"---\nmarp: true\n---\n\n<!-- _class: lead -->\n\n# One\n\n---\n\n# Two\n\n---\n\n# Three\n",
			[]string{"lead", "", ""},
		},
		{
			"local class from slide 2 onward",
			"---\nmarp: true\n---\n\n# One\n\n---\n\n<!-- class: lead -->\n\n# Two\n\n---\n\n# Three\n",
			[]string{"", "lead", "lead"},
		},
		{
			"front-matter class on every slide",
			"---\nmarp: true\nclass: lead\n---\n\n" + threeSlides,
			[]string{"lead", "lead", "lead"},
		},
		{
			"spot overrides the inherited class for its slide only",
			"---\nmarp: true\nclass: lead\n---\n\n# One\n\n---\n\n<!-- _class: invert -->\n\n# Two\n\n---\n\n# Three\n",
			[]string{"lead", "invert", "lead"},
		},
		{
			"several classes",
			"---\nmarp: true\n---\n\n<!-- _class: lead invert -->\n\n# One\n",
			[]string{"lead invert"},
		},
		{
			"a directive after the content still applies to its slide",
			"---\nmarp: true\n---\n\n# One\n\n<!-- _class: lead -->\n\n---\n\n# Two\n",
			[]string{"lead", ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			page := convertHTML(t, &Config{}, tt.content)
			got := slideClasses(t, page)
			if strings.Join(got, "|") != strings.Join(tt.want, "|") {
				t.Errorf("slide classes = %q, want %q", got, tt.want)
			}
			if strings.Contains(page, "_class") || strings.Contains(page, "class: ") {
				t.Error("a directive comment was left in the output")
			}
		})
	}
}

func TestHTMLOutput_ClassValuesAreEscaped(t *testing.T) {
	page := convertHTML(t, &Config{}, "---\nmarp: true\n---\n\n<!-- _class: '\"><script>x</script>' -->\n\n# One\n")
	if strings.Contains(page, "<script>") {
		t.Error("a class value broke out of the attribute")
	}
}

func TestHTMLOutput_NoteCommentsAreNotDirectives(t *testing.T) {
	page := convertHTML(t, &Config{}, "---\nmarp: true\n---\n\n<!-- hello: world -->\n\n# One\n")
	if got := slideClasses(t, page); len(got) != 1 || got[0] != "" {
		t.Errorf("slide classes = %q, want one slide with no class", got)
	}
}

func TestHTMLOutput_DirectivesInCodeAreCode(t *testing.T) {
	page := convertHTML(t, &Config{}, "---\nmarp: true\n---\n\n```html\n<!-- _class: lead -->\n```\n")
	if got := slideClasses(t, page); len(got) != 1 || got[0] != "" {
		t.Errorf("a directive inside a code block took effect: %q", got)
	}
	if !strings.Contains(page, "_class: lead") {
		t.Error("the code block lost its content")
	}
}

func TestHTMLOutput_SlideModeUsesTheSlideStylesheet(t *testing.T) {
	slides := convertHTML(t, &Config{}, "---\nmarp: true\n---\n\n# One\n")
	if strings.Contains(slides, "max-width: 900px") {
		t.Error("slide mode still carries the document stylesheet's 900px column")
	}
	if !strings.Contains(slides, "section.slide.lead") {
		t.Error("slide mode lacks the slide stylesheet's lead layout")
	}

	doc := convertHTML(t, &Config{}, "# One\n")
	if !strings.Contains(doc, "max-width: 900px") || strings.Contains(doc, "section.slide") {
		t.Error("document mode's stylesheet changed")
	}
}

func TestHTMLOutput_CustomCSSComesAfterTheSlideStylesheet(t *testing.T) {
	css := filepath.Join(t.TempDir(), "my.css")
	if err := os.WriteFile(css, []byte("/* user-rules */\n"), 0o600); err != nil {
		t.Fatalf("write css: %v", err)
	}
	page := convertHTML(t, &Config{CSSFiles: []string{css}}, "---\nmarp: true\n---\n\n# One\n")
	user := strings.Index(page, "/* user-rules */")
	slide := strings.Index(page, "section.slide.lead")
	if user < 0 || slide < 0 || user < slide {
		t.Errorf("user CSS at %d, slide stylesheet at %d; want the user rules last", user, slide)
	}
}
