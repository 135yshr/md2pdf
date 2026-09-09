// Package converter turns Markdown into PDF, DOCX, or styled terminal output.
//
// The PDF pipeline consists of three stages:
//  1. Parse Markdown and extract fenced Mermaid code blocks.
//  2. Render each Mermaid block to an SVG file using the mmdc CLI.
//  3. Build a self-contained GitHub-styled HTML file and print it to PDF
//     using a headless Chromium browser (via the Playwright Python driver).
//
// DOCX output is produced directly from Markdown by pandoc, and console output
// is rendered to ANSI text by glamour; both bypass the HTML stage.
package converter

import (
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Supported output formats.
const (
	// FormatPDF prints the document to PDF with headless Chromium.
	FormatPDF = "pdf"
	// FormatDOCX converts the document to a Word document with pandoc.
	FormatDOCX = "docx"
	// FormatConsole renders the document as styled ANSI text for a terminal.
	FormatConsole = "console"
	// FormatHTML writes the self-contained HTML that PDF output is built from,
	// stopping before headless Chromium runs.
	FormatHTML = "html"
)

// Config holds all runtime options for the converter.
type Config struct {
	// InputFiles are the source Markdown paths, in the order given on the
	// command line. A single entry of StdinPath means the document is read from
	// standard input. Only console format accepts more than one entry.
	InputFiles []string
	// OutputFile is the destination output path (PDF or DOCX).
	OutputFile string
	// Format selects the output format: "pdf" (default), "docx" or "console".
	// An empty value is treated as "pdf".
	Format string
	// FontRegular is the file path to the Noto Sans CJK JP Regular font.
	FontRegular string
	// FontBold is the file path to the Noto Sans CJK JP Bold font.
	FontBold string
	// FontMedium is the file path to the Noto Sans CJK JP Medium font.
	FontMedium string
	// MmdcPath is the path to the mmdc (Mermaid CLI) binary.
	MmdcPath string
	// PandocPath is the path to the pandoc binary used for DOCX output.
	// When empty, md2pdf auto-detects pandoc on PATH and in common locations.
	PandocPath string
	// DOCXFont is the font family applied to DOCX output for both Latin and
	// East Asian text. When empty, a Japanese-friendly default is used.
	DOCXFont string
	// PuppeteerConfig is an optional path to a Puppeteer JSON config file
	// passed to mmdc via its -p flag. When empty the converter auto-generates
	// a temporary config pointing at the system Chromium.
	PuppeteerConfig string
	// CSSFiles are extra stylesheet paths injected after the built-in
	// GitHub-flavored CSS, in the order given, so later files win the cascade.
	CSSFiles []string
	// PageSize controls the PDF paper size (A4, Letter, A3).
	PageSize string
	// MarginTop, MarginBottom, MarginLeft, MarginRight set PDF page margins.
	MarginTop    string
	MarginBottom string
	MarginLeft   string
	MarginRight  string
	// ConsoleWidth overrides the word-wrap width of console output. Zero
	// follows the terminal width (capped at 120 columns) and falls back to 80
	// columns when the output is not a terminal.
	ConsoleWidth int
	// ConsoleStyle selects the console color theme: a built-in style name
	// ("auto", "dark", "light", "notty", ...) or a path to a JSON stylesheet.
	// An empty value is treated as "auto".
	ConsoleStyle string
	// ConsolePager sends console output through $PAGER (default: less -R -F)
	// when writing to an interactive terminal.
	ConsolePager bool
	// MermaidRender selects how Mermaid blocks are drawn in console output:
	// "auto" (default), "image" or "source".
	MermaidRender string
	// Verbose enables detailed progress logging.
	Verbose bool
}

// Converter manages the conversion lifecycle including temporary file cleanup.
type Converter struct {
	cfg     *Config
	workDir string // temporary directory for intermediate files
	// rasterizeMermaid renders one Mermaid block to a PNG and returns its
	// absolute path, and mermaidAvailable reports whether the renderer can run
	// at all. Both are fields so tests can stand in for the external mmdc
	// invocation without depending on what is installed on the machine.
	rasterizeMermaid func(idx int, source string) (string, error)
	mermaidAvailable func() bool
	// stdin is where StdinPath reads from. Nil means os.Stdin; tests set it to
	// supply a document without touching the process's standard input.
	stdin io.Reader
}

// New creates a new Converter and prepares a temporary working directory.
func New(cfg *Config) (*Converter, error) {
	work, err := os.MkdirTemp("", "md2pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	c := &Converter{cfg: cfg, workDir: work}
	c.rasterizeMermaid = c.consoleDiagramPNG
	c.mermaidAvailable = c.mmdcAvailable
	return c, nil
}

// Close removes the temporary working directory and all intermediate files.
func (c *Converter) Close() {
	_ = os.RemoveAll(c.workDir)
}

// Convert runs the conversion pipeline for the given inputs, writing the result
// to outputPath. PDF output flows through the Markdown → HTML → Chromium
// stages; HTML output stops after the HTML stage and emits that file directly;
// DOCX output is produced directly from Markdown by pandoc so the result uses
// clean, Word-native styling instead of HTML-derived markup. Console output is
// written to standard output instead of outputPath, which is ignored.
//
// Console format renders every input in order; the other formats take exactly
// one, which the CLI enforces before calling this.
func (c *Converter) Convert(inputs []string, outputPath string) error {
	if len(inputs) == 0 {
		return errors.New("no input documents")
	}

	if strings.EqualFold(c.cfg.Format, FormatConsole) {
		if err := c.renderConsole(inputs, os.Stdout); err != nil {
			return fmt.Errorf("render console: %w", err)
		}
		return nil
	}

	if len(inputs) > 1 {
		return fmt.Errorf("%s output takes a single input document, got %d", c.cfg.Format, len(inputs))
	}
	inputPath := inputs[0]

	mdBytes, err := c.readInput(inputPath)
	if err != nil {
		return err
	}

	srcDir, err := inputDir(inputPath)
	if err != nil {
		return err
	}

	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}

	if strings.EqualFold(c.cfg.Format, FormatDOCX) {
		c.logf("Converting Markdown to DOCX with pandoc...")
		if err := c.convertMarkdownDOCX(mdBytes, srcDir, absOut); err != nil {
			return fmt.Errorf("convert docx: %w", err)
		}
		return nil
	}

	c.logf("Parsing Markdown and extracting Mermaid blocks...")
	doc, err := parseMarkdown(mdBytes)
	if err != nil {
		return fmt.Errorf("parse markdown: %w", err)
	}

	c.logf("Rendering %d Mermaid diagram(s)...", len(doc.mermaidBlocks))
	if err := c.renderMermaid(doc); err != nil {
		return fmt.Errorf("render mermaid: %w", err)
	}

	// HTML output is the PDF pipeline's own intermediate, so it goes straight to
	// the caller's path and Chromium is never started. Image paths are left
	// exactly as the Markdown had them, which resolves correctly for the default
	// output location beside the input file.
	htmlOnly := strings.EqualFold(c.cfg.Format, FormatHTML)
	htmlPath := filepath.Join(c.workDir, "document.html")
	if htmlOnly {
		htmlPath = absOut
	}

	c.logf("Building HTML...")
	if err := c.buildHTML(doc, htmlPath); err != nil {
		return fmt.Errorf("build html: %w", err)
	}
	if htmlOnly {
		return nil
	}

	c.logf("Copying images to working directory...")
	if err := c.copyImages(doc.HTML, srcDir); err != nil {
		return fmt.Errorf("copy images: %w", err)
	}

	c.logf("Printing PDF with headless Chromium...")
	if err := c.printPDF(htmlPath, absOut); err != nil {
		return fmt.Errorf("print pdf: %w", err)
	}

	return nil
}

// logf prints a formatted message to standard error when verbose mode is
// enabled. Logging to stderr keeps progress output separate from the rendered
// document that console format writes to stdout.
func (c *Converter) logf(format string, args ...any) {
	if c.cfg.Verbose {
		fmt.Fprintf(os.Stderr, "  "+format+"\n", args...)
	}
}

// imgSrcRe matches src attributes in <img> tags.
var imgSrcRe = regexp.MustCompile(`<img\s[^>]*?\bsrc=["']([^"']+)["']`)

// copyImages scans rendered HTML for <img> tags with relative paths and copies
// the referenced files from srcDir into the working directory, preserving the
// relative directory structure so that the HTML can reference them as-is.
func (c *Converter) copyImages(html, srcDir string) error {
	matches := imgSrcRe.FindAllStringSubmatch(html, -1)
	for _, m := range matches {
		src := m[1]

		// Skip absolute URLs and data URIs.
		if strings.HasPrefix(src, "http://") || strings.HasPrefix(src, "https://") || strings.HasPrefix(src, "data:") {
			continue
		}

		// Skip diagrams md2pdf generated itself; they already live in the
		// working directory and have no counterpart in the source tree.
		if strings.HasPrefix(src, mermaidImageSubdir+"/") {
			continue
		}

		// Decode percent-encoded paths (e.g. spaces as %20).
		decoded, err := url.PathUnescape(src)
		if err != nil {
			decoded = src
		}

		// Skip absolute file paths.
		if filepath.IsAbs(decoded) {
			continue
		}

		origPath := filepath.Clean(filepath.Join(srcDir, decoded))
		destPath := filepath.Clean(filepath.Join(c.workDir, decoded))

		// Prevent path traversal outside the source or working directory.
		if !strings.HasPrefix(origPath+string(os.PathSeparator), srcDir+string(os.PathSeparator)) {
			c.logf("  warning: image path escapes source directory: %s", decoded)
			continue
		}
		if !strings.HasPrefix(destPath+string(os.PathSeparator), c.workDir+string(os.PathSeparator)) {
			c.logf("  warning: image path escapes work directory: %s", decoded)
			continue
		}

		if _, err := os.Stat(origPath); err != nil {
			c.logf("  warning: image not found: %s", origPath)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return fmt.Errorf("create image dir: %w", err)
		}
		if err := copyFile(origPath, destPath); err != nil {
			return fmt.Errorf("copy %s: %w", decoded, err)
		}
		c.logf("  copied image: %s", decoded)
	}
	return nil
}

// copyFile copies the file at src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}

// ChromiumPath locates the Chromium executable md2pdf drives, honouring
// CHROME_PATH and then probing the usual install locations. It is exported so
// callers can report on the environment without attempting a conversion.
func ChromiumPath() (string, error) {
	return chromiumPath()
}

// chromiumPath attempts to locate the system Chromium executable.
// It checks common install paths, including caches left by Playwright.
func chromiumPath() (string, error) {
	// Honour CHROME_PATH if set. Fail fast on invalid values.
	if p := os.Getenv("CHROME_PATH"); p != "" {
		info, err := os.Stat(p)
		if err != nil {
			return "", fmt.Errorf("CHROME_PATH is set but invalid: %w", err)
		}
		if info.IsDir() {
			return "", fmt.Errorf("CHROME_PATH points to a directory: %s", p)
		}
		if info.Mode()&0o111 == 0 {
			return "", fmt.Errorf("CHROME_PATH is not executable: %s", p)
		}
		return p, nil
	}

	candidates := []string{
		// Linux (CI / Playwright)
		"/opt/pw-browsers/chromium-1194/chrome-linux/chrome",
		"/usr/bin/chromium-browser",
		"/usr/bin/chromium",
		"/usr/bin/google-chrome",
	}

	// macOS: Playwright cache.
	if home, err := os.UserHomeDir(); err == nil {
		cacheDir := filepath.Join(home, "Library", "Caches", "ms-playwright")
		if entries, err := os.ReadDir(cacheDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() {
					continue
				}
				macBin := filepath.Join(cacheDir, e.Name(), "chrome-mac", "Chromium.app", "Contents", "MacOS", "Chromium")
				if _, err := os.Stat(macBin); err == nil {
					candidates = append([]string{macBin}, candidates...)
					break
				}
			}
		}
	}

	// macOS: common install locations.
	candidates = append(candidates,
		"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
		"/Applications/Chromium.app/Contents/MacOS/Chromium",
	)

	for _, p := range candidates {
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}

	// Last resort: ask the shell.
	for _, name := range []string{"chromium-browser", "chromium", "google-chrome"} {
		if out, err := exec.LookPath(name); err == nil {
			return out, nil
		}
	}
	return "", fmt.Errorf("no Chromium executable found; install chromium-browser or set CHROME_PATH")
}
