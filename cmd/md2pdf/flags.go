package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/135yshr/md2pdf/internal/converter"
)

// defaultFontPaths lists common locations for Noto Sans CJK JP fonts,
// searched in order when no explicit -font flag is provided.
var defaultFontPaths = []string{
	// Linux (Debian/Ubuntu)
	"/usr/share/fonts/opentype/noto/NotoSansCJK-Regular.ttc",
	"/usr/share/fonts/truetype/noto/NotoSansCJK-Regular.ttc",
	// macOS (Homebrew)
	"/opt/homebrew/share/fonts/noto-cjk/NotoSansCJK-Regular.ttc",
	"/usr/local/share/fonts/noto/NotoSansCJK-Regular.ttc",
	// Fallback: no custom font (system default)
	"",
}

// mmdcDefaultPaths lists common install locations for the Mermaid CLI (mmdc).
var mmdcDefaultPaths = []string{
	"mmdc", // found in $PATH
	"/usr/local/bin/mmdc",
	"/usr/bin/mmdc",
	// npm global installs (Linux/macOS)
	"/home/claude/.npm-global/bin/mmdc",
	"/usr/local/lib/node_modules/.bin/mmdc",
	"/opt/homebrew/bin/mmdc",
}

// parseFlags parses command-line arguments and returns a Config.
func parseFlags(args []string) (*converter.Config, error) {
	fs := flag.NewFlagSet("md2pdf", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)

	output := fs.String("o", "", "Output file path (default: <input>.pdf, or .docx with -format docx)")
	format := fs.String("format", "", "Output format: pdf (default), docx or console (inferred from -o extension when omitted)")
	fontRegular := fs.String("font", "", "Path to Noto Sans CJK JP Regular .ttc/.ttf font file")
	fontBold := fs.String("font-bold", "", "Path to Noto Sans CJK JP Bold .ttc/.ttf font file")
	fontMedium := fs.String("font-medium", "", "Path to Noto Sans CJK JP Medium .ttc/.ttf font file")
	mmdcPath := fs.String("mmdc", "", "Path to mmdc binary (Mermaid CLI)")
	pandocPath := fs.String("pandoc", "", "Path to pandoc binary (used for -format docx)")
	docxFont := fs.String("docx-font", "", "Font family for DOCX output (default: Yu Gothic)")
	pythonPath := fs.String("python", "", "Path to Python 3 interpreter with the playwright package (overrides MD2PDF_PYTHON)")
	puppeteerCfg := fs.String("puppeteer-config", "", "Path to Puppeteer JSON config file for mmdc (auto-created if omitted)")
	pageSize := fs.String("page-size", "A4", "PDF page size: A4, Letter, A3")
	consoleWidth := fs.Int("width", 0, "Console word-wrap width in columns (0: follow terminal, max 120)")
	consoleStyle := fs.String("style", "", "Console color theme: auto (default), dark, light, notty, ... or a JSON stylesheet path")
	consolePager := fs.Bool("pager", true, "Send console output through $PAGER (default: less -R -F) on a terminal")
	mermaidRender := fs.String("mermaid-render", "", "Console Mermaid rendering: auto (default), image, ascii or source")
	marginTop := fs.String("margin-top", "18mm", "Top margin (e.g. 18mm, 1in)")
	marginBottom := fs.String("margin-bottom", "18mm", "Bottom margin")
	marginLeft := fs.String("margin-left", "14mm", "Left margin")
	marginRight := fs.String("margin-right", "14mm", "Right margin")
	verbose := fs.Bool("v", false, "Verbose output")
	showVersion := fs.Bool("version", false, "Print version and exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if *showVersion {
		fmt.Printf("md2pdf version %s (commit: %s, built: %s)\n", version, commit, date)
		os.Exit(0)
	}

	if fs.NArg() != 1 {
		return nil, errors.New("exactly one input Markdown file is required")
	}

	input := fs.Arg(0)
	if _, err := os.Stat(input); err != nil {
		return nil, fmt.Errorf("input file not found: %s", input)
	}
	if !strings.EqualFold(filepath.Ext(input), ".md") {
		return nil, fmt.Errorf("input file must have a .md extension: %s", input)
	}

	// Resolve output format. An explicit -format flag wins; otherwise it is
	// inferred from the -o extension, defaulting to pdf.
	outFormat, err := resolveFormat(*format, *output)
	if err != nil {
		return nil, err
	}

	// Resolve output path, defaulting the extension to the chosen format.
	// Console format writes to standard output and needs no path.
	out := *output
	if out == "" && outFormat != converter.FormatConsole {
		base := strings.TrimSuffix(input, filepath.Ext(input))
		out = base + "." + outFormat
	}

	// Validate the console-only options up front so a bad value fails before
	// any rendering work starts.
	if outFormat == converter.FormatConsole {
		if *consoleWidth < 0 {
			return nil, fmt.Errorf("-width must be zero or positive, got %d", *consoleWidth)
		}
		if err := converter.ValidateConsoleStyle(*consoleStyle); err != nil {
			return nil, err
		}
		if err := converter.ValidateMermaidRenderMode(*mermaidRender); err != nil {
			return nil, err
		}
	}

	// Resolve font paths.
	regular := *fontRegular
	if regular == "" {
		regular = findFirst(defaultFontPaths)
	}
	bold := *fontBold
	if bold == "" {
		// Derive bold from regular path by replacing "Regular" with "Bold".
		bold = strings.ReplaceAll(regular, "Regular", "Bold")
		if _, err := os.Stat(bold); err != nil {
			bold = regular // fallback: use regular weight
		}
	}
	medium := *fontMedium
	if medium == "" {
		medium = strings.ReplaceAll(regular, "Regular", "Medium")
		if _, err := os.Stat(medium); err != nil {
			medium = regular
		}
	}

	// Resolve mmdc path.
	mmdc := *mmdcPath
	if mmdc == "" {
		mmdc = findFirst(mmdcDefaultPaths)
	}

	// Resolve Python interpreter. The flag wins; otherwise MD2PDF_PYTHON.
	// An empty result triggers auto-detection inside the converter.
	python := *pythonPath
	if python == "" {
		python = os.Getenv("MD2PDF_PYTHON")
	}

	return &converter.Config{
		InputFile:       input,
		OutputFile:      out,
		Format:          outFormat,
		FontRegular:     regular,
		FontBold:        bold,
		FontMedium:      medium,
		MmdcPath:        mmdc,
		PandocPath:      *pandocPath,
		DOCXFont:        *docxFont,
		PythonPath:      python,
		PuppeteerConfig: *puppeteerCfg,
		PageSize:        *pageSize,
		MarginTop:       *marginTop,
		MarginBottom:    *marginBottom,
		MarginLeft:      *marginLeft,
		MarginRight:     *marginRight,
		ConsoleWidth:    *consoleWidth,
		ConsoleStyle:    *consoleStyle,
		ConsolePager:    *consolePager,
		MermaidRender:   *mermaidRender,
		Verbose:         *verbose,
	}, nil
}

