// Package converter orchestrates the Markdown → HTML → PDF pipeline.
//
// The pipeline consists of three stages:
//  1. Parse Markdown and extract fenced Mermaid code blocks.
//  2. Render each Mermaid block to an SVG file using the mmdc CLI.
//  3. Build a self-contained GitHub-styled HTML file and print it to PDF
//     using a headless Chromium browser (via the Playwright Python driver).
package converter

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Config holds all runtime options for the converter.
type Config struct {
	// InputFile is the path to the source Markdown file.
	InputFile string
	// OutputFile is the destination PDF path.
	OutputFile string
	// FontRegular is the file path to the Noto Sans CJK JP Regular font.
	FontRegular string
	// FontBold is the file path to the Noto Sans CJK JP Bold font.
	FontBold string
	// FontMedium is the file path to the Noto Sans CJK JP Medium font.
	FontMedium string
	// MmdcPath is the path to the mmdc (Mermaid CLI) binary.
	MmdcPath string
	// PuppeteerConfig is an optional path to a Puppeteer JSON config file
	// passed to mmdc via its -p flag. When empty the converter auto-generates
	// a temporary config pointing at the system Chromium.
	PuppeteerConfig string
	// PageSize controls the PDF paper size (A4, Letter, A3).
	PageSize string
	// MarginTop, MarginBottom, MarginLeft, MarginRight set PDF page margins.
	MarginTop    string
	MarginBottom string
	MarginLeft   string
	MarginRight  string
	// Verbose enables detailed progress logging.
	Verbose bool
}

// Converter manages the conversion lifecycle including temporary file cleanup.
type Converter struct {
	cfg     *Config
	workDir string // temporary directory for intermediate files
}

// New creates a new Converter and prepares a temporary working directory.
func New(cfg *Config) (*Converter, error) {
	work, err := os.MkdirTemp("", "md2pdf-*")
	if err != nil {
		return nil, fmt.Errorf("create temp dir: %w", err)
	}
	return &Converter{cfg: cfg, workDir: work}, nil
}

// Close removes the temporary working directory and all intermediate files.
func (c *Converter) Close() {
	_ = os.RemoveAll(c.workDir)
}

// Convert runs the full Markdown → PDF pipeline for the given input file,
// writing the result to outputPath.
func (c *Converter) Convert(inputPath, outputPath string) error {
	mdBytes, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
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

	c.logf("Building HTML...")
	htmlPath := filepath.Join(c.workDir, "document.html")
	if err := c.buildHTML(doc, htmlPath); err != nil {
		return fmt.Errorf("build html: %w", err)
	}

	c.logf("Copying images to working directory...")
	srcDir, err := filepath.Abs(filepath.Dir(inputPath))
	if err != nil {
		return fmt.Errorf("resolve input dir: %w", err)
	}
	if err := c.copyImages(doc.HTML, srcDir); err != nil {
		return fmt.Errorf("copy images: %w", err)
	}

	c.logf("Printing PDF with headless Chromium...")
	absOut, err := filepath.Abs(outputPath)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	if err := c.printPDF(htmlPath, absOut); err != nil {
		return fmt.Errorf("print pdf: %w", err)
	}

	return nil
}

// logf prints a formatted message when verbose mode is enabled.
func (c *Converter) logf(format string, args ...any) {
	if c.cfg.Verbose {
		fmt.Printf("  "+format+"\n", args...)
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

// chromiumPath attempts to locate the system Chromium executable.
// It checks common Linux paths and falls back to whatever Playwright ships.
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
