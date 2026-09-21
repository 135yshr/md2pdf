package converter

import (
	"bytes"
	"fmt"

	"go.yaml.in/yaml/v3"
)

// frontMatter is the metadata parsed from a document's leading YAML block.
// A nil value means the document had none.
type frontMatter map[string]any

// String returns the value of key when it is a string, and "" otherwise, so a
// caller can read an optional text field without type-switching.
func (m frontMatter) String(key string) string {
	s, _ := m[key].(string)
	return s
}

// Bool reports whether key is the YAML boolean true. A string such as "yes"
// or "true" is not, since YAML already has a boolean for that.
func (m frontMatter) Bool(key string) bool {
	b, _ := m[key].(bool)
	return b
}

// document is one input read and split into its metadata and its Markdown.
type document struct {
	// meta is the parsed front-matter, nil when the document has none.
	meta frontMatter
	// body is the Markdown with the front-matter block removed.
	body []byte
}

// readDocument reads path like readInput and separates the front-matter from
// the Markdown, so every format sees the same body. A front-matter error names
// the input, since with several inputs the YAML message alone does not say
// which one is broken.
func (c *Converter) readDocument(path string) (document, error) {
	src, err := c.readInput(path)
	if err != nil {
		return document{}, err
	}
	meta, body, err := splitFrontMatter(src)
	if err != nil {
		return document{}, fmt.Errorf("%s: %w", describeInput(path), err)
	}
	return document{meta: meta, body: body}, nil
}

// splitFrontMatter removes a leading YAML front-matter block from src and
// returns its parsed contents together with the remaining Markdown.
//
// A block opens with "---" on the very first line and closes at the next line
// that is "---" or "..." (trailing blanks allowed on either). Anything else
// leaves src untouched with nil metadata: a block that is never closed, or one
// that is not on the first line, is ordinary Markdown — a thematic break — and
// was rendered as such before front-matter was recognized. The same holds for
// a closed block whose YAML is a scalar or a sequence, like "---\nSome text\n---":
// only a mapping is metadata, and treating prose as metadata would silently
// delete it from the document. YAML that fails to parse is an error, since the
// author evidently meant it as metadata.
//
// A leading UTF-8 byte order mark, which Windows editors commonly write, is
// skipped when looking for the opening delimiter and removed along with the
// block; a document without front-matter keeps it, since nothing is changed.
func splitFrontMatter(src []byte) (frontMatter, []byte, error) {
	first, rest, ok := cutLine(bytes.TrimPrefix(src, []byte("\ufeff")))
	if !ok || !isDelimiter(first, "---") {
		return nil, src, nil
	}

	var yamlSrc []byte
	body := rest
	closed := false
	for len(body) > 0 {
		line, next, _ := cutLine(body)
		if isDelimiter(line, "---") || isDelimiter(line, "...") {
			yamlSrc = rest[:len(rest)-len(body)]
			body = next
			closed = true
			break
		}
		body = next
	}
	if !closed {
		return nil, src, nil
	}

	var parsed any
	if err := yaml.Unmarshal(yamlSrc, &parsed); err != nil {
		return nil, nil, fmt.Errorf("parse front-matter: %w", err)
	}
	if parsed == nil {
		return frontMatter{}, body, nil
	}
	mapping, isMap := parsed.(map[string]any)
	if !isMap {
		return nil, src, nil
	}
	return frontMatter(mapping), body, nil
}

// cutLine splits off the first line of b, without its line terminator, and
// returns the remainder after it. The last result is false only when b is
// empty.
func cutLine(b []byte) (line, rest []byte, ok bool) {
	if len(b) == 0 {
		return nil, nil, false
	}
	i := bytes.IndexByte(b, '\n')
	if i < 0 {
		return bytes.TrimSuffix(b, []byte("\r")), nil, true
	}
	return bytes.TrimSuffix(b[:i], []byte("\r")), b[i+1:], true
}

// isDelimiter reports whether line is exactly delim, ignoring trailing blanks.
func isDelimiter(line []byte, delim string) bool {
	return string(bytes.TrimRight(line, " \t")) == delim
}
