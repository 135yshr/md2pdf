---
title: "Getting Started"
description: "Install md2pdf and convert your first Markdown file to PDF."
weight: 10
---

## Install md2pdf

### Homebrew (macOS / Linux)

```sh
brew install 135yshr/tap/md2pdf
```

This installs two commands: `md2pdf`, and `mdview`, which is the same binary
under a second name and renders to the terminal by default.

### Go install

```sh
go install github.com/135yshr/md2pdf/cmd/md2pdf@latest

# go install can only produce one name; link the second one yourself.
ln -s "$(go env GOPATH)/bin/md2pdf" "$(go env GOPATH)/bin/mdview"
```

### Build from source

```sh
git clone https://github.com/135yshr/md2pdf.git
cd md2pdf
go build -o md2pdf ./cmd/md2pdf
ln -s md2pdf mdview        # optional second name, defaults to terminal output
```

## Install runtime dependencies

md2pdf uses external tools for diagram rendering and PDF generation.

```sh
# Mermaid CLI (diagram rendering)
npm install -g @mermaid-js/mermaid-cli

# Playwright + Chromium (PDF generation)
pip install playwright
playwright install chromium
```

### pandoc (optional — for DOCX output)

Only required when exporting Word documents with `-format docx`:

```sh
brew install pandoc          # macOS / Linux (Homebrew)
# sudo apt install pandoc    # Ubuntu / Debian
```

## Install fonts (optional)

For Japanese text support, install the Noto Sans CJK JP font:

**macOS**
```sh
brew install font-noto-sans-cjk
```

**Ubuntu / Debian**
```sh
sudo apt install fonts-noto-cjk
```

## Convert your first document

```sh
md2pdf document.md
```

A `document.pdf` file will be generated in the same directory. To export a Word
document instead, run `md2pdf -format docx document.md` (requires pandoc).
