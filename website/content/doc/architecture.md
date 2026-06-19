---
title: "Architecture"
description: "How md2pdf converts Markdown to PDF or DOCX — the four-stage pipeline."
weight: 30
---

md2pdf converts Markdown to PDF or DOCX through a four-stage pipeline. The first
three stages are shared; the final stage is selected by the output format.

## Pipeline overview

1. **Parse** — goldmark converts Markdown to HTML with GFM extensions (tables, fenced code blocks, strikethrough). Mermaid code blocks are extracted and replaced with placeholders.
2. **Render diagrams** — each Mermaid block is rendered via the `mmdc` CLI: to inline SVG for PDF, or to a PNG image for DOCX (Word cannot reliably display pandoc-embedded SVG).
3. **Build HTML** — a self-contained HTML file is assembled with GitHub-flavored CSS, `@font-face` declarations for Noto Sans CJK JP, and the rendered diagrams injected inline.
4. **Render output** — depending on `-format`:
   - **PDF** (default): a headless Chromium browser (via Playwright) loads the HTML and prints it to PDF.
   - **DOCX**: `pandoc` converts the HTML to a Word document, using a generated reference document so GFM tables render with visible borders.

## Source layout

```
internal/converter/
  converter.go   # Orchestrates the pipeline, manages temp directory
  parser.go      # Stage 1 — goldmark parsing
  mermaid.go     # Stage 2 — mmdc SVG/PNG rendering
  html.go        # Stage 3 — HTML assembly
  pdf.go         # Stage 4 (pdf) — Chromium PDF printing
  docx.go        # Stage 4 (docx) — pandoc DOCX conversion

cmd/md2pdf/
  main.go        # CLI entry point
  flags.go       # Argument parsing, auto-detection, format resolution
```

## External dependencies

| Dependency | Purpose |
|---|---|
| [goldmark](https://github.com/yuin/goldmark) | Markdown to HTML (Go library) |
| [mmdc](https://github.com/mermaid-js/mermaid-cli) | Mermaid diagram rendering |
| [Playwright](https://playwright.dev/python/) + Chromium | HTML to PDF |
| [pandoc](https://pandoc.org/) | HTML to DOCX (only for `-format docx`) |
| Noto Sans CJK JP | Japanese font support |
