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

`converter.go`'s `Convert` branches on `Config.Format`: PDF and DOCX take **different pipelines** because each format reads best from a different source.

### PDF pipeline (Markdown → HTML → Chromium)

1. **parser.go** — goldmark parses Markdown to HTML, extracting fenced Mermaid code blocks into a `parsedDoc` struct with placeholders
2. **mermaid.go** — each Mermaid block is rendered to inline SVG via the external `mmdc` CLI (`renderMermaid`)
3. **html.go** — assembles a self-contained HTML file with GitHub CSS, `@font-face` declarations, and inlined SVGs
4. **pdf.go** — headless Chromium (via Playwright Python driver) prints the HTML to PDF

### DOCX pipeline (Markdown → pandoc, no HTML)

DOCX is produced **directly from Markdown** by pandoc's `gfm` reader rather than from HTML, so pandoc emits clean, Word-native paragraph/list styles instead of HTML-derived ones (`docx.go` `convertMarkdownDOCX`).

1. **mermaid_markdown.go** — `extractMermaidFromMarkdown` scans the raw Markdown line by line, replacing each fenced Mermaid block with a placeholder and preserving non-Mermaid fences verbatim
2. **mermaid.go** — `renderMermaidPNGs` rasterises each block to a PNG (Word cannot reliably display pandoc-embedded SVG); placeholders are then rewritten to `![](…)` image references
3. **docx.go** — pandoc converts the processed Markdown with `-f gfm`, running with the working directory set (generated diagrams resolve there) and `--resource-path` pointing at the source dir (user images resolve there). It also builds a styled reference document (`buildReferenceDoc` → `patchReferenceDoc`): table borders (`injectTableBorders`), a 10.5pt body (`setBodyFontSize`), compact headings (`shrinkHeadings`), and a Japanese-friendly font for both Latin and East Asian runs via the theme (`setThemeFonts`, default `Yu Gothic`, overridable with `-docx-font`). Reference-doc styling is best-effort and falls back to pandoc defaults on failure.

**converter.go** orchestrates both pipelines and manages a temporary working directory for intermediate files. **Config** struct holds all runtime options including `Format` ("pdf"|"docx") and `DOCXFont`.

**cmd/md2pdf/** — CLI entry point. `flags.go` handles argument parsing and auto-detection of font/mmdc paths; `resolveFormat` derives the output format from `-format` or the `-o` extension. Pandoc is resolved in `docx.go` (`findPandoc`) unless `-pandoc` is provided. `main.go` wires flags to the converter.

## External Dependencies

Runtime: `mmdc` (Mermaid CLI via npm), Python 3 + Playwright + Chromium, Noto Sans CJK JP fonts. DOCX output additionally requires `pandoc`.
Go modules: `github.com/yuin/goldmark` (Markdown parsing).

## Code Style

- All exported symbols require GoDoc comments ending with a period (godot linter)
- Comments and GoDoc in English
- Errors crossing package boundaries must be wrapped (wrapcheck)
- Go 1.22+, CI tests against Go 1.22 and 1.23
