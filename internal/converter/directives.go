package converter

import (
	"strings"

	"github.com/yuin/goldmark/ast"
	"go.yaml.in/yaml/v3"
)

// directiveNames are the directive keys Marp recognizes: Marpit's global and
// local directives, the ones Marp Core adds, and Marp CLI's metadata. Only
// some of them are implemented here, but all are recognized, so a Marp deck's
// <!-- paginate: true --> is dropped as a directive instead of surfacing as a
// presenter note.
var directiveNames = map[string]bool{
	// Marpit global directives.
	"headingDivider": true, "lang": true, "style": true, "theme": true,
	// Marpit local directives.
	"paginate": true, "header": true, "footer": true, "class": true,
	"backgroundColor": true, "backgroundImage": true, "backgroundPosition": true,
	"backgroundRepeat": true, "backgroundSize": true, "color": true,
	// Marp Core.
	"size": true, "math": true, "marp": true,
	// Marp CLI metadata and the bespoke template.
	"title": true, "description": true, "author": true, "keywords": true,
	"url": true, "image": true, "transition": true,
}

// parseDirectiveComment reports whether raw, the text of an HTML block, is a
// single HTML comment holding directives, and returns them.
//
// It is a directive only when the comment's body parses as a YAML mapping and
// every key is a known directive name, optionally prefixed with "_" to apply it
// to one slide. Anything else — prose, an unknown key, a key mixed with prose,
// invalid YAML — is an ordinary comment, which slide mode treats as a
// presenter note. Requiring every key to be known is what stops a note that
// happens to contain a colon, like "<!-- TODO: shorten -->", from being eaten.
func parseDirectiveComment(raw string) (map[string]any, bool) {
	body, ok := commentBody(raw)
	if !ok {
		return nil, false
	}
	var parsed map[string]any
	if err := yaml.Unmarshal([]byte(body), &parsed); err != nil || len(parsed) == 0 {
		return nil, false
	}
	for key := range parsed {
		if !directiveNames[strings.TrimPrefix(key, "_")] {
			return nil, false
		}
	}
	return parsed, true
}

// commentBody returns what is between "<!--" and "-->" when raw, trimmed, is
// exactly one HTML comment.
func commentBody(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if !strings.HasPrefix(s, "<!--") || !strings.HasSuffix(s, "-->") || len(s) < len("<!---->") {
		return "", false
	}
	body := s[len("<!--") : len(s)-len("-->")]
	if strings.Contains(body, "-->") {
		return "", false
	}
	return body, true
}

// htmlBlockText returns the source text of an HTML block, closing line
// included.
func htmlBlockText(n *ast.HTMLBlock, src []byte) string {
	var sb strings.Builder
	for i := range n.Lines().Len() {
		line := n.Lines().At(i)
		sb.Write(line.Value(src))
	}
	if n.HasClosure() {
		sb.Write(n.ClosureLine.Value(src))
	}
	return sb.String()
}

// classValue reads a class directive: a space-separated string, or a YAML
// sequence of them. Any other type yields no classes.
func classValue(v any) []string {
	switch val := v.(type) {
	case string:
		return strings.Fields(val)
	case []any:
		var out []string
		for _, item := range val {
			if s, ok := item.(string); ok {
				out = append(out, strings.Fields(s)...)
			}
		}
		return out
	default:
		return nil
	}
}

// slideDirectives tracks the directives in effect while slides are rendered.
// Local directives apply to their slide and every slide after it; the
// "_"-prefixed spot form applies to its slide alone and overrides the local
// value there.
type slideDirectives struct {
	class []string
	spot  map[string]any
}

// newSlideDirectives starts from the front-matter, where a local directive
// such as class applies from the first slide on.
func newSlideDirectives(meta frontMatter) *slideDirectives {
	return &slideDirectives{class: classValue(meta["class"]), spot: map[string]any{}}
}

// apply records the directives of one comment on the current slide.
func (d *slideDirectives) apply(dirs map[string]any) {
	for key, value := range dirs {
		if name, spot := strings.CutPrefix(key, "_"); spot {
			d.spot[name] = value
			continue
		}
		if key == "class" {
			d.class = classValue(value)
		}
	}
}

// finish returns the current slide's classes and resets the spot directives
// for the next slide.
func (d *slideDirectives) finish() []string {
	classes := d.class
	if v, ok := d.spot["class"]; ok {
		classes = classValue(v)
	}
	d.spot = map[string]any{}
	return classes
}
