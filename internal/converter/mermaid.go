package converter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// mermaidImageSubdir is the working-directory subdirectory that holds PNGs
// rendered for DOCX output. The unusual name keeps generated diagrams from
// colliding with user-supplied images copied by copyImages.
const mermaidImageSubdir = "_md2pdf_mermaid"

// mermaidBackground is the background colour passed to mmdc for rendered diagrams.
const mermaidBackground = "white"

// mermaidPNGScale scales raster (PNG) diagram output up so it stays sharp when
// embedded in a document.
const mermaidPNGScale = "2"

// puppeteerConfig is the JSON structure written for mmdc's -p flag.
type puppeteerConfig struct {
	ExecutablePath string   `json:"executablePath"`
	Args           []string `json:"args"`
}

// renderMermaid iterates over all Mermaid blocks in doc, writes each source to
// a .mmd temp file, invokes mmdc to produce an SVG, and stores the SVG content
// back into the block's SVGContent field.
func (c *Converter) renderMermaid(doc *parsedDoc) error {
	if len(doc.mermaidBlocks) == 0 {
		return nil
	}

	pcfgPath, err := c.ensurePuppeteerConfig()
	if err != nil {
		return fmt.Errorf("puppeteer config: %w", err)
	}

	// DOCX output embeds raster images, since Word cannot reliably display the
	// inline/standalone SVG that pandoc produces from HTML. PDF output keeps the
	// crisp inline SVG.
	if strings.EqualFold(c.cfg.Format, "docx") {
		for i, block := range doc.mermaidBlocks {
			name, err := c.renderSingleDiagramPNG(i, block.Source, pcfgPath)
			if err != nil {
				return fmt.Errorf("diagram %d: %w", i, err)
			}
			block.ImagePath = name
			c.logf("  diagram %d rendered to PNG (%s)", i, name)
		}
		return nil
	}

	for i, block := range doc.mermaidBlocks {
		svg, err := c.renderSingleDiagram(i, block.Source, pcfgPath)
		if err != nil {
			return fmt.Errorf("diagram %d: %w", i, err)
		}
		block.SVGContent = svg
		c.logf("  diagram %d rendered (%d bytes)", i, len(svg))
	}
	return nil
}

// renderSingleDiagramPNG writes the Mermaid source to a temp file, runs mmdc to
// produce a PNG inside the working directory, and returns the PNG's filename
// (relative to the working directory) for embedding as an <img>.
func (c *Converter) renderSingleDiagramPNG(idx int, source, puppeteerCfgPath string) (string, error) {
	// Isolate generated diagrams in a dedicated subdirectory so they cannot be
	// overwritten by user-supplied images that copyImages later copies into the
	// working directory (e.g. a Markdown <img> referencing "diagram_0.png").
	if err := os.MkdirAll(filepath.Join(c.workDir, mermaidImageSubdir), 0o755); err != nil {
		return "", fmt.Errorf("create mermaid image dir: %w", err)
	}

	mmdFile := filepath.Join(c.workDir, mermaidImageSubdir, fmt.Sprintf("diagram_%d.mmd", idx))
	pngRel := filepath.ToSlash(filepath.Join(mermaidImageSubdir, fmt.Sprintf("diagram_%d.png", idx)))
	pngFile := filepath.Join(c.workDir, filepath.FromSlash(pngRel))

	// Scale up so the raster diagram stays sharp in the document.
	if err := c.runMmdc(mmdFile, pngFile, source, puppeteerCfgPath, "-s", mermaidPNGScale); err != nil {
		return "", err
	}
	if _, err := os.Stat(pngFile); err != nil {
		return "", fmt.Errorf("mmdc did not produce PNG output: %w", err)
	}
	return pngRel, nil
}

// renderSingleDiagram writes the Mermaid source to a temp file, runs mmdc, and
// returns the resulting SVG bytes as a string.
func (c *Converter) renderSingleDiagram(idx int, source, puppeteerCfgPath string) (string, error) {
	mmdFile := filepath.Join(c.workDir, fmt.Sprintf("diagram_%d.mmd", idx))
	svgFile := filepath.Join(c.workDir, fmt.Sprintf("diagram_%d.svg", idx))

	if err := c.runMmdc(mmdFile, svgFile, source, puppeteerCfgPath); err != nil {
		return "", err
	}

	svgBytes, err := os.ReadFile(svgFile)
	if err != nil {
		return "", fmt.Errorf("read SVG output: %w", err)
	}
	return string(svgBytes), nil
}

// runMmdc writes the Mermaid source to mmdFile and invokes mmdc to render it to
// outFile. extraArgs are appended after the standard flags (e.g. PNG scaling),
// and the Puppeteer config is passed when non-empty.
func (c *Converter) runMmdc(mmdFile, outFile, source, puppeteerCfgPath string, extraArgs ...string) error {
	if err := os.WriteFile(mmdFile, []byte(source), 0o644); err != nil {
		return fmt.Errorf("write .mmd file: %w", err)
	}

	args := []string{"-i", mmdFile, "-o", outFile, "-b", mermaidBackground}
	args = append(args, extraArgs...)
	if puppeteerCfgPath != "" {
		args = append(args, "-p", puppeteerCfgPath)
	}

	cmd := exec.Command(c.resolveMmdc(), args...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mmdc failed: %w\noutput: %s", err, out)
	}
	return nil
}

// resolveMmdc returns the mmdc binary to invoke, honouring the configured path
// and resolving it against $PATH so the caller's environment is respected.
func (c *Converter) resolveMmdc() string {
	mmdcBin := c.cfg.MmdcPath
	if mmdcBin == "" {
		mmdcBin = "mmdc"
	}
	if resolved, err := exec.LookPath(mmdcBin); err == nil {
		mmdcBin = resolved
	}
	return mmdcBin
}

// ensurePuppeteerConfig returns the path to a Puppeteer JSON config suitable
// for mmdc. If the user provided one via -puppeteer-config, it is used as-is;
// otherwise a temporary config pointing at the detected system Chromium is
// generated inside the working directory.
func (c *Converter) ensurePuppeteerConfig() (string, error) {
	if c.cfg.PuppeteerConfig != "" {
		return c.cfg.PuppeteerConfig, nil
	}

	chromeExe, err := chromiumPath()
	if err != nil {
		return "", err
	}

	cfg := puppeteerConfig{
		ExecutablePath: chromeExe,
		Args:           []string{"--no-sandbox", "--disable-setuid-sandbox"},
	}
	data, err := json.Marshal(cfg)
	if err != nil {
		return "", err
	}

	cfgPath := filepath.Join(c.workDir, "puppeteer.json")
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return "", fmt.Errorf("write puppeteer config: %w", err)
	}
	c.logf("  auto-generated Puppeteer config: %s (chrome: %s)", cfgPath, chromeExe)
	return cfgPath, nil
}
