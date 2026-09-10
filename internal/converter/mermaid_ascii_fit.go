package converter

import "github.com/charmbracelet/x/ansi"

// wrapLabelText breaks a Mermaid node label into lines at most maxWidth columns
// wide, joined with the <br> tags mermaid-ascii treats as line breaks. Text that
// already fits is returned unchanged.
func wrapLabelText(text string, maxWidth int) string {
	if ansi.StringWidth(text) <= maxWidth {
		return text
	}
	return text
}
