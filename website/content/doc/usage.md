---
title: "Usage"
description: "Command-line options and examples for md2pdf."
weight: 20
---

## Basic usage

```sh
md2pdf [options] <input.md>
```

## Options

| Flag | Default | Description |
|---|---|---|
| `-o <path>` | `<input>.pdf` | Output path (`.docx` extension implies `-format docx`) |
| `-format <fmt>` | `pdf` | Output format: `pdf` or `docx` (inferred from `-o` extension when omitted) |
| `-font <path>` | auto-detected | Noto Sans CJK JP Regular font |
| `-font-bold <path>` | auto-detected | Noto Sans CJK JP Bold font |
| `-font-medium <path>` | auto-detected | Noto Sans CJK JP Medium font |
| `-mmdc <path>` | auto-detected | Path to `mmdc` binary |
| `-pandoc <path>` | auto-detected | Path to `pandoc` binary (used for `-format docx`) |
| `-puppeteer-config <f>` | auto-generated | Puppeteer JSON config for mmdc |
| `-page-size <size>` | `A4` | `A4`, `Letter`, or `A3` |
| `-margin-top <m>` | `18mm` | Top margin |
| `-margin-bottom <m>` | `18mm` | Bottom margin |
| `-margin-left <m>` | `14mm` | Left margin |
| `-margin-right <m>` | `14mm` | Right margin |
| `-v` | false | Verbose output |
| `-version` | — | Print version and exit |

## Examples

```sh
# Basic conversion
md2pdf document.md

# Custom output path
md2pdf -o report.pdf document.md

# Export to Word (DOCX) — requires pandoc
md2pdf -format docx document.md
md2pdf -o report.docx document.md   # format inferred from extension

# Explicit font path
md2pdf -font /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc document.md

# Letter size with wider margins
md2pdf -page-size Letter -margin-left 20mm -margin-right 20mm document.md

# Verbose output
md2pdf -v document.md
```

## DOCX output

Pass `-format docx` (or use a `.docx` output path) to export an editable Word
document instead of a PDF. This requires [pandoc](https://pandoc.org/) to be
installed (`brew install pandoc` / `sudo apt install pandoc`).

```sh
md2pdf -format docx document.md
```

GFM tables are exported with visible borders, and Mermaid diagrams are embedded
as PNG images so they display reliably in Word.
