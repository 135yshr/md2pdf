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

// fitLabelsInLine wraps every node label on one line of Mermaid source.
func fitLabelsInLine(line string, maxWidth int) string {
	var out strings.Builder
	for i := 0; i < len(line); {
		const opener, closer = "[", "]"
		end := 0
		ok := false
		if strings.HasPrefix(line[i:], opener) && precededByNodeID(line, i) {
			end, ok = findLabelEnd(line, i+len(opener), closer)
		}
		if !ok {
			out.WriteByte(line[i])
			i++
			continue
		}
		out.WriteString(opener)
		out.WriteString(wrapLabelText(line[i+len(opener):end], maxWidth))
		out.WriteString(closer)
		i = end + len(closer)
	}
	return out.String()
}

// fitMermaidLabels rewrites Mermaid source so that no node label line is wider
// than maxWidth columns, by inserting the <br> breaks mermaid-ascii honours.
// Narrowing the labels narrows the boxes, which is the only way to make an
// over-wide text-art diagram fit: the library can compact its padding but never
// reflows.
func fitMermaidLabels(source string, maxWidth int) string {
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		lines[i] = fitLabelsInLine(line, maxWidth)
	}
	return strings.Join(lines, "\n")
}
