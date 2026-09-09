// md2pdf converts Markdown files to PDF with GitHub-style design.
//
// It renders Mermaid diagrams, supports Noto Sans CJK JP font for Japanese text,
// and uses a headless Chromium browser for high-fidelity PDF output. It can also
// export DOCX and print a styled, readable rendering straight to the terminal.
//
// Usage:
//
//	md2pdf [options] <input.md>... | -
//
// Examples:
//
//	md2pdf document.md
//	md2pdf -o output.pdf document.md
//	md2pdf -format html -css custom.css document.md
//	md2pdf -format console document.md
//	md2pdf -font /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc document.md
package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/135yshr/md2pdf/internal/converter"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

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

func main() {
	cfg, err := parseFlags(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "md2pdf: %v\n", err)
		printUsage()
		os.Exit(1)
	}

	c, err := converter.New(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "md2pdf: failed to initialize converter: %v\n", err)
		os.Exit(1)
	}
	defer c.Close()

	// Console format renders the document to stdout, so progress chatter would
	// end up mixed into the document itself.
	quiet := cfg.Format == converter.FormatConsole

	if !quiet {
		fmt.Printf("Converting %s ...\n", describeInputs(cfg.InputFiles))
	}
	if err := c.Convert(cfg.InputFiles, cfg.OutputFile); err != nil {
		fmt.Fprintf(os.Stderr, "md2pdf: conversion failed: %v\n", err)
		os.Exit(1)
	}

	if !quiet {
		fmt.Printf("%s saved to %s\n", strings.ToUpper(cfg.Format), cfg.OutputFile)
	}
}
