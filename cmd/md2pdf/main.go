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
	os.Exit(run())
}

// run executes the command line under a context an interrupt cancels, and
// returns the exit code. It is separate from main so that its defers run:
// os.Exit does not execute them.
func run() int {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// The first interrupt cancels the context, which stops the external tools
	// the run started: without it, Ctrl-C leaves mmdc, pandoc and the headless
	// browser running. The second one exits outright.
	//
	// The second one has to be handled here rather than left to the runtime.
	// Neither signal.Stop nor signal.Reset restores the default behavior once
	// the process has asked to be notified — the signal is swallowed from then
	// on — so a stage that cannot observe the context, such as reading a
	// standard input that never reaches EOF, would otherwise leave the command
	// impossible to interrupt at all.
	sig := make(chan os.Signal, 2)
	signal.Notify(sig, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sig)

	go func() {
		<-sig
		cancel()

		second := <-sig
		code := 130 // 128 + SIGINT, the shell's convention for an interrupt.
		if s, ok := second.(syscall.Signal); ok {
			code = 128 + int(s)
		}
		os.Exit(code)
	}()

	return cli.Run(ctx, os.Args, cli.BuildInfo{Version: version, Commit: commit, Date: date})
}
