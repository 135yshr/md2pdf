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

// errEmptyInput reports an empty document arriving on standard input.
var errEmptyInput = errors.New("empty input document")

// readInput returns the Markdown bytes for path, reading standard input when
// path is StdinPath.
//
// Empty input is rejected for standard input only, and deliberately not for a
// file. An empty pipe is invisible and nearly always means the command upstream
// produced nothing, so failing makes the pipeline fail too; an empty file is
// something the caller pointed at directly and could see. Erroring on it would
// also change what md2pdf did before multi-input support, which rendered an
// empty document and exited 0.
//
// Whitespace-only input counts as empty, since "echo | md2pdf -" still sends a
// newline and testing for zero bytes would let that through.
func (c *Converter) readInput(path string) ([]byte, error) {
	if path != StdinPath {
		data, err := os.ReadFile(path) //nolint:gosec // G304: reading the caller's chosen Markdown file is the point
		if err != nil {
			return nil, fmt.Errorf("read input: %w", err)
		}
		return data, nil
	}

	data, err := io.ReadAll(c.stdinReader())
	if err != nil {
		return nil, fmt.Errorf("read standard input: %w", err)
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
