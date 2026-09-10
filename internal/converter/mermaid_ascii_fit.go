package converter

import (
	"strings"
	"unicode"

	"github.com/charmbracelet/x/ansi"
)

// labelBreak is the line break mermaid-ascii understands inside a node label.
// Its own parser accepts <br>, <br/> and <br />; the shortest spelling is
// emitted so wrapping costs the layout as few columns as possible.
const labelBreak = "<br>"

// labelToken is one piece of a label that wrapping never splits.
//
// A run of narrow characters is one token, because breaking a word or a URL
// mid-way would make it unreadable. A wide character is a token of its own,
// which is what lets a Japanese label with no spaces in it wrap at all.
type labelToken struct {
	text string
	// spaced records that a space separated this token from the previous one.
	// The space is restored when both land on the same line and dropped when
	// the token starts a new one.
	spaced bool
}

// labelTokens splits label text into the pieces wrapping may reorder onto
// separate lines.
func labelTokens(text string) []labelToken {
	var tokens []labelToken
	spaced := false
	for _, r := range text {
		switch {
		case unicode.IsSpace(r):
			spaced = true
		case ansi.StringWidth(string(r)) > 1:
			tokens = append(tokens, labelToken{text: string(r), spaced: spaced})
			spaced = false
		case len(tokens) > 0 && !spaced && isNarrowRun(tokens[len(tokens)-1].text):
			tokens[len(tokens)-1].text += string(r)
		default:
			tokens = append(tokens, labelToken{text: string(r), spaced: spaced})
			spaced = false
		}
	}
	return tokens
}

// isNarrowRun reports whether text is a run of narrow characters, so the next
// narrow character may join it instead of starting a new token.
func isNarrowRun(text string) bool {
	return ansi.StringWidth(text) == len([]rune(text))
}

// wrapLabelText breaks a Mermaid node label into lines at most maxWidth columns
// wide, joined with the <br> tags mermaid-ascii treats as line breaks. Text that
// already fits is returned unchanged.
func wrapLabelText(text string, maxWidth int) string {
	if maxWidth <= 0 || ansi.StringWidth(text) <= maxWidth {
		return text
	}

	var lines []string
	current := ""
	for _, token := range labelTokens(text) {
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
