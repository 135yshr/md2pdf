package converter

import (
	"fmt"
	"os/exec"
	"path/filepath"
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

// convertDOCX converts the assembled HTML file to a DOCX file using pandoc.
//
// The command runs with its working directory set to the converter's temporary
// directory so that pandoc resolves the relative image paths emitted by the
// HTML stage against the copies placed there by copyImages.
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
