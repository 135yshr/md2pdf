# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

md2pdf is a Go CLI tool that converts Markdown files to PDF with GitHub-flavored styling. It supports Mermaid diagrams (rendered as inline SVG) and Japanese text via Noto Sans CJK JP fonts. It also exports DOCX and renders documents as styled ANSI text for reading in a terminal.

## Build & Run

```sh
go build -o md2pdf ./cmd/md2pdf
go vet ./...
```

## Testing

```sh
# Everything that needs no external tools (this is what CI's unit job runs)
go test ./...

# Adds the integration suite (requires mmdc, python3 playwright, chromium, fonts-noto-cjk)
go test ./... -tags integration -timeout 120s

# Single test
go test ./internal/converter/ -run TestSpecificName -v
```

## Linting

Uses golangci-lint with config in `.golangci.yml`. Key enabled linters: errcheck, gosimple, govet, staticcheck, unused, gofmt, goimports, misspell, godot, gosec, noctx, wrapcheck, exhaustive. G204 (subprocess with variable) is excluded since mmdc/python invocations are intentional. Test files have relaxed rules (no wrapcheck, gosec, errcheck).

## Architecture

`converter.go`'s `Convert` branches on `Config.Format` (`FormatPDF` / `FormatDOCX` / `FormatConsole`): each format takes a **different pipeline** because each reads best from a different source.

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

### Console pipeline (Markdown → glamour → terminal)

