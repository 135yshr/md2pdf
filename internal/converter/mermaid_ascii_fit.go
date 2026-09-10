package converter

import (
	"regexp"
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// labelBreak is the line break mermaid-ascii understands inside a node label.
// Its own parser accepts <br>, <br/> and <br />; the shortest spelling is
// emitted so wrapping costs the layout as few columns as possible.
const labelBreak = "<br>"

// labelBreakRe matches the <br> spellings mermaid-ascii treats as line breaks.
// It mirrors the library's own pattern (pkg/graph/label.go) so wrapping splits a
// label exactly where the renderer will.
var labelBreakRe = regexp.MustCompile(`(?i)<br\s*/?>`)

// labelShapes lists the node shapes whose label text may be rewritten, in the
// order they must be matched so that a two-character delimiter wins over the
// one-character delimiter it starts with.
//
// The set mirrors mermaid-ascii's parseNode with one deliberate omission: its
// ">...]" asymmetric shape is left out, because a ">" cannot be told apart from
// the one ending an "-->" arrow.
var labelShapes = []struct{ open, close string }{
	{open: "[(", close: ")]"},
	{open: "{{", close: "}}"},
	{open: "{", close: "}"},
	{open: "[", close: "]"},
	{open: "(", close: ")"},
}

// labelToken is one piece of a label that wrapping never splits.
//
// A run of narrow characters is one token, because breaking a word or a path
// mid-way would make it unreadable. A wide character is a token of its own,
// which is what lets a Japanese label containing no spaces wrap at all.
type labelToken struct {
	text string
	// spaced records that a space separated this token from the previous one.
	// The space is restored when both land on the same line and dropped when
	// the token starts a new one.
	spaced bool
}

// labelTokens splits label text into the pieces wrapping may move onto separate
// lines.
func labelTokens(text string) []labelToken {
	var tokens []labelToken
	spaced := false
	narrowRun := false
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			spaced = true
			narrowRun = false
		case ansi.StringWidth(string(r)) > 1:
			tokens = append(tokens, labelToken{text: string(r), spaced: spaced})
			spaced, narrowRun = false, false
		case narrowRun:
			tokens[len(tokens)-1].text += string(r)
		default:
			tokens = append(tokens, labelToken{text: string(r), spaced: spaced})
			spaced, narrowRun = false, true
		}
	}
	return tokens
}

// wrapLabelText breaks a Mermaid node label into lines at most maxWidth columns
// wide, joined with the <br> tags mermaid-ascii treats as line breaks.
//
// Breaks the author wrote are boundaries rather than text: each segment between
// them is wrapped on its own, so a hand-tuned label is never reflowed into one
// line. Text that already fits is returned byte-for-byte unchanged.
func wrapLabelText(text string, maxWidth int) string {
	if maxWidth <= 0 {
		return text
	}

	segments := labelBreakRe.Split(text, -1)
	if len(segments) == 1 && ansi.StringWidth(text) <= maxWidth {
		return text
	}

	wrapped := make([]string, len(segments))
	changed := false
	for i, segment := range segments {
		wrapped[i] = wrapLabelSegment(segment, maxWidth)
		changed = changed || wrapped[i] != segment
	}
	if !changed {
		return text
	}
	return strings.Join(wrapped, labelBreak)
}

