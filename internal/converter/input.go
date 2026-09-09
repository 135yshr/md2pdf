package converter

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// StdinPath is the input path that means "read the document from standard
// input" rather than from a file.
const StdinPath = "-"

// errEmptyInput reports an input document with no content. Empty input is
// almost always an upstream failure in a pipeline rather than a request to
// render nothing, so it fails loudly instead of producing an empty document.
var errEmptyInput = errors.New("empty input document")

// readInput returns the Markdown bytes for path, reading standard input when
// path is StdinPath. Input consisting only of whitespace counts as empty: a
// bare "echo | md2pdf -" still sends a newline, so testing for zero bytes
// would let that through.
func (c *Converter) readInput(path string) ([]byte, error) {
	var (
		data []byte
		err  error
	)
	if path == StdinPath {
		data, err = io.ReadAll(c.stdinReader())
		if err != nil {
			return nil, fmt.Errorf("read standard input: %w", err)
		}
	} else {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read input: %w", err)
		}
	}

	if len(bytes.TrimSpace(data)) == 0 {
		return nil, fmt.Errorf("%w: %s", errEmptyInput, describeInput(path))
	}
	return data, nil
}

// stdinReader returns the reader standard input is taken from, defaulting to
// os.Stdin. The field is a seam so tests can supply a document directly.
func (c *Converter) stdinReader() io.Reader {
	if c.stdin != nil {
		return c.stdin
	}
	return os.Stdin
}

// inputDir returns the directory that relative paths inside the document
// resolve against: the input file's own directory, or the working directory for
// standard input, which has no location of its own to anchor them to.
func inputDir(path string) (string, error) {
	if path == StdinPath {
		cwd, err := os.Getwd()
		if err != nil {
			return "", fmt.Errorf("resolve working directory: %w", err)
		}
		return cwd, nil
	}
	dir, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", fmt.Errorf("resolve input dir: %w", err)
	}
	return dir, nil
}

// describeInput names an input for use in messages, since "-" on its own reads
// poorly in an error.
func describeInput(path string) string {
	if path == StdinPath {
		return "standard input"
	}
	return path
}