// resolveFormat determines the output format from the explicit -format flag and
// the -o path. The flag takes precedence; otherwise the format is inferred from
// the output file extension, defaulting to pdf. It returns an error when the two
// disagree or when an unsupported format is requested. Console format renders to
// the terminal, so it cannot be combined with -o.
func resolveFormat(format, output string) (string, error) {
	fromExt := ""
	switch strings.ToLower(filepath.Ext(output)) {
	case ".pdf":
		fromExt = converter.FormatPDF
	case ".docx":
		fromExt = converter.FormatDOCX
	}

	if format == "" {
		if fromExt != "" {
			return fromExt, nil
		}
		return converter.FormatPDF, nil
	}

	normalized := normalizeFormat(format)
	switch normalized {
	case converter.FormatPDF, converter.FormatDOCX:
	case converter.FormatConsole:
		if output != "" {
			return "", errors.New("-format console renders to the terminal; remove -o")
		}
		return converter.FormatConsole, nil
	default:
		return "", fmt.Errorf("unsupported output format %q: must be pdf, docx or console", format)
	}

	if fromExt != "" && fromExt != normalized {
		return "", fmt.Errorf("output extension .%s conflicts with -format %s", fromExt, normalized)
	}
	return normalized, nil
}

// normalizeFormat lowercases a -format value and folds the terminal aliases
// onto the canonical console format name.
func normalizeFormat(format string) string {
	normalized := strings.ToLower(format)
	if normalized == "term" || normalized == "terminal" {
		return converter.FormatConsole
	}
	return normalized
}