// wrapLabelSegment greedily packs one break-free stretch of label text into
// lines of at most maxWidth columns. A single token wider than maxWidth gets a
// line to itself and is left to overflow, since splitting it would corrupt it.
func wrapLabelSegment(segment string, maxWidth int) string {
	if ansi.StringWidth(segment) <= maxWidth {
		return segment
	}

	var lines []string
	current := ""
	for _, token := range labelTokens(segment) {
		separator := ""
		if token.spaced {
			separator = " "
		}
		switch {
		case current == "":
			current = token.text
		case ansi.StringWidth(current+separator+token.text) <= maxWidth:
			current += separator + token.text
		default:
			lines = append(lines, current)
			current = token.text
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, labelBreak)
}

// wrapLabelValue wraps the text between a shape's delimiters. Quotes the author
// put around the label are kept outermost, so the renderer still strips them and
// the columns they occupy are not charged against the label's width.
func wrapLabelValue(value string, maxWidth int) string {
	if len(value) >= 2 && strings.HasPrefix(value, `"`) && strings.HasSuffix(value, `"`) {
		return `"` + wrapLabelText(value[1:len(value)-1], maxWidth) + `"`
	}
	return wrapLabelText(value, maxWidth)
}

// isNodeIDByte reports whether b can end a Mermaid node id, which is how a
// shape delimiter is told apart from the same character used elsewhere on the
// line. A hyphen is deliberately excluded: it is legal inside an id, but
// accepting it would let an arrow such as "--" open a label.
func isNodeIDByte(b byte) bool {
	return b == '_' ||
		('a' <= b && b <= 'z') ||
		('A' <= b && b <= 'Z') ||
		('0' <= b && b <= '9')
}

// precededByNodeID reports whether the shape delimiter at index i follows a node
// id, ignoring any spaces between the two.
func precededByNodeID(line string, i int) bool {
	j := i - 1
	for j >= 0 && line[j] == ' ' {
		j--
	}
	return j >= 0 && isNodeIDByte(line[j])
}

// containsUnquoted reports whether text contains substr outside double quotes.
func containsUnquoted(text, substr string) bool {
	inQuotes := false
	for i := 0; i < len(text); i++ {
		if text[i] == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && strings.HasPrefix(text[i:], substr) {
			return true
		}
	}
	return false
}

// findLabelEnd returns the index of the closing delimiter for a label starting
// at start, skipping delimiters inside double quotes. It reports ok=false when
// the label is unterminated.
func findLabelEnd(line string, start int, closer string) (int, bool) {
	inQuotes := false
	for i := start; i < len(line); i++ {
		if line[i] == '"' {
			inQuotes = !inQuotes
			continue
		}
		if !inQuotes && strings.HasPrefix(line[i:], closer) {
			return i, true
		}
	}
	return 0, false
}

// labelValueAt returns the label text of the node shape opening at index i.
// It reports ok=false when no shape opens there, when the shape is unterminated,
// or when the text between the delimiters holds another one of them: the
// renderer parses such a label differently, and rewriting it would change the
// diagram rather than narrow it.
func labelValueAt(line string, i int) (opener, closer, value string, ok bool) {
	if !precededByNodeID(line, i) {
		return "", "", "", false
	}
	for _, shape := range labelShapes {
		if !strings.HasPrefix(line[i:], shape.open) {
			continue
		}
		end, found := findLabelEnd(line, i+len(shape.open), shape.close)
		if !found {
			return "", "", "", false
		}
		inner := line[i+len(shape.open) : end]
		if containsUnquoted(inner, shape.open) || containsUnquoted(inner, shape.close) {
			return "", "", "", false
		}
		return shape.open, shape.close, inner, true
	}
	return "", "", "", false
}

// nonNodeStatements lists the statement keywords that introduce a line carrying
// no node definition. Their lines are passed through untouched: a comment or a
// styling directive may contain brackets that are not a label at all, and
// subgraph labels are out of scope for this rewrite.
var nonNodeStatements = []string{
	"subgraph", "end", "classDef", "class", "style", "linkStyle",
	"click", "direction", "accTitle", "accDescr",
}

// rewritableLine reports whether a line of Mermaid source may hold a node label
// this rewrite is allowed to wrap.
func rewritableLine(line string) bool {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "%%") {
		return false
	}
	for _, statement := range nonNodeStatements {
		if trimmed == statement || strings.HasPrefix(trimmed, statement+" ") {
			return false
		}
	}
	return true
}

// fitLabelsInLine wraps every node label on one line of Mermaid source.
//
// Edge labels are stepped over rather than searched, in both spellings Mermaid
// allows. A shape delimiter is only a delimiter outside them: in
// A -- "read [the docs]" --> B and A -->|read [the docs]| B alike, the
// characters before the bracket look like a node id, but the bracket is label
// text this rewrite has no business touching. A node label is unaffected either
// way, since its delimiter sits outside the quotes or pipes and the whole span
// is consumed in one step.
func fitLabelsInLine(line string, maxWidth int) string {
	var out strings.Builder
	inQuotes := false
	for i := 0; i < len(line); {
		if line[i] == '"' {
			inQuotes = !inQuotes
			out.WriteByte(line[i])
			i++
			continue
		}
		if inQuotes {
			out.WriteByte(line[i])
			i++
			continue
		}
		// A pipe only opens an edge label when another one closes it. A lone
		// pipe is just a character, and treating it as an opener would swallow
		// every node after it.
		if line[i] == '|' {
			if end, ok := findLabelEnd(line, i+1, "|"); ok {
				out.WriteString(line[i : end+1])
				i = end + 1
				continue
			}
		}
		opener, closer, value, ok := labelValueAt(line, i)
		if !ok {
			out.WriteByte(line[i])
			i++
			continue
		}
		out.WriteString(opener)
		out.WriteString(wrapLabelValue(value, maxWidth))
		out.WriteString(closer)
		i += len(opener) + len(value) + len(closer)
	}
	return out.String()
}

