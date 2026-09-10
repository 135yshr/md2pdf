package converter

import (
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// labelBreak is the line break mermaid-ascii understands inside a node label.
// Its own parser accepts <br>, <br/> and <br />; the shortest spelling is
// emitted so wrapping costs the layout as few columns as possible.
const labelBreak = "<br>"

// wrapLabelText breaks a Mermaid node label into lines at most maxWidth columns
// wide, joined with the <br> tags mermaid-ascii treats as line breaks. Text that
// already fits is returned unchanged.
func wrapLabelText(text string, maxWidth int) string {
	if maxWidth <= 0 || ansi.StringWidth(text) <= maxWidth {
		return text
	}

	var lines []string
	current := ""
	for _, word := range strings.Fields(text) {
		switch {
		case current == "":
			current = word
		case ansi.StringWidth(current)+1+ansi.StringWidth(word) <= maxWidth:
			current += " " + word
		default:
			lines = append(lines, current)
			current = word
		}
	}
	if current != "" {
		lines = append(lines, current)
	}
	return strings.Join(lines, labelBreak)
}
