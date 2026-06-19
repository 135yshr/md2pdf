package converter

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

const pandocInstallHint = "install with: brew install pandoc (macOS) or apt-get install pandoc (Debian/Ubuntu)"

// defaultDOCXFont is the font family applied to DOCX output when none is
// configured. Yu Gothic (游ゴシック) ships with Japanese Windows and macOS and
// renders Japanese body text far more readably than pandoc's empty default.
const defaultDOCXFont = "Yu Gothic"

// docxBodyHalfPt is the default DOCX body font size in half-points
// (21 = 10.5pt), the conventional body size for Japanese business documents.
const docxBodyHalfPt = "21"

// docxHeadingHalfPt maps heading style IDs to compact font sizes in half-points,
// replacing pandoc's oversized defaults so headings sit closer to the body text.
var docxHeadingHalfPt = map[string]string{
	"Heading1": "32", "Heading1Char": "32",
	"Heading2": "26", "Heading2Char": "26",
	"Heading3": "24", "Heading3Char": "24",
}

// pandocDefaultPaths lists common install locations for the pandoc binary,
// tried in order when no explicit -pandoc flag is provided.
var pandocDefaultPaths = []string{
	"pandoc", // found in $PATH
	"/opt/homebrew/bin/pandoc",
	"/usr/local/bin/pandoc",
	"/usr/bin/pandoc",
}

