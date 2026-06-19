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

// convertDOCX converts the assembled HTML file to a DOCX file using pandoc.
//
// The command runs with its working directory set to the converter's temporary
// directory so that pandoc resolves the relative image paths emitted by the
// HTML stage against the copies placed there by copyImages. A reference document
// with bordered tables is generated and applied when possible.
func (c *Converter) convertDOCX(htmlPath, docxPath string) error {
	pandoc, err := c.findPandoc()
	if err != nil {
		return err
	}

	absHTML, err := filepath.Abs(htmlPath)
	if err != nil {
		return fmt.Errorf("resolve html path: %w", err)
	}

	args := []string{absHTML, "-f", "html", "-o", docxPath}

	// Best-effort: a bordered reference document makes tables look like tables.
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

// buildReferenceDoc fetches pandoc's default reference.docx, adds borders to the
// Table style, writes the result into the working directory, and returns its
// path. The returned document is passed to pandoc via --reference-doc.
func (c *Converter) buildReferenceDoc(pandoc string) (string, error) {
	cmd := exec.Command(pandoc, "--print-default-data-file", "reference.docx") //nolint:gosec
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("read default reference: %w\n%s", err, stderr.String())
	}

	patched, err := addTableBorders(stdout.Bytes())
	if err != nil {
		return "", err
	}

	refPath := filepath.Join(c.workDir, "reference.docx")
	if err := os.WriteFile(refPath, patched, 0o644); err != nil {
		return "", fmt.Errorf("write reference doc: %w", err)
	}
	return refPath, nil
}

// addTableBorders rewrites the word/styles.xml entry of a reference.docx so the
// default Table style carries visible borders, returning the new archive bytes.
func addTableBorders(docx []byte) ([]byte, error) {
	zr, err := zip.NewReader(bytes.NewReader(docx), int64(len(docx)))
	if err != nil {
		return nil, fmt.Errorf("open reference zip: %w", err)
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, f := range zr.File {
		if err := copyZipEntry(zw, f); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("finalize reference zip: %w", err)
	}
	return buf.Bytes(), nil
}

// copyZipEntry copies a single archive entry into zw, patching word/styles.xml
// to add table borders along the way.
func copyZipEntry(zw *zip.Writer, f *zip.File) error {
	rc, err := f.Open()
	if err != nil {
		return fmt.Errorf("open zip entry %s: %w", f.Name, err)
	}
	defer rc.Close()

	content, err := io.ReadAll(rc)
	if err != nil {
		return fmt.Errorf("read zip entry %s: %w", f.Name, err)
	}

	if f.Name == "word/styles.xml" {
		content = []byte(injectTableBorders(string(content)))
	}

	w, err := zw.Create(f.Name)
	if err != nil {
		return fmt.Errorf("create zip entry %s: %w", f.Name, err)
	}
	if _, err := w.Write(content); err != nil {
		return fmt.Errorf("write zip entry %s: %w", f.Name, err)
	}
	return nil
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