Console output is rendered **directly from Markdown** to styled ANSI text by [glamour](https://github.com/charmbracelet/glamour) (`charm.land/glamour/v2`), which is itself goldmark-based, so no HTML stage is involved (`console.go`).

1. **console.go** — `detectConsoleEnv` inspects the output file: TTY status, terminal size, `NO_COLOR`, `GLAMOUR_STYLE`, and the color profile
2. `resolveConsoleWidth` follows the terminal width less a 2-column margin, capped at 120 and floored at 20; non-terminals get a fixed 80
3. `resolveConsoleStyle` forces `notty` when `NO_COLOR` is set or the output is not a terminal; otherwise `-style` wins over `GLAMOUR_STYLE`, and `auto` resolves to `dark`/`light` via `lipgloss.HasDarkBackground`
4. `renderConsoleMarkdown` renders through glamour. Mermaid handling lives in **mermaid_console.go**: `resolveConsoleMermaidPlan` turns `-mermaid-render` (`auto`/`image`/`ascii`/`source`) plus the TTY/`NO_COLOR` state and `detectImageProtocol` (kitty / iTerm2 / Sixel, from `TERM` and `TERM_PROGRAM`) into a plan; `prepareConsoleMermaid` swaps each block for a short token and rasterises it via the `Converter.rasterizeMermaid` seam, and `encodeTerminalImage` emits the protocol's escape sequence through `x/ansi`. `spliceConsoleDiagrams` then replaces each token's line in glamour's output, keeping its indent. Failures split into two paths: `resolveConsoleMermaidPlan` **returns an error** when `-mermaid-render image` is asked for but the output is not a TTY, `NO_COLOR` is set, or no protocol was detected (`auto` degrades to `ascii` instead); once a plan is chosen, `prepareConsoleMermaid` **falls back with exit status 0** — a missing `mmdc` or a failed rasterisation drops to text art, a diagram type text art cannot draw drops to the fenced source, and a width too narrow to hold a token abandons drawing for the whole document. The one exception is `plan.strict`, set when the user named `image`: there a missing `mmdc` or a failed rasterisation is an error, so the flag is a guarantee rather than a preference.

The fallback chain proper (`image → ascii → source`) is `renderConsoleBlock`, applied per diagram, so one unrenderable block never affects the others. **mermaid_ascii.go** owns the `ascii` step: `asciiDiagramSupported` gates on an explicit allowlist (`flowchart`, `graph`, `sequenceDiagram`) rather than trusting the library to reject a type — mermaid-ascii's graph parser is the fallback for any unrecognized header, and its `erDiagram` output is misaligned, so both are excluded here. `renderMermaidASCII` then calls `mermaid-ascii`'s `render.RenderDiagram` behind a `recover()`, because a layout panic on a fallback path must degrade rather than kill the run
5. Output goes through `$PAGER` (default `less -R -F`, with `-R` injected when the user's `less` lacks it) when writing to a TTY and `-pager` is on; colors are downsampled by `colorprofile` to what the terminal supports. **The pager is skipped whenever inline images were drawn**, since kitty and iTerm2 sequences do not survive `less`

Two details in the console Mermaid path are load-bearing:

- The token left for glamour is short (`MD2PDFDG<n>`, not the long `mermaidPlaceholderPrefix` the DOCX path uses) because glamour **hard-wraps a word wider than the wrap width**, which would split the token and lose the diagram. `consoleTokenFits` bails out to source output when even the short token cannot fit an explicit `-width`.
- `Converter.rasterizeMermaid` and `Converter.mermaidAvailable` are struct fields so tests can stand in for `mmdc`; the repo's own CI is the only place the real binary runs.
- Clipping text art to the wrap width happens in `spliceConsoleDiagrams`, not in `renderMermaidASCII`, because only the splice knows how far glamour indented the diagram. `consoleDiagram.clip` marks which content is safe to trim — text art is, an image escape sequence is not.
- `consoleWantsPureASCII` reads the **`-style` flag and `GLAMOUR_STYLE`**, not just the resolved style, because `resolveConsoleStyle` collapses redirected output to `notty` and would otherwise throw away an explicit `-style ascii`.
- Whether to skip the pager is decided by `anyImageDiagram(diagrams)`, **not** by `plan.emitsImages()`. An image plan still yields text art when `mmdc` is missing, and text art must keep paging.
- `mermaidDiagramHeader` strips YAML frontmatter with the renderer's own `diagram.StripFrontmatter`, or a diagram with a `---` title block would be rejected as an unsupported type before the renderer (which strips it) ever ran.

Because console format writes the document to stdout, `Converter.logf` writes verbose logs to **stderr** and `main.go` suppresses the "Converting…/saved to…" lines for this format.

With several inputs, `renderConsole` renders each document **independently** through `renderConsoleDocument` and then concatenates them. Independence matters: Mermaid placeholder tokens are per-document, so rendering as one stream would let tokens collide. Two things are shared or aggregated instead — the Mermaid plan is resolved once (it depends on the terminal, not the document), and "did any diagram become an image" is OR'"'"'d across all documents, since a single image anywhere forces the pager to be skipped. The separator from `consoleSeparator` is glamour'"'"'s own horizontal rule, rendered by passing `---` through the same style and width, which keeps it in step with the theme for free instead of hand-drawing a rule per style.

**input.go** reads documents: `readInput` takes the bytes from a file or, for `StdinPath` (`-`), from the `Converter.stdin` seam, and treats whitespace-only input as **empty and an error for stdin only** — an empty pipe is invisible and nearly always an upstream failure, whereas an empty file is something the caller pointed at directly, and erroring on it would break the "single input behaves as before" regression guard. `inputDir` returns the directory relative paths resolve against: the file's own directory, or the **working directory** for stdin.

**converter.go** orchestrates all three pipelines and manages a temporary working directory for intermediate files. `Convert` takes a **slice** of inputs; only console format renders more than one. **Config** struct holds all runtime options including `InputFiles` (a slice; `-` means stdin), `Format` ("pdf"|"docx"|"console"), `DOCXFont`, and the console options (`ConsoleWidth`, `ConsoleStyle`, `ConsolePager`, `MermaidRender`).

**cmd/md2pdf/** — CLI entry point. `inputs.go` owns the input contract: `resolveInputs` validates **every** positional argument before any rendering starts (so a typo in the third of three paths fails immediately, naming it), rejects more than one input for non-console formats, refuses `-` mixed with file paths, and fails fast via `stdinUsable` when `-` is given but standard input is a terminal — reading a terminal would block with no indication of what md2pdf is waiting for. `resolveOutputPath` makes `-o` mandatory for `pdf`/`docx` read from stdin, since there is no input filename to derive one from. `flags.go` handles argument parsing and auto-detection of font/mmdc paths; `resolveFormat` derives the output format from `-format` or the `-o` extension, folds the `term`/`terminal` aliases onto `console`, and rejects `-o` combined with console. Console `-style` values are validated up front by `converter.ValidateConsoleStyle`, and `-mermaid-render` by `converter.ValidateMermaidRenderMode`. Pandoc is resolved in `docx.go` (`findPandoc`) unless `-pandoc` is provided. `main.go` wires flags to the converter.

## External Dependencies

Runtime: `mmdc` (Mermaid CLI via npm), Python 3 + Playwright + Chromium, Noto Sans CJK JP fonts. DOCX output additionally requires `pandoc`. Console output needs **no external tools** (a pager is used when available).
Go modules: `github.com/yuin/goldmark` (Markdown parsing), `charm.land/glamour/v2` (terminal rendering), `charm.land/lipgloss/v2` (terminal background detection), `github.com/charmbracelet/colorprofile` (color downsampling), `github.com/charmbracelet/x/term` (TTY detection and size), `github.com/charmbracelet/x/ansi` (inline image escape sequences: `KittyGraphics`, `ITerm2`, `SixelGraphics` and the `kitty`/`iterm2`/`sixel` subpackages), `github.com/AlexanderGrooff/mermaid-ascii` (Mermaid → text art; pinned to a pseudo-version, as it publishes no tags).

Only the *image* step of the console Mermaid chain needs `mmdc`. Its absence downgrades diagrams to text art, which is rendered in-process — console output never requires an external tool and never fails the run over a missing one.

## Code Style

- All exported symbols require GoDoc comments ending with a period (godot linter)
- Comments and GoDoc in English
- Errors crossing package boundaries must be wrapped (wrapcheck)
- Go 1.26 (see `go.mod`); CI tests against Go 1.26
- Tests that need the external toolchain go behind the `integration` build tag (see `converter_integration_test.go`), never behind a naming convention. `go test ./...` must pass with nothing installed, so anything left untagged has to be self-contained or use a stub binary. `ci_workflow_test.go` fails the build if CI goes back to selecting tests by name