// tableBorders is the OOXML fragment that adds single-line borders to every
// edge and inner gridline of the default Table style. It is inserted into the
// reference document so converted GFM tables render as a visible grid.
const tableBorders = `<w:tblBorders>` +
	`<w:top w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
	`<w:left w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
	`<w:bottom w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
	`<w:right w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
	`<w:insideH w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
	`<w:insideV w:val="single" w:sz="4" w:space="0" w:color="auto"/>` +
	`</w:tblBorders>`

// tableStyleRe matches the default "Table" style definition in styles.xml.
var tableStyleRe = regexp.MustCompile(`(?s)<w:style [^>]*w:styleId="Table".*?</w:style>`)

// convertMarkdownDOCX converts Markdown bytes to a DOCX file using pandoc.
//
// DOCX is produced directly from Markdown (pandoc's gfm reader) rather than from
// the intermediate HTML used for PDF, so pandoc emits clean, Word-native
// paragraph and list styles instead of HTML-derived ones. Fenced Mermaid blocks
// are rendered to PNG and spliced back in as image references. A styled
// reference document (Japanese-friendly font, compact headings, bordered tables)
// is generated and applied when possible, falling back to pandoc's defaults.
//
// pandoc runs with its working directory set to the converter's temporary
// directory so generated diagrams resolve there; user images are resolved
// against srcDir via --resource-path.
func (c *Converter) convertMarkdownDOCX(mdBytes []byte, srcDir, docxPath string) error {
	pandoc, err := c.findPandoc()
	if err != nil {
		return err
	}

	markdown, blocks := extractMermaidFromMarkdown(string(mdBytes))
	c.logf("Rendering %d Mermaid diagram(s)...", len(blocks))
	if err := c.renderMermaidPNGs(blocks); err != nil {
		return fmt.Errorf("render mermaid: %w", err)
	}
	for _, b := range blocks {
		markdown = strings.Replace(markdown, b.Placeholder, fmt.Sprintf("![](%s)", b.ImagePath), 1)
	}

	mdPath := filepath.Join(c.workDir, "document.md")
	if err := os.WriteFile(mdPath, []byte(markdown), 0o644); err != nil {
		return fmt.Errorf("write markdown: %w", err)
	}

	// Resolve generated diagrams against the working directory (cmd.Dir) and
	// user images against the source directory.
	resourcePath := c.workDir + string(os.PathListSeparator) + srcDir
	args := []string{mdPath, "-f", "gfm", "-o", docxPath, "--resource-path", resourcePath}

	// Best-effort: a styled reference document makes the output readable.
	// If it cannot be built, fall back to pandoc's default styling.
	if refDoc, rerr := c.buildReferenceDoc(pandoc); rerr != nil {
		c.logf("  warning: could not build reference document, using pandoc defaults: %v", rerr)
	} else {
		args = append(args, "--reference-doc", refDoc)
	}

	cmd := exec.Command(pandoc, args...) //nolint:gosec // G204: pandoc path is auto-detected or user-provided, args are controlled
	cmd.Dir = c.workDir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pandoc DOCX conversion failed: %w\noutput: %s", err, out)
	}
	c.logf("  pandoc output: %s", strings.TrimSpace(string(out)))
	return nil
}

// renderMermaidPNGs renders each Mermaid block to a PNG inside the working
// directory and records the working-directory-relative path on the block.
// It is a no-op when no blocks are present.
func (c *Converter) renderMermaidPNGs(blocks []*mermaidBlock) error {
	if len(blocks) == 0 {
		return nil
	}
	pcfg, err := c.ensurePuppeteerConfig()
	if err != nil {
		return fmt.Errorf("puppeteer config: %w", err)
	}
	for i, b := range blocks {
		name, err := c.renderSingleDiagramPNG(i, b.Source, pcfg)
		if err != nil {
			return fmt.Errorf("diagram %d: %w", i, err)
		}
		b.ImagePath = name
		c.logf("  diagram %d rendered to PNG (%s)", i, name)
	}
	return nil
}

// docxFont returns the configured DOCX font family, or the default.
func (c *Converter) docxFont() string {
	if c.cfg.DOCXFont != "" {
		return c.cfg.DOCXFont
	}
	return defaultDOCXFont
}

// findPandoc resolves the pandoc binary. An explicit PandocPath wins; otherwise
// the well-known locations in pandocDefaultPaths are searched on PATH.
func (c *Converter) findPandoc() (string, error) {
	if c.cfg.PandocPath != "" {
		resolved, err := exec.LookPath(c.cfg.PandocPath)
		if err != nil {
			return "", fmt.Errorf("pandoc not found at %q: %w; %s", c.cfg.PandocPath, err, pandocInstallHint)
		}
		return resolved, nil
	}

	for _, name := range pandocDefaultPaths {
		if resolved, err := exec.LookPath(name); err == nil {
			return resolved, nil
		}
	}
	return "", fmt.Errorf("pandoc not found; %s", pandocInstallHint)
}

// buildReferenceDoc fetches pandoc's default reference.docx, styles it for
// readable output, writes the result into the working directory, and returns its
// path. The returned document is passed to pandoc via --reference-doc.
func (c *Converter) buildReferenceDoc(pandoc string) (string, error) {
	cmd := exec.Command(pandoc, "--print-default-data-file", "reference.docx") //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("read default reference: %w\n%s", err, stderr.String())
	}

	patched, err := patchReferenceDoc(stdout.Bytes(), c.docxFont())
	if err != nil {
		return "", err
	}

	refPath := filepath.Join(c.workDir, "reference.docx")
	if err := os.WriteFile(refPath, patched, 0o644); err != nil {
		return "", fmt.Errorf("write reference doc: %w", err)
	}
	return refPath, nil
}

// patchReferenceDoc styles a reference.docx in memory so converted documents
// read well: bordered tables, a 10.5pt body, compact headings and a
// Japanese-friendly font for both Latin and East Asian text. It returns the
// rewritten archive bytes.
func patchReferenceDoc(docx []byte, font string) ([]byte, error) {
	return rewriteDocxEntries(docx, map[string]func(string) string{
		"word/styles.xml": func(s string) string {
			s = injectTableBorders(s)
			s = setBodyFontSize(s, docxBodyHalfPt)
			s = shrinkHeadings(s)
			return s
		},
		"word/theme/theme1.xml": func(t string) string {
			return setThemeFonts(t, font)
		},
	})
}

// addTableBorders rewrites the word/styles.xml entry of a reference.docx so the
// default Table style carries visible borders, returning the new archive bytes.
func addTableBorders(docx []byte) ([]byte, error) {
	return rewriteDocxEntries(docx, map[string]func(string) string{
		"word/styles.xml": injectTableBorders,
	})
}

// rewriteDocxEntries copies a docx archive, applying the matching transform to
// each named entry's textual contents and leaving every other entry untouched.
func rewriteDocxEntries(docx []byte, transforms map[string]func(string) string) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("open reference zip: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		content, err := readZipFile(f)
		if err != nil {
			return nil, err
		}
		if transform, ok := transforms[f.Name]; ok {
			content = []byte(transform(string(content)))
		}
		w, err := zw.Create(f.Name)
		if err != nil {
			return nil, fmt.Errorf("create zip entry %s: %w", f.Name, err)
		}
		if _, err := w.Write(content); err != nil {
			return nil, fmt.Errorf("write zip entry %s: %w", f.Name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize reference zip: %w", err)
	}
	return buf.Bytes(), nil
}

// readZipFile reads the full contents of a single archive entry.
func readZipFile(f *zip.File) ([]byte, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return nil, fmt.Errorf("read zip entry %s: %w", f.Name, err)
	}
	return content, nil
}

// injectTableBorders inserts the border fragment into the Table style's table
// properties, just before the cell margins (to respect the OOXML element order).
// When the expected structure is absent it returns the input unchanged.
func injectTableBorders(styles string) string {
	return tableStyleRe.ReplaceAllStringFunc(styles, func(style string) string {
		if strings.Contains(style, "<w:tblBorders>") {
			return style
		}
		if strings.Contains(style, "<w:tblCellMar>") {
			return strings.Replace(style, "<w:tblCellMar>", tableBorders+"<w:tblCellMar>", 1)
		}
		if strings.Contains(style, "</w:tblPr>") {
			return strings.Replace(style, "</w:tblPr>", tableBorders+"</w:tblPr>", 1)
		}
		return style
	})
}

// themeLatinRe and themeEastAsiaRe match the typeface attribute of the Latin and
// East Asian font entries in a theme's major/minor font scheme.
var (
	themeLatinRe    = regexp.MustCompile(`(<a:latin typeface=")[^"]*(")`)
	themeEastAsiaRe = regexp.MustCompile(`(<a:ea typeface=")[^"]*(")`)
)

