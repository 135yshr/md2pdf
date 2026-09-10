---
title: "Architecture"
description: "How md2pdf converts Markdown to PDF, DOCX, or terminal output — the pipelines behind each format."
weight: 30
---

md2pdf reads Markdown once and then branches on the output format. Each format
has its own pipeline, because each renders best from a different source.

The format comes from `-format`, or from the `-o` extension, or — failing both
— from the name the binary was invoked as: `mdview` defaults to `console`,
every other name to `pdf`.

## PDF pipeline (Markdown → HTML → Chromium)

1. **Parse** — goldmark converts Markdown to HTML with GFM extensions (tables, fenced code blocks, strikethrough). Mermaid code blocks are extracted and replaced with placeholders.
2. **Render diagrams** — each Mermaid block is rendered to inline SVG via the `mmdc` CLI.
3. **Build HTML** — a self-contained HTML file is assembled with GitHub-flavored CSS, `@font-face` declarations for Noto Sans CJK JP, and the rendered diagrams injected inline.
4. **Print** — a headless Chromium browser (via Playwright) loads the HTML and prints it to PDF.

## DOCX pipeline (Markdown → pandoc)

DOCX is produced **directly from Markdown** by pandoc's `gfm` reader, with no
HTML in between, so pandoc emits clean, Word-native paragraph and list styles.
Mermaid blocks are rasterized to PNG and spliced back in as image references
(Word cannot reliably display pandoc-embedded SVG). A generated reference
document supplies the styling: bordered GFM tables, a 10.5pt body, compact
headings, and a Japanese-friendly font.

## Console pipeline (Markdown → glamour → terminal)

Console output is rendered **directly from Markdown** to styled ANSI text by
[glamour](https://github.com/charmbracelet/glamour), the goldmark-based renderer
behind [glow](https://github.com/charmbracelet/glow). The wrap width follows the
terminal, the theme follows the terminal background, and the result is paged
through `$PAGER`. No external conversion tools are involved — only the pager
itself, when one is available — and Mermaid blocks stay visible as their source.

## Source layout

```
internal/converter/
  converter.go   # Orchestrates the pipelines, manages temp directory
  parser.go      # goldmark parsing
  mermaid.go     # mmdc SVG/PNG rendering
  html.go        # HTML assembly
  pdf.go         # PDF — Chromium PDF printing
  docx.go        # DOCX — pandoc conversion and reference-doc styling
  console.go     # Console — glamour ANSI rendering, width/theme/pager handling

internal/cli/
  cli.go         # Run: the entry point every binary shares
  name.go        # argv[0] -> default output format
  flags.go       # Argument parsing, auto-detection, format resolution
  inputs.go      # Input validation, output path resolution
  usage.go       # Help text, assembled per invoked name
  fonts.go       # CJK font discovery
  doctor.go      # -doctor report formatting

cmd/md2pdf/
  main.go        # Thin main: ldflags version vars, then cli.Run
```

## External dependencies

| Dependency | Purpose |
|---|---|
| [goldmark](https://github.com/yuin/goldmark) | Markdown to HTML (Go library) |
| [mmdc](https://github.com/mermaid-js/mermaid-cli) | Mermaid diagram rendering |
| [Playwright](https://playwright.dev/python/) + Chromium | HTML to PDF |
| [pandoc](https://pandoc.org/) | Markdown to DOCX (only for `-format docx`) |
| [glamour](https://github.com/charmbracelet/glamour) | Markdown to styled ANSI text (Go library, `-format console`) |
| Noto Sans CJK JP | Japanese font support |
