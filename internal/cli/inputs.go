package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/135yshr/md2pdf/internal/converter"
)

// resolveInputs validates the positional arguments and returns the documents to
// render, in the order given.
//
// Everything is checked before any rendering starts, so a typo in the third of
// three paths fails immediately rather than after two documents have already
// been written out.
//
// Multiple inputs are console-only. Concatenating several documents into one
// PDF or DOCX would need decisions about heading level shifts, page breaks and
// per-file image bases that have no obvious answer, so those formats keep the
// single-document contract.
func (p *program) resolveInputs(args []string, format string) ([]string, error) {
	if len(args) == 0 {
		return nil, errors.New("at least one input Markdown file is required")
	}

	stdinCount := 0
	for _, arg := range args {
		if arg == converter.StdinPath {
			stdinCount++
		}
	}
	if stdinCount > 0 && len(args) > 1 {
		return nil, fmt.Errorf(
			"standard input (%q) cannot be combined with other inputs, got %d inputs",
			converter.StdinPath, len(args))
	}

	if len(args) > 1 && format != converter.FormatConsole {
		return nil, fmt.Errorf(
			"multiple input files are only supported with -format console, got %d inputs for %s",
			len(args), format)
	}

	if stdinCount == 1 {
		if err := p.stdinUsable(); err != nil {
			return nil, err
		}
		return []string{converter.StdinPath}, nil
	}

	inputs := make([]string, 0, len(args))
	for _, arg := range args {
		if err := validateInputFile(arg); err != nil {
			return nil, err
		}
		// Duplicates are kept: rendering the same file twice is a reasonable
		// thing to ask for, and silently collapsing it would be surprising.
		inputs = append(inputs, arg)
	}
	return inputs, nil
}

// validateInputFile reports whether path is an existing Markdown file, naming
// the offending path so the message is actionable with several inputs.
func validateInputFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("input file not found: %s", path)
	}
	if info.IsDir() {
		return fmt.Errorf("input is a directory, not a Markdown file: %s", path)
	}
	if !strings.EqualFold(filepath.Ext(path), ".md") {
		return fmt.Errorf("input file must have a .md extension: %s", path)
	}
	return nil
}

// stdinUsable reports whether standard input can be read from. Reading a
// terminal would block with no indication of what md2pdf is waiting for, so
// that case fails immediately with instructions instead.
func (p *program) stdinUsable() error {
	if p.stdinIsTerminal {
		return fmt.Errorf(
			"reading from standard input (%q), but standard input is a terminal; "+
				"pipe or redirect a document into %s, for example: cat doc.md | %s %s",
			converter.StdinPath, p.name, p.name, converter.StdinPath)
	}
	return nil
}

// resolveOutputPath decides where a file format writes its result. A document
// read from standard input has no filename to derive one from, so -o becomes
// mandatory there.
func resolveOutputPath(explicit string, inputs []string, format string) (string, error) {
	if format == converter.FormatConsole {
		return "", nil
	}
	if explicit != "" {
		return explicit, nil
	}
	if inputs[0] == converter.StdinPath {
		return "", fmt.Errorf(
			"-o is required when reading from standard input: there is no input filename to derive the %s name from",
			strings.ToUpper(format))
	}
	base := strings.TrimSuffix(inputs[0], filepath.Ext(inputs[0]))
	return base + "." + format, nil
}
