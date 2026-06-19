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
2. **mermaid.go** — each Mermaid block is rendered via the external `mmdc` CLI: to inline SVG for PDF, or to a PNG file for DOCX (since Word cannot reliably display pandoc-embedded SVG)
3. **html.go** — assembles a self-contained HTML file with GitHub CSS, `@font-face` declarations, and either inlined SVGs (PDF) or `<img>` tags pointing at the PNGs (DOCX)
4. Final stage — selected by `Config.Format`:
   - **pdf.go** (default) — headless Chromium (via Playwright Python driver) prints the HTML to PDF
   - **docx.go** — the external `pandoc` CLI converts the assembled HTML to DOCX (run with the working directory set so relative image paths resolve). It also generates a reference document from pandoc's default, patching the Table style with borders (`addTableBorders`/`injectTableBorders`) so GFM tables render as a visible grid; this is best-effort and falls back to pandoc defaults on failure.

**converter.go** orchestrates the pipeline and manages a temporary working directory for intermediate files. **Config** struct holds all runtime options including `Format` ("pdf"|"docx").

**cmd/md2pdf/** — CLI entry point. `flags.go` handles argument parsing and auto-detection of font/mmdc paths; `resolveFormat` derives the output format from `-format` or the `-o` extension. Pandoc is resolved in `docx.go` (`findPandoc`) unless `-pandoc` is provided. `main.go` wires flags to the converter.

## External Dependencies

Runtime: `mmdc` (Mermaid CLI via npm), Python 3 + Playwright + Chromium, Noto Sans CJK JP fonts. DOCX output additionally requires `pandoc`.
Go modules: `github.com/yuin/goldmark` (Markdown parsing).

## Code Style

- All exported symbols require GoDoc comments ending with a period (godot linter)
- Comments and GoDoc in English
- Errors crossing package boundaries must be wrapped (wrapcheck)
- Go 1.22+, CI tests against Go 1.22 and 1.23