// frontmatterEnd returns the index of the first diagram line after a leading
// YAML frontmatter block, or 0 when the source has none. The block is data for
// the renderer rather than diagram text, so a bracket inside it is not a label.
//
// It reads the source the way diagram.StripFrontmatter does: blank lines may
// precede the opening delimiter, and an unterminated block is not frontmatter at
// all but part of the diagram.
func frontmatterEnd(lines []string) int {
	start := 0
	for start < len(lines) && strings.TrimSpace(lines[start]) == "" {
		start++
	}
	if start >= len(lines) || strings.TrimSpace(lines[start]) != "---" {
		return 0
	}
	for i := start + 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			return i + 1
		}
	}
	return 0
}

// fitMermaidLabels rewrites Mermaid source so that no node label line is wider
// than maxWidth columns, by inserting the <br> breaks mermaid-ascii honours.
// Narrowing the labels narrows the boxes, which is the only way to make an
// over-wide text-art diagram fit: the library can compact its padding but never
// reflows a layout.
func fitMermaidLabels(source string, maxWidth int) string {
	lines := strings.Split(source, "\n")
	for i := frontmatterEnd(lines); i < len(lines); i++ {
		if !rewritableLine(lines[i]) {
			continue
		}
		lines[i] = fitLabelsInLine(lines[i], maxWidth)
	}
	return strings.Join(lines, "\n")
}

// mermaidArtWidth returns the width of the widest line of text art in terminal
// columns.
//
// It deliberately does not use the width mermaid-ascii reports alongside its
// output: that goes through go-runewidth, which counts a box-drawing "─" as two
// columns under an East Asian locale and so overstates the art by the number of
// horizontal borders in it. Measuring here with the same function that clips the
// art keeps the fit decision and the clip in agreement.
func mermaidArtWidth(art string) int {
	widest := 0
	for _, line := range strings.Split(art, "\n") {
		widest = max(widest, ansi.StringWidth(line))
	}
	return widest
}

// labelFitCaps lists the label widths the fit loop tries, widest first, so the
// least-wrapped art that fits is the one chosen. The list stops at 8 columns:
// below that a label is broken into so many lines that the box is taller than it
// is readable, and the diagram has stopped being worth drawing.
var labelFitCaps = []int{40, 32, 24, 20, 16, 12, 8}

// fitLabelDiagramPrefixes lists the diagram types whose labels may be wrapped.
// Sequence diagrams are excluded: they have no bracketed node labels, so a
// rewrite there could only corrupt the source.
var fitLabelDiagramPrefixes = []string{"flowchart", "graph"}

// diagramSupportsLabelFitting reports whether source names a diagram type whose
// node labels the fit loop may rewrite.
func diagramSupportsLabelFitting(source string) bool {
	header, ok := mermaidDiagramHeader(source)
	if !ok {
		return false
	}
	for _, prefix := range fitLabelDiagramPrefixes {
		if header == prefix || strings.HasPrefix(header, prefix+" ") {
			return true
		}
	}
	return false
}

// fitMermaidArt narrows text art that overruns width by wrapping its node
// labels, and returns art unchanged when it already fits.
//
// Wrapping the labels is the only lever available. mermaid-ascii sizes each box
// from its label and lays independent chains out side by side, so a wide
// diagram is wide because its labels are; the library's own MaxWidth only
// compacts padding once and never reflows a layout. Trading width for height
// this way is what keeps every chain on screen instead of clipping the
// rightmost ones away.
//
// Nothing here can fail: a rewrite the renderer rejects, and a diagram too wide
// even at the narrowest cap, both fall back to art that draws. The caller is
// left to decide what to do about art that is still too wide.
func fitMermaidArt(source, art string, width int, pureASCII bool) string {
	if width <= 0 || mermaidArtWidth(art) <= width || !diagramSupportsLabelFitting(source) {
		return art
	}

	narrowest := art
	for _, labelCap := range labelFitCaps {
		fitted := fitMermaidLabels(source, labelCap)
		if fitted == source {
			continue
		}
		candidate, err := renderMermaidArt(fitted, width, pureASCII)
		if err != nil {
			continue
		}
		switch candidateWidth := mermaidArtWidth(candidate); {
		case candidateWidth <= width:
			return candidate
		case candidateWidth < mermaidArtWidth(narrowest):
			narrowest = candidate
		}
	}
	return narrowest
}

// fitDiagramIndent trims the indent glamour placed a diagram at down to the
// columns the diagram can spare.
//
// Text art is a figure, not a paragraph. Art fitted to the full wrap width would
// lose its rightmost columns to the document indent — the very clipping fitting
// set out to avoid — so it is allowed to hang into the left margin instead. Art
// that fits with the indent intact keeps it, so nothing moves unnecessarily.
func fitDiagramIndent(indent string, artWidth, width int) string {
	spare := width - artWidth
	if spare >= len(indent) {
		return indent
	}
	return indent[:max(0, spare)]
}
