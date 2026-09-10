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
# Every test in the module. Nothing is filtered or tagged out.
go test ./...

# The integration test skips itself unless mmdc, a Chromium and fonts-noto-cjk
# are installed. To make a missing tool fail instead of skip
# (this is what CI does), declare the toolchain mandatory:
MD2PDF_REQUIRE_INTEGRATION=1 go test ./... -timeout 120s

# Single test
go test ./internal/converter/ -run TestSpecificName -v
```

## Linting

Uses golangci-lint with config in `.golangci.yml`. Key enabled linters: errcheck, gosimple, govet, staticcheck, unused, gofmt, goimports, misspell, godot, gosec, noctx, wrapcheck, exhaustive. G204 (subprocess with variable) is excluded since the mmdc and pandoc invocations are intentional. Test files have relaxed rules (no wrapcheck, gosec, errcheck).

## Architecture

`converter.go`'s `Convert` branches on `Config.Format` (`FormatPDF` / `FormatHTML` / `FormatDOCX` / `FormatConsole`): each format takes a **different pipeline** because each reads best from a different source.

### PDF pipeline (Markdown → HTML → Chromium), and HTML output

1. **parser.go** — goldmark parses Markdown to HTML, extracting fenced Mermaid code blocks into a `parsedDoc` struct with placeholders
2. **mermaid.go** — each Mermaid block is rendered to inline SVG via the external `mmdc` CLI (`renderMermaid`)
3. **html.go** — assembles a self-contained HTML file with GitHub CSS, `@font-face` declarations, and inlined SVGs
4. **pdf.go** — a headless Chromium prints the HTML to PDF, driven **directly over the DevTools Protocol** with chromedp. There is no Python stage: `resolvePrintOptions` converts `-page-size` and the `-margin-*` flags into the inches `Page.printToPDF` requires (`paper.go`), and `renderPDF` navigates to the `file://` URL, awaits `document.fonts.ready` — a Promise, so `WithAwaitPromise` is required, and printing before it settles lays the page out with fallback font metrics — then prints. The browser comes from `ChromiumPath`, the same resolution handed to `mmdc` via the generated Puppeteer config, so one Chromium serves both stages and `CHROME_PATH` overrides both

`FormatHTML` is the same pipeline stopping after step 3: `Convert` points `buildHTML` at the caller's output path instead of the working directory and returns, so Chromium never runs. Image paths are deliberately **not** rewritten or copied — the default output sits beside the input, where the Markdown's own relative paths already resolve.

`-css` stylesheets are read by `buildCSS`/`readCustomCSS` (`html.go`) and appended **after** `baseCSS` in the same inline `<style>` block, so user rules win the cascade; multiple `-css` files concatenate in argument order. A stylesheet containing `</style` is **rejected** rather than escaped: the page is assembled with `text/template`, which does no escaping, so the sequence would otherwise close the block and inject markup.

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

