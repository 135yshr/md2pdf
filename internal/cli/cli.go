// Package cli implements the md2pdf command line: flag parsing, input
// validation and the wiring that drives internal/converter.
//
// It is a package rather than a main so that more than one binary can share
// it, and so that the parts that used to end in os.Exit can be tested.
package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/135yshr/md2pdf/internal/converter"
)

// BuildInfo carries the version metadata a binary was linked with.
type BuildInfo struct {
	Version string
	Commit  string
	Date    string
}

// Run executes one invocation of the command line and returns the process exit
// code. argv is the whole argument vector, os.Args, because the name the binary
// was invoked under selects the default output format.
func Run(argv []string, build BuildInfo) int {
	var args []string
	if len(argv) > 1 {
		args = argv[1:]
	}
	p := &program{
		build:           build,
		diagnose:        converter.Diagnose,
		stdout:          os.Stdout,
		stderr:          os.Stderr,
		name:            defaultProgramName,
		defaultFormat:   converter.FormatPDF,
		stdinIsTerminal: term.IsTerminal(os.Stdin.Fd()),
	}
	return p.run(args)
}

// defaultProgramName is the name this command is known by.
const defaultProgramName = "md2pdf"

// program is one invocation of the command line: the name it was invoked
// under, the output format that name implies, the streams it writes to and the
// build it was linked from.
type program struct {
	build           BuildInfo
	diagnose        func(*converter.Config) converter.Report
	stdout          io.Writer
	stderr          io.Writer
	name            string
	defaultFormat   string
	stdinIsTerminal bool
}

// exitError reports that the invocation finished during flag parsing, as
// -version and -doctor do, and carries the status to exit with.
type exitError struct{ code int }

// Error implements the error interface.
func (e exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// run parses args, converts the documents they name and returns the process
// exit code.
func (p *program) run(args []string) int {
	cfg, err := p.parseFlags(args)
	var done exitError
	switch {
	case errors.As(err, &done):
		return done.code
	case err != nil:
		fmt.Fprintf(p.stderr, "%s: %v\n", p.name, err)
		p.printUsage()
		return 1
	}

	c, err := converter.New(cfg)
	if err != nil {
		fmt.Fprintf(p.stderr, "%s: failed to initialize converter: %v\n", p.name, err)
		return 1
	}
	defer c.Close()

	// Console format renders the document to stdout, so progress chatter would
	// end up mixed into the document itself.
	quiet := cfg.Format == converter.FormatConsole

	if !quiet {
		fmt.Fprintf(p.stdout, "Converting %s ...\n", describeInputs(cfg.InputFiles))
	}
	if err := c.Convert(cfg.InputFiles, cfg.OutputFile); err != nil {
		fmt.Fprintf(p.stderr, "%s: conversion failed: %v\n", p.name, err)
		return 1
	}

	if !quiet {
		fmt.Fprintf(p.stdout, "%s saved to %s\n", strings.ToUpper(cfg.Format), cfg.OutputFile)
	}
	return 0
}

// describeInputs names the inputs for progress output, since "-" on its own
// reads poorly in a message.
func describeInputs(inputs []string) string {
	named := make([]string, 0, len(inputs))
	for _, in := range inputs {
		if in == converter.StdinPath {
			named = append(named, "standard input")
			continue
		}
		named = append(named, in)
	}
	return strings.Join(named, ", ")
}
