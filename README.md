# md2pdf

A CLI tool that converts Markdown files to PDF with GitHub-flavored styling.

- Renders **Mermaid diagrams** (flowchart, sequence, etc.) as inline SVG
- GitHub-flavored Markdown: tables, fenced code blocks, strikethrough
- **Noto Sans CJK JP** font support for Japanese text
- Configurable page size and margins

## Requirements

| Dependency | Purpose | Install |
|---|---|---|
| Go 1.26+ | Build | https://go.dev |
| [mmdc](https://github.com/mermaid-js/mermaid-cli) | Mermaid → SVG | `npm install -g @mermaid-js/mermaid-cli` |
| Python 3 + [playwright](https://playwright.dev/python/) | HTML → PDF | `pip install playwright && playwright install chromium` |
| Noto Sans CJK JP | Japanese font (optional) | See below |

### Installing Noto Sans CJK JP

**Ubuntu / Debian**
```sh
sudo apt install fonts-noto-cjk
```

**macOS (Homebrew)**
```sh
brew install font-noto-sans-cjk
```

## Installation

### Homebrew (macOS / Linux)

```sh
brew install 135yshr/tap/md2pdf
```

### Go install

```sh
go install github.com/135yshr/md2pdf/cmd/md2pdf@latest
```

### Build from source

```sh
git clone https://github.com/135yshr/md2pdf.git
cd md2pdf
go build -o md2pdf ./cmd/md2pdf
```

## Usage

```sh
md2pdf [options] <input.md>
```

### Options

| Flag | Default | Description |
|---|---|---|
| `-o <path>` | `<input>.pdf` | Output PDF path |
| `-font <path>` | auto-detected | Noto Sans CJK JP Regular font |
| `-font-bold <path>` | auto-detected | Noto Sans CJK JP Bold font |
| `-font-medium <path>` | auto-detected | Noto Sans CJK JP Medium font |
| `-mmdc <path>` | auto-detected | Path to `mmdc` binary |
| `-puppeteer-config <f>` | auto-generated | Puppeteer JSON config for mmdc |
| `-page-size <size>` | `A4` | `A4`, `Letter`, or `A3` |
| `-margin-top <m>` | `18mm` | Top margin |
| `-margin-bottom <m>` | `18mm` | Bottom margin |
| `-margin-left <m>` | `14mm` | Left margin |
| `-margin-right <m>` | `14mm` | Right margin |
| `-v` | false | Verbose output |
| `-version` | — | Print version and exit |

### Examples

```sh
# Basic conversion
md2pdf document.md

# Custom output path
md2pdf -o report.pdf document.md

# Explicit font path
md2pdf -font /usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc document.md

# Letter size with wider margins
md2pdf -page-size Letter -margin-left 20mm -margin-right 20mm document.md

# Verbose output
md2pdf -v document.md
```

## How It Works

```
Markdown (.md)
     │
     ▼
[goldmark parser]  ──── extracts Mermaid blocks
     │                        │
     │                        ▼
     │                  [mmdc CLI]
     │                  Mermaid → SVG
     │                        │
     ▼                        ▼
[HTML builder] ── injects SVGs + GitHub CSS + @font-face
     │
     ▼
[Playwright / Chromium]
     │
     ▼
   PDF output
```

1. **Parse** — goldmark converts Markdown to HTML (GFM tables, fenced code blocks). Mermaid code blocks are extracted and replaced with placeholders.
2. **Render diagrams** — each Mermaid block is rendered to SVG via the `mmdc` CLI.
3. **Build HTML** — a self-contained HTML file is assembled with GitHub-flavored CSS, `@font-face` declarations for Noto Sans CJK JP, and the rendered SVGs injected inline.
4. **Print PDF** — a headless Chromium browser (via Playwright) loads the HTML and prints it to PDF.

## Running Tests

```sh
# Unit tests only
go test ./internal/converter/ -run 'Test[^C]'

# All tests including integration (requires mmdc + python3 playwright)
go test ./...

# With verbose output
go test -v ./...
```

## Contributing

Contributions are welcome! Please read [CONTRIBUTING.md](CONTRIBUTING.md) before opening a pull request.

## License

MIT License — see [LICENSE](LICENSE).
