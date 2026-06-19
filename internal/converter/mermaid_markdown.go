package converter

import (
	"fmt"
	"strings"
)

// fence describes an opened Markdown code fence: the fence character (backtick
// or tilde), its run length and the language token from the info string.
type fence struct {
	char byte
	n    int
	lang string
}

// extractMermaidFromMarkdown scans Markdown for fenced code blocks tagged
// "mermaid", replacing each with a single-line placeholder token and returning
// the rewritten Markdown together with the extracted blocks in document order.
// Non-Mermaid fenced blocks are copied verbatim, so a ```mermaid example shown
// inside another code block is left untouched.
func extractMermaidFromMarkdown(src string) (string, []*mermaidBlock) {
	lines := strings.Split(src, "\n")
	out := make([]string, 0, len(lines))
	var blocks []*mermaidBlock

	for i := 0; i < len(lines); {
		open, ok := parseFenceOpen(lines[i])
		if !ok {
			out = append(out, lines[i])
			i++
			continue
		}

		// Gather the block body up to the matching closing fence (or EOF).
		var body []string
		j := i + 1
		for j < len(lines) && !isFenceClose(lines[j], open) {
			body = append(body, lines[j])
			j++
		}
		closed := j < len(lines)

		if strings.EqualFold(open.lang, "mermaid") {
			placeholder := fmt.Sprintf("MD2PDFMERMAIDPLACEHOLDER%d", len(blocks))
			blocks = append(blocks, &mermaidBlock{
				Source:      strings.Join(body, "\n") + "\n",
				Placeholder: placeholder,
			})
			out = append(out, placeholder)
		} else {
			out = append(out, lines[i])
			out = append(out, body...)
			if closed {
				out = append(out, lines[j])
			}
		}

		if closed {
			i = j + 1
		} else {
			i = j
		}
	}

	return strings.Join(out, "\n"), blocks
}

// parseFenceOpen reports whether line opens a fenced code block, returning the
// fence descriptor when it does. Lines indented four or more spaces are treated
// as indented code rather than fences.
func parseFenceOpen(line string) (fence, bool) {
	rest, ok := stripCodeIndent(line)
	if !ok {
		return fence{}, false
	}

	var ch byte
	switch {
	case strings.HasPrefix(rest, "```"):
		ch = '`'
	case strings.HasPrefix(rest, "~~~"):
		ch = '~'
	default:
		return fence{}, false
	}

	n := 0
	for n < len(rest) && rest[n] == ch {
		n++
	}

	info := strings.TrimSpace(rest[n:])
	// A backtick info string may not contain a backtick (CommonMark §4.5).
	if ch == '`' && strings.Contains(info, "`") {
		return fence{}, false
	}

	lang := ""
	if fields := strings.Fields(info); len(fields) > 0 {
		lang = fields[0]
	}
	return fence{char: ch, n: n, lang: lang}, true
}

// isFenceClose reports whether line closes the given open fence: a run of the
// same fence character at least as long as the opener, followed only by spaces.
func isFenceClose(line string, open fence) bool {
	rest, ok := stripCodeIndent(line)
	if !ok {
		return false
	}

	n := 0
	for n < len(rest) && rest[n] == open.char {
		n++
	}
	if n < open.n {
		return false
	}
	return strings.TrimSpace(rest[n:]) == ""
}

// stripCodeIndent removes up to three leading spaces. Four or more spaces make
// the line an indented code block rather than a fence, so it returns ok=false.
func stripCodeIndent(line string) (string, bool) {
	indent := 0
	for indent < len(line) && line[indent] == ' ' {
		indent++
	}
	if indent >= 4 {
		return "", false
	}
	return line[indent:], true
}