// findFirst returns the first path from the list that exists on disk,
// or the first element if none exist (to preserve fallback semantics).
func findFirst(paths []string) string {
	for _, p := range paths {
		if p == "" {
			return p
		}
		if _, err := os.Stat(p); err == nil {
			return p
		}
	}
	return paths[0]
}

// printUsage prints a friendly usage summary to stderr.
func printUsage() {
	fmt.Fprintf(os.Stderr, `
Usage:
  md2pdf [options] <input.md>

Options:
  -o <path>               Output path (default: <input>.pdf, or .docx with -format docx)
  -format <fmt>           Output format: pdf (default), docx or console
                          (console has the aliases term and terminal;
                          inferred from -o extension when omitted)
  -font <path>            Noto Sans CJK JP Regular font (.ttc/.ttf)
  -font-bold <path>       Noto Sans CJK JP Bold font
  -font-medium <path>     Noto Sans CJK JP Medium font
  -mmdc <path>            Path to mmdc (Mermaid CLI) binary
  -pandoc <path>          Path to pandoc binary (used for -format docx)
  -docx-font <family>     Font family for DOCX output (default: Yu Gothic)
  -python <path>          Path to Python 3 interpreter with the playwright
                          package (env: MD2PDF_PYTHON)
  -puppeteer-config <f>   Path to Puppeteer JSON config for mmdc
  -page-size <size>       PDF page size: A4 (default), Letter, A3
  -margin-top <m>         Top margin    (default: 18mm)
  -margin-bottom <m>      Bottom margin (default: 18mm)
  -margin-left <m>        Left margin   (default: 14mm)
  -margin-right <m>       Right margin  (default: 14mm)
  -width <cols>           Console word-wrap width (default: terminal width,
                          capped at 120 columns)
  -style <name|path>      Console color theme: auto (default), dark, light,
                          notty, ascii, dracula, pink, tokyo-night, or a path
                          to a JSON stylesheet (env: GLAMOUR_STYLE)
  -pager                  Page console output through $PAGER on a terminal
                          (default: true; use -pager=false to disable)
  -mermaid-render <mode>  How to draw Mermaid diagrams in console output:
                          auto (default) tries an inline image on terminals
                          that support kitty, iTerm2 or Sixel, then falls back
                          to box-drawing text art, then to the source;
                          image forces inline images and fails if the terminal
                          cannot show them; ascii always draws text art;
                          source always prints the Mermaid source.
                          Inline images bypass the pager, which cannot display
                          them; text art pages normally.
  -v                      Verbose output
  -version                Print version and exit

Examples:
  md2pdf document.md
  md2pdf -o report.pdf document.md
  md2pdf -format docx document.md
  md2pdf -o report.docx document.md
  md2pdf -format console document.md
  md2pdf -format console -style dark -width 100 document.md
  md2pdf -format console -pager=false document.md | cat
  md2pdf -format console -mermaid-render image document.md
  md2pdf -format console -mermaid-render ascii document.md
  md2pdf -format console -mermaid-render source document.md
  md2pdf -font /path/to/NotoSansCJK-Regular.ttc document.md
`)
}