The fallback chain proper (`image → ascii → source`) is `renderConsoleBlock`, applied per diagram, so one unrenderable block never affects the others. **mermaid_ascii.go** owns the `ascii` step: `asciiDiagramSupported` gates on an explicit allowlist (`flowchart`, `graph`, `sequenceDiagram`) rather than trusting the library to reject a type — mermaid-ascii's graph parser is the fallback for any unrecognized header, and its `erDiagram` output is misaligned, so both are excluded here. `renderMermaidArt` then calls `mermaid-ascii`'s `render.RenderDiagram` behind a `recover()`, because a layout panic on a fallback path must degrade rather than kill the run. `renderMermaidASCII` wraps that with the **fit-to-width** loop in **mermaid_ascii_fit.go**: art wider than the wrap width has its node labels re-wrapped with `<br>` and is re-drawn, trying `labelFitCaps` widest-first so the least-wrapped art that fits is the one kept. Wrapping labels is the only lever there is — mermaid-ascii sizes each box from its label and lays independent chains side by side, so a wide diagram is wide because its labels are, and the library's own `MaxWidth` compacts padding once and never reflows. The loop cannot fail: a rewrite the renderer rejects and a diagram too wide even at the narrowest cap both fall back to art that draws, and `clippedArtWarning` reports what is still going to be clipped. `fitMermaidLabels` rewrites only the text between the shape delimiters `parseNode` recognizes, skips comments/`classDef`/`style`/`subgraph` lines and YAML frontmatter, and leaves a label holding another delimiter alone, since the renderer parses such a label differently and rewriting it would change the diagram rather than narrow it. `>...]` is excluded because a `>` cannot be told apart from the one ending an `-->`, and only `flowchart`/`graph` are fitted at all — a `sequenceDiagram` has no bracketed node labels, so a rewrite there could only corrupt it
5. Output goes through `$PAGER` (default `less -R -F`, with `-R` injected when the user's `less` lacks it) when writing to a TTY and `-pager` is on; colors are downsampled by `colorprofile` to what the terminal supports. **The pager is skipped whenever inline images were drawn**, since kitty and iTerm2 sequences do not survive `less`

Two details in the console Mermaid path are load-bearing:

- The token left for glamour is short (`MD2PDFDG<n>`, not the long `mermaidPlaceholderPrefix` the DOCX path uses) because glamour **hard-wraps a word wider than the wrap width**, which would split the token and lose the diagram. `consoleTokenFits` bails out to source output when even the short token cannot fit an explicit `-width`.
- `Converter.rasterizeMermaid` and `Converter.mermaidAvailable` are struct fields so tests can stand in for `mmdc`; the repo's own CI is the only place the real binary runs.
- Clipping text art to the wrap width happens in `spliceConsoleDiagrams`, not in `renderMermaidASCII`, because only the splice knows how far glamour indented the diagram. `consoleDiagram.clippable` marks which content is safe to trim — text art is, an image escape sequence is not. The splice gives up the **indent** before it gives up columns: `fitDiagramIndent` trims the indent glamour placed the diagram at down to what the diagram can spare, so art fitted to the full wrap width hangs into the left margin instead of losing its right edge. Art that fits with the indent intact keeps it. Measuring is done with `mermaidArtWidth` rather than the width mermaid-ascii reports, because the library's `displayWidth` goes through go-runewidth, which counts a box-drawing `─` as two columns under an East Asian locale and so overstates the art by the number of horizontal borders in it (the reported diagram measures 152 columns here and 301 there).
- `consoleWantsPureASCII` reads the **`-style` flag and `GLAMOUR_STYLE`**, not just the resolved style, because `resolveConsoleStyle` collapses redirected output to `notty` and would otherwise throw away an explicit `-style ascii`.
- Whether to skip the pager is decided by `anyImageDiagram(diagrams)`, **not** by `plan.emitsImages()`. An image plan still yields text art when `mmdc` is missing, and text art must keep paging.
- `mermaidDiagramHeader` strips YAML frontmatter with the renderer's own `diagram.StripFrontmatter`, or a diagram with a `---` title block would be rejected as an unsupported type before the renderer (which strips it) ever ran.

Because console format writes the document to stdout, everything the converter reports goes to **stderr** through `Converter.errOut` (nil `stderr` means `os.Stderr`; tests set it to read the output back), and `main.go` suppresses the "Converting…/saved to…" lines for this format. There are two channels: `logf` is progress and only speaks under `-v`, while `warnf` speaks **always**. `warnf` exists for the one thing a verbose log cannot carry — output the user asked for that did not survive. A diagram still clipped after fitting is reported through it, because a reader who does not pass `-v` must still be told a right edge went missing; routing it through `logf` would leave the loss as silent as the clipping fitting replaced.

With several inputs, `renderConsole` renders each document **independently** through `renderConsoleDocument` and then concatenates them. Independence matters: Mermaid placeholder tokens are per-document, so rendering as one stream would let tokens collide. Two things are shared or aggregated instead — the Mermaid plan is resolved once (it depends on the terminal, not the document), and "did any diagram become an image" is OR'"'"'d across all documents, since a single image anywhere forces the pager to be skipped. The separator from `consoleSeparator` is glamour'"'"'s own horizontal rule, rendered by passing `---` through the same style and width, which keeps it in step with the theme for free instead of hand-drawing a rule per style.

**input.go** reads documents: `readInput` takes the bytes from a file or, for `StdinPath` (`-`), from the `Converter.stdin` seam, and treats whitespace-only input as **empty and an error for stdin only** — an empty pipe is invisible and nearly always an upstream failure, whereas an empty file is something the caller pointed at directly, and erroring on it would break the "single input behaves as before" regression guard. `inputDir` returns the directory relative paths resolve against: the file's own directory, or the **working directory** for stdin.

**converter.go** orchestrates all three pipelines and manages a temporary working directory for intermediate files. `Convert` takes a **slice** of inputs; only console format renders more than one. **Config** struct holds all runtime options including `InputFiles` (a slice; `-` means stdin), `Format` ("pdf"|"html"|"docx"|"console"), `CSSFiles`, `DOCXFont`, and the console options (`ConsoleWidth`, `ConsoleStyle`, `ConsolePager`, `MermaidRender`).

**cmd/md2pdf/** — CLI entry point. `-doctor` (`doctor.go` + `converter.Diagnose` in `diagnose.go`) inspects the environment and reports which formats can run, exiting non-zero when one is blocked. It resolves every tool through the **same** functions the pipeline uses (`ChromiumPath`, the configured-or-PATH lookup for mmdc and pandoc, `resolveFontRegular`), so the report cannot claim a readiness conversion would not deliver. Font lookup lives in `cmd/md2pdf/fonts.go` and searches **directories** for any of several filenames, because Noto Sans CJK ships under different ones: `font-noto-sans-cjk` installs a single `NotoSansCJK.ttc` collection, `font-noto-sans-cjk-jp` installs per-weight `NotoSansCJKjp-*.otf`, and Debian packages `NotoSansCJK-Regular.ttc`. `~/Library/Fonts` is searched first, since that is where a macOS font cask installs — omitting it was why an installed font was reported missing, and why the PDF silently fell back to Hiragino. Finding the collection is only half of it: CSS has no syntax for a face **inside** a collection, so `url('…/NotoSansCJK.ttc')` hands back that file's first face — `Noto Sans CJK JP Thin`, one of 45 — whatever weight the `@font-face` rule declares, which rendered whole documents in hairlines with no bold. `localFaceNames` therefore names the wanted face (`local('Noto Sans CJK JP Bold')` and the PostScript spelling) **ahead of** the `url()` in `fontFace`'s `src` list, so the system font manager resolves the weight and the file stays as a fallback for a font the system does not know. Only a **weightless** collection needs this; `NotoSansCJK-Bold.ttc` is a collection too, but its first face is the weight it is named for. Dependencies are classified rather than lumped together: `pandoc` blocks docx unconditionally, a Chromium blocks pdf, `mmdc` is **conditional** — a document with no Mermaid blocks converts without it, verified against the binary — and the CJK font is **optional**, degrading output instead of preventing it. Only unconditional needs appear in `formatRequirements`. `inputs.go` owns the input contract: `resolveInputs` validates **every** positional argument before any rendering starts (so a typo in the third of three paths fails immediately, naming it), rejects more than one input for non-console formats, refuses `-` mixed with file paths, and fails fast via `stdinUsable` when `-` is given but standard input is a terminal — reading a terminal would block with no indication of what md2pdf is waiting for. `resolveOutputPath` makes `-o` mandatory for `pdf`/`docx` read from stdin, since there is no input filename to derive one from. `flags.go` handles argument parsing and auto-detection of font/mmdc paths; `resolveFormat` derives the output format from `-format` or the `-o` extension (`.html`/`.htm` imply html), `-css` is a repeatable `flag.Value` (`cssFlag`) whose files are stat-checked at parse time so a typo fails before any rendering, folds the `term`/`terminal` aliases onto `console`, and rejects `-o` combined with console. Console `-style` values are validated up front by `converter.ValidateConsoleStyle`, and `-mermaid-render` by `converter.ValidateMermaidRenderMode`. Pandoc is resolved in `docx.go` (`findPandoc`) unless `-pandoc` is provided. `main.go` wires flags to the converter.

## External Dependencies

Runtime: `mmdc` (Mermaid CLI via npm), a Chromium or Chrome binary, Noto Sans CJK JP fonts. DOCX output additionally requires `pandoc`. Console output needs **no external tools** (a pager is used when available).
Go modules: `github.com/yuin/goldmark` (Markdown parsing), `charm.land/glamour/v2` (terminal rendering), `charm.land/lipgloss/v2` (terminal background detection), `github.com/charmbracelet/colorprofile` (color downsampling), `github.com/charmbracelet/x/term` (TTY detection and size), `github.com/charmbracelet/x/ansi` (inline image escape sequences: `KittyGraphics`, `ITerm2`, `SixelGraphics` and the `kitty`/`iterm2`/`sixel` subpackages), `github.com/AlexanderGrooff/mermaid-ascii` (Mermaid → text art; pinned to a pseudo-version, as it publishes no tags).

Only the *image* step of the console Mermaid chain needs `mmdc`. Its absence downgrades diagrams to text art, which is rendered in-process — console output never requires an external tool and never fails the run over a missing one.

## Code Style

- All exported symbols require GoDoc comments ending with a period (godot linter)
- Comments and GoDoc in English
- Errors crossing package boundaries must be wrapped (wrapcheck)
- Go 1.26 (see `go.mod`); CI tests against Go 1.26
- **Nothing excludes tests from a run** — no `-run` filter, no build tags. A test needing the external toolchain checks for it and skips (see `requireTool` in `converter_integration_test.go`); everything else must pass with nothing installed, using a stub binary where a tool is involved. Both CI jobs run the identical `go test ./... -v`; the integration job differs only in having the tools installed and setting `MD2PDF_REQUIRE_INTEGRATION=1`, which turns a missing tool into a failure so a broken install cannot pass silently
