---
title: "Architecture"
description: "How md2pdf converts Markdown to PDF — the four-stage pipeline."
weight: 30
---

md2pdf converts Markdown to PDF through a four-stage pipeline.

## Pipeline overview

1. **Parse** — goldmark converts Markdown to HTML with GFM extensions (tables, fenced code blocks, strikethrough). Mermaid code blocks are extracted and replaced with placeholders.
2. **Render diagrams** — each Mermaid block is rendered to SVG via the `mmdc` CLI.
3. **Build HTML** — a self-contained HTML file is assembled with GitHub-flavored CSS, `@font-face` declarations for Noto Sans CJK JP, and the rendered SVGs injected inline.
4. **Print PDF** — a headless Chromium browser (via Playwright) loads the HTML and prints it to PDF.

## Source layout

```
internal/converter/
  converter.go   # Orchestrates the pipeline, manages temp directory
  parser.go      # Stage 1 — goldmark parsing
  mermaid.go     # Stage 2 — mmdc SVG rendering
  html.go        # Stage 3 — HTML assembly
  pdf.go         # Stage 4 — Chromium PDF printing

cmd/md2pdf/
  main.go        # CLI entry point
  flags.go       # Argument parsing, auto-detection
```

## External dependencies

| Dependency | Purpose |
|---|---|
| [goldmark](https://github.com/yuin/goldmark) | Markdown to HTML (Go library) |
| [mmdc](https://github.com/mermaid-js/mermaid-cli) | Mermaid diagram rendering |
| [Playwright](https://playwright.dev/python/) + Chromium | HTML to PDF |
| Noto Sans CJK JP | Japanese font support |
