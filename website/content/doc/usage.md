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
| `-o <path>` | `<input>.pdf` | Output path (`.docx` extension implies `-format docx`; not allowed with `-format console`) |
| `-format <fmt>` | `pdf` | Output format: `pdf`, `docx`, or `console` (aliases `term`, `terminal`; inferred from `-o` extension when omitted) |
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
| `-width <cols>` | terminal width | Console word-wrap width, capped at 120 columns (`-format console`) |
| `-style <name\|path>` | `auto` | Console theme: `auto`, `dark`, `light`, `notty`, `ascii`, `dracula`, `pink`, `tokyo-night`, or a JSON stylesheet path (env: `GLAMOUR_STYLE`) |
| `-pager` | true | Page console output through `$PAGER` (default `less -R -F`); `-pager=false` writes straight to stdout |
| `-v` | false | Verbose output (progress logs go to stderr) |
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

# Read the document in the terminal
md2pdf -format console document.md
md2pdf -format console -style dark -width 100 document.md
md2pdf -format console -pager=false document.md | cat   # plain text, no color

# Explicit font path
md2pdf -font /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc document.md

# Letter size with wider margins
md2pdf -page-size Letter -margin-left 20mm -margin-right 20mm document.md

# Verbose output
md2pdf -v document.md
```

## Console output

Pass `-format console` (or `-format term`) to read a Markdown file in the
terminal instead of producing a file. The document is rendered to styled ANSI
text — headings, tables, lists, blockquotes, and syntax-highlighted code —
wrapped to your terminal width and paged through `$PAGER`.

```sh
md2pdf -format console document.md
```

Unlike PDF and DOCX output, this needs no external tools: no `mmdc`, no
Playwright, no `pandoc`. Mermaid blocks are shown as their source inside a code
block.

Behavior worth knowing:

- **Width** follows the terminal, minus a two-column margin and capped at 120
  columns so long lines stay readable on wide displays. `-width` overrides it,
  and piped output uses a fixed 80 columns.
- **Theme** is detected from the terminal background (`auto`). Set `-style` or
  the `GLAMOUR_STYLE` environment variable to choose one explicitly, or point
  `-style` at your own JSON stylesheet.
- **No color** is emitted when `NO_COLOR` is set or when the output is piped or
  redirected, so `md2pdf -format console doc.md > doc.txt` gives you plain text.
- **Paging** happens only on an interactive terminal. `$PAGER` is honored
  (`-R` is added automatically if your `less` lacks it); `-pager=false` writes
  straight to stdout.

Japanese and other East Asian text is measured by display width, so tables and
wrapped paragraphs stay aligned.

## DOCX output

Pass `-format docx` (or use a `.docx` output path) to export an editable Word
document instead of a PDF. This requires [pandoc](https://pandoc.org/) to be
installed (`brew install pandoc` / `sudo apt install pandoc`).

```sh
md2pdf -format docx document.md
```

GFM tables are exported with visible borders, and Mermaid diagrams are embedded
as PNG images so they display reliably in Word.
