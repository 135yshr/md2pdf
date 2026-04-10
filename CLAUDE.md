# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

md2pdf is a Go CLI tool that converts Markdown files to PDF with GitHub-flavored styling. It supports Mermaid diagrams (rendered as inline SVG) and Japanese text via Noto Sans CJK JP fonts.

## Build & Run

```sh
go build -o md2pdf ./cmd/md2pdf
go vet ./...
```

## Testing

```sh
# Unit tests only (no external dependencies needed)
go test ./internal/converter/ -run 'Test[^C]'

# All tests including integration (requires mmdc, python3 playwright, chromium, fonts-noto-cjk)
go test ./... -timeout 120s

# Single test
go test ./internal/converter/ -run TestSpecificName -v
```

## Linting

Uses golangci-lint with config in `.golangci.yml`. Key enabled linters: errcheck, gosimple, govet, staticcheck, unused, gofmt, goimports, misspell, godot, gosec, noctx, wrapcheck, exhaustive. G204 (subprocess with variable) is excluded since mmdc/python invocations are intentional. Test files have relaxed rules (no wrapcheck, gosec, errcheck).

## Architecture

The conversion pipeline flows through four stages in `internal/converter/`:

1. **parser.go** — goldmark parses Markdown to HTML, extracting fenced Mermaid code blocks into a `parsedDoc` struct with placeholders
2. **mermaid.go** — each Mermaid block is rendered to SVG via the external `mmdc` CLI
3. **html.go** — assembles a self-contained HTML file with GitHub CSS, `@font-face` declarations, and inlined SVGs
4. **pdf.go** — headless Chromium (via Playwright Python driver) prints the HTML to PDF

**converter.go** orchestrates the pipeline and manages a temporary working directory for intermediate files. **Config** struct holds all runtime options.

**cmd/md2pdf/** — CLI entry point. `flags.go` handles argument parsing and auto-detection of font/mmdc paths. `main.go` wires flags to the converter.

## External Dependencies

Runtime: `mmdc` (Mermaid CLI via npm), Python 3 + Playwright + Chromium, Noto Sans CJK JP fonts.
Go modules: `github.com/yuin/goldmark` (Markdown parsing).

## Code Style

- All exported symbols require GoDoc comments ending with a period (godot linter)
- Comments and GoDoc in English
- Errors crossing package boundaries must be wrapped (wrapcheck)
- Go 1.22+, CI tests against Go 1.22 and 1.23
