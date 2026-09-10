package cli

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/135yshr/md2pdf/internal/converter"
)

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

// cssFlag collects repeated -css occurrences, preserving the order they were
// given so the cascade is predictable.
type cssFlag []string

// String implements flag.Value.
func (f *cssFlag) String() string { return strings.Join(*f, ", ") }

// Set implements flag.Value, appending rather than replacing so -css can be
// passed more than once.
func (f *cssFlag) Set(value string) error {
	*f = append(*f, value)
	return nil
}

// parseFlags parses command-line arguments and returns a Config. The -version
// and -doctor flags print their own output and finish the invocation here; they
// report that as an exitError rather than exiting the process, so the caller
// keeps ownership of the exit status.
func (p *program) parseFlags(args []string) (*converter.Config, error) {
	fs := flag.NewFlagSet(p.name, flag.ContinueOnError)
	fs.SetOutput(p.stderr)

	output := fs.String("o", "", "Output file path (default: <input>.pdf, or .docx with -format docx)")
	format := fs.String("format", "", "Output format: pdf (default), html, docx or console (inferred from -o extension when omitted)")
	fontRegular := fs.String("font", "", "Path to Noto Sans CJK JP Regular .ttc/.ttf font file")
	fontBold := fs.String("font-bold", "", "Path to Noto Sans CJK JP Bold .ttc/.ttf font file")
	fontMedium := fs.String("font-medium", "", "Path to Noto Sans CJK JP Medium .ttc/.ttf font file")
	mmdcPath := fs.String("mmdc", "", "Path to mmdc binary (Mermaid CLI)")
	pandocPath := fs.String("pandoc", "", "Path to pandoc binary (used for -format docx)")
	docxFont := fs.String("docx-font", "", "Font family for DOCX output (default: Yu Gothic)")
	puppeteerCfg := fs.String("puppeteer-config", "", "Path to Puppeteer JSON config file for mmdc (auto-created if omitted)")
	var cssFiles cssFlag
	fs.Var(&cssFiles, "css", "Path to a custom CSS file, applied after the built-in stylesheet (repeatable)")
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
	doctor := fs.Bool("doctor", false, "Report which runtime dependencies are present and which formats can run, then exit")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	if *showVersion {
		fmt.Fprintf(p.stdout, "%s version %s (commit: %s, built: %s)\n",
			p.name, p.build.Version, p.build.Commit, p.build.Date)
		return nil, exitError{code: 0}
	}

	// -doctor inspects the machine rather than converting anything, so it runs
	// before the input arguments are required. Font resolution still happens
	// below, because the report needs the path that a conversion would use.
	if *doctor {
		cfg := &converter.Config{
			MmdcPath:    resolveMmdcPath(*mmdcPath),
			PandocPath:  *pandocPath,
			FontRegular: resolveFontRegular(*fontRegular),
		}
		if runDoctor(p.stdout, p.diagnose(cfg)) {
			return nil, exitError{code: 0}
		}
		return nil, exitError{code: 1}
	}

	// Resolve output format first: it decides whether several inputs are
	// allowed at all. An explicit -format flag wins; otherwise it is inferred
	// from the -o extension, defaulting to pdf.
	outFormat, err := resolveFormat(*format, *output)
	if err != nil {
		return nil, err
	}

	inputs, err := p.resolveInputs(fs.Args(), outFormat)
	if err != nil {
		return nil, err
	}

	// Resolve output path, defaulting the extension to the chosen format.
	// Console format writes to standard output and needs no path.
	out, err := resolveOutputPath(*output, inputs, outFormat)
	if err != nil {
		return nil, err
	}

	// Fail on an unreadable stylesheet before any rendering work starts, so a
	// typo in -css is reported immediately and by name.
	for _, path := range cssFiles {
		info, err := os.Stat(path)
		if err != nil {
			return nil, fmt.Errorf("custom CSS file not found: %s", path)
		}
		if info.IsDir() {
			return nil, fmt.Errorf("custom CSS path is a directory: %s", path)
		}
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
	regular := resolveFontRegular(*fontRegular)
	bold := *fontBold
	if bold == "" {
		bold = deriveFontWeight(regular, "Bold")
	}
	medium := *fontMedium
	if medium == "" {
		medium = deriveFontWeight(regular, "Medium")
	}

	// Resolve mmdc path.
	mmdc := resolveMmdcPath(*mmdcPath)

	return &converter.Config{
		InputFiles:       inputs,
		OutputFile:       out,
		Format:           outFormat,
		FontRegular:      regular,
		FontBold:         bold,
		FontMedium:       medium,
		FontLocalRegular: localFaceNames(regular, "Regular"),
		FontLocalBold:    localFaceNames(bold, "Bold"),
		FontLocalMedium:  localFaceNames(medium, "Medium"),
		MmdcPath:         mmdc,
		PandocPath:       *pandocPath,
		DOCXFont:         *docxFont,
		PuppeteerConfig:  *puppeteerCfg,
		CSSFiles:         cssFiles,
		PageSize:         *pageSize,
		MarginTop:        *marginTop,
		MarginBottom:     *marginBottom,
		MarginLeft:       *marginLeft,
		MarginRight:      *marginRight,
		ConsoleWidth:     *consoleWidth,
		ConsoleStyle:     *consoleStyle,
		ConsolePager:     *consolePager,
		MermaidRender:    *mermaidRender,
		Verbose:          *verbose,
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
	case ".html", ".htm":
		fromExt = converter.FormatHTML
	}

	if format == "" {
		if fromExt != "" {
			return fromExt, nil
		}
		return converter.FormatPDF, nil
	}

	normalized := normalizeFormat(format)
	switch normalized {
	case converter.FormatPDF, converter.FormatDOCX, converter.FormatHTML:
	case converter.FormatConsole:
		if output != "" {
			return "", errors.New("-format console renders to the terminal; remove -o")
		}
		return converter.FormatConsole, nil
	default:
		return "", fmt.Errorf("unsupported output format %q: must be pdf, html, docx or console", format)
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

// resolveFontRegular returns the regular-weight font to use: the -font flag, or
// the first Noto Sans CJK file found in the well-known font directories. Shared
// with -doctor so the report names the same font a conversion would load.
func resolveFontRegular(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return findCJKFont(fontSearchDirs(userHomeDir()))
}

// resolveMmdcPath returns the mmdc binary to use: the -mmdc flag, or the first
// of the well-known locations that exists.
func resolveMmdcPath(explicit string) string {
	if explicit != "" {
		return explicit
	}
	return findFirst(mmdcDefaultPaths)
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