// setThemeFonts rewrites the major and minor font scheme in theme1.xml so both
// Latin and East Asian runs use font. This fixes Japanese text, which pandoc's
// default theme leaves to an empty (fallback) East Asian typeface, and cascades
// to headings via the theme font references in styles.xml.
func setThemeFonts(theme, font string) string {
	theme = themeLatinRe.ReplaceAllString(theme, "${1}"+font+"${2}")
	theme = themeEastAsiaRe.ReplaceAllString(theme, "${1}"+font+"${2}")
	return theme
}

// docDefaultsRe matches the docDefaults block of a styles.xml document.
var docDefaultsRe = regexp.MustCompile(`(?s)<w:docDefaults>.*?</w:docDefaults>`)

// szValRe matches the value of a <w:sz>/<w:szCs> font-size element.
var szValRe = regexp.MustCompile(`(<w:sz(?:Cs)? w:val=")\d+(")`)

// setBodyFontSize sets the default body font size (in half-points) within the
// docDefaults block, leaving size overrides on individual styles intact.
func setBodyFontSize(styles, halfPt string) string {
	return docDefaultsRe.ReplaceAllStringFunc(styles, func(block string) string {
		return szValRe.ReplaceAllString(block, "${1}"+halfPt+"${2}")
	})
}

// shrinkHeadings replaces pandoc's oversized heading font sizes with the compact
// sizes in docxHeadingHalfPt so headings sit closer to the body text.
func shrinkHeadings(styles string) string {
	for id, halfPt := range docxHeadingHalfPt {
		re := styleBlockRe(id)
		styles = re.ReplaceAllStringFunc(styles, func(block string) string {
			return szValRe.ReplaceAllString(block, "${1}"+halfPt+"${2}")
		})
	}
	return styles
}

// styleBlockRe returns a regexp matching a single <w:style> block by its
// styleId attribute.
func styleBlockRe(styleID string) *regexp.Regexp {
	return regexp.MustCompile(`(?s)<w:style [^>]*w:styleId="` + regexp.QuoteMeta(styleID) + `".*?</w:style>`)
}
