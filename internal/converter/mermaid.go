package converter

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

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

// renderSingleDiagram writes the Mermaid source to a temp file, runs mmdc, and
// returns the resulting SVG bytes as a string.
func (c *Converter) renderSingleDiagram(idx int, source, puppeteerCfgPath string) (string, error) {
	mmdFile := filepath.Join(c.workDir, fmt.Sprintf("diagram_%d.mmd", idx))
	svgFile := filepath.Join(c.workDir, fmt.Sprintf("diagram_%d.svg", idx))

	if err := os.WriteFile(mmdFile, []byte(source), 0o644); err != nil {
		return "", fmt.Errorf("write .mmd file: %w", err)
	}

	// Resolve the mmdc binary at runtime so the caller's $PATH is honoured.
	mmdcBin := c.cfg.MmdcPath
	if mmdcBin == "" {
		mmdcBin = "mmdc"
	}
	if resolved, lerr := exec.LookPath(mmdcBin); lerr == nil {
		mmdcBin = resolved
	}

	args := []string{
		"-i", mmdFile,
		"-o", svgFile,
		"-b", "white",
	}
	if puppeteerCfgPath != "" {
		args = append(args, "-p", puppeteerCfgPath)
	}

	cmd := exec.Command(mmdcBin, args...) //nolint:gosec
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("mmdc failed: %w\noutput: %s", err, out)
	}

	svgBytes, err := os.ReadFile(svgFile)
	if err != nil {
		return "", fmt.Errorf("read SVG output: %w", err)
	}
	return string(svgBytes), nil
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
