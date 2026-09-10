package converter

import (
	"errors"
	"fmt"
	"strings"

	"charm.land/glamour/v2/styles"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/diagram"
	"github.com/AlexanderGrooff/mermaid-ascii/pkg/render"
	"github.com/charmbracelet/x/ansi"
)

// asciiDiagramPrefixes lists the Mermaid diagram types rendered as text art.
//
// The set is deliberately narrower than what mermaid-ascii accepts. Its
// erDiagram support parses but lays the entities out misaligned, and its graph
// parser is the fallback for any unrecognized header, so relying on the library
// to reject a type is not enough. Everything outside this list keeps its source.
var asciiDiagramPrefixes = []string{"flowchart", "graph", "sequenceDiagram"}

// errUnsupportedASCIIDiagram reports a diagram type that has no legible text-art
// rendering, so the caller falls back to showing the Mermaid source.
var errUnsupportedASCIIDiagram = errors.New("diagram type has no text-art rendering")

// consoleWantsPureASCII reports whether the console style asks for plain ASCII
// box characters instead of Unicode box-drawing ones. Glamour's "ascii" style is
// the marker for a terminal that cannot show the latter.
//
// It follows the same precedence as resolveConsoleStyle — the -style flag over
// GLAMOUR_STYLE — rather than looking only at the resolved style, because that
// resolves to "notty" for redirected output and would otherwise discard an
// explicit request for ASCII.
func consoleWantsPureASCII(explicit, envStyle, resolved string) bool {
	switch {
	case explicit != "":
		return explicit == styles.AsciiStyle
	case envStyle != "":
		return envStyle == styles.AsciiStyle
	default:
		return resolved == styles.AsciiStyle
	}
}

// asciiDiagramSupported reports whether the Mermaid source names a diagram type
// that renders legibly as text art.
func asciiDiagramSupported(source string) bool {
	header, ok := mermaidDiagramHeader(source)
	if !ok {
		return false
	}
	for _, prefix := range asciiDiagramPrefixes {
		if header == prefix || strings.HasPrefix(header, prefix+" ") {
			return true
		}
	}
	return false
}

// mermaidDiagramHeader returns the first meaningful line of a Mermaid diagram,
// skipping blank lines and %% comments. It reports ok=false for a source with no
// such line.
//
// YAML frontmatter is stripped with the renderer's own helper first. Without
// that, a diagram carrying a "---" title block would look like an unsupported
// type here and be rejected before render.RenderDiagram — which strips it — ever
// saw it.
func mermaidDiagramHeader(source string) (string, bool) {
	source, _ = diagram.StripFrontmatter(source)
	for _, line := range strings.Split(source, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
			continue
		}
		return trimmed, true
	}
	return "", false
}

// renderMermaidASCII converts Mermaid source to box-drawing text art at most
// width columns wide. When pureASCII is set, the Unicode box-drawing characters
// are swapped for plain +, - and | so the art stays readable on terminals that
// cannot show them.
//
// Diagram types outside asciiDiagramPrefixes are rejected before parsing. Art
// wider than width has its node labels wrapped until it fits; see fitMermaidArt
// for why that is the only lever available.
func renderMermaidASCII(source string, width int, pureASCII bool) (string, error) {
	if !asciiDiagramSupported(source) {
		header, _ := mermaidDiagramHeader(source)
		return "", fmt.Errorf("%w: %q", errUnsupportedASCIIDiagram, header)
	}

	art, err := renderMermaidArt(source, width, pureASCII)
	if err != nil {
		return "", err
	}
	return fitMermaidArt(source, art, width, pureASCII), nil
}

// renderMermaidArt draws Mermaid source as text art exactly as written, with no
// attempt to make it fit. It is the single place the library is called, so the
// fit loop can re-draw a rewritten source through the same path.
//
// The layout is guarded against panics here, because mermaid-ascii is a fallback
// path: a crash there must degrade to the Mermaid source rather than take down
// the run.
func renderMermaidArt(source string, width int, pureASCII bool) (art string, err error) {
	cfg := diagram.DefaultConfig()
	cfg.UseAscii = pureASCII
	cfg.StyleType = "cli"
	if width > 0 {
		cfg.MaxWidth = width
	}
	// Config.GraphDirection is deliberately left at its default: mermaid-ascii's
	// parser takes the direction from the diagram header, so setting it here has
	// no effect. A header without one (bare "graph") lays out top-down, matching
	// Mermaid's own default.
	defer func() {
		if r := recover(); r != nil {
			art = ""
			err = fmt.Errorf("text-art layout panicked: %v", r)
		}
	}()

	art, err = render.RenderDiagram(source, cfg)
	if err != nil {
		return "", fmt.Errorf("render mermaid as text art: %w", err)
	}
	return art, nil
}

// clipConsoleArt trims each line of text art to width columns. The art is
// spliced into the document after glamour has wrapped it, so an over-wide line
// would otherwise be folded by the terminal into unreadable fragments. Clipping
// keeps the diagram's shape intact at the cost of its right edge.
//
// It is applied at splice time rather than at render time because only the
// splice knows how far glamour indented the diagram.
func clipConsoleArt(art string, width int) string {
	if width <= 0 {
		return art
	}
	lines := strings.Split(art, "\n")
	for i, line := range lines {
		if ansi.StringWidth(line) > width {
			lines[i] = ansi.Truncate(line, width, "")
		}
	}
	return strings.Join(lines, "\n")
}
