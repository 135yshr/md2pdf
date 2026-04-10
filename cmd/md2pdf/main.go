// md2pdf converts Markdown files to PDF with GitHub-style design.
//
// It renders Mermaid diagrams, supports Noto Sans CJK JP font for Japanese text,
// and uses a headless Chromium browser for high-fidelity PDF output.
//
// Usage:
//
//	md2pdf [options] <input.md>
//
// Examples:
//
//	md2pdf document.md
//	md2pdf -o output.pdf document.md
//	md2pdf -font /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc document.md
package main

import (
	"fmt"
	"os"

	"github.com/135yshr/md2pdf/internal/converter"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

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

	fmt.Printf("Converting %s ...\n", cfg.InputFile)
	if err := c.Convert(cfg.InputFile, cfg.OutputFile); err != nil {
		fmt.Fprintf(os.Stderr, "md2pdf: conversion failed: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("PDF saved to %s\n", cfg.OutputFile)
}
