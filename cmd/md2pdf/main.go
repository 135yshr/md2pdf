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
//
// The command line itself lives in internal/cli, so that more than one binary
// can share it.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/135yshr/md2pdf/internal/cli"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	// Interrupting the command cancels the external tools it started. Without
	// this, Ctrl-C leaves mmdc, pandoc and the headless browser running.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	os.Exit(cli.Run(ctx, os.Args, cli.BuildInfo{Version: version, Commit: commit, Date: date}))
}
