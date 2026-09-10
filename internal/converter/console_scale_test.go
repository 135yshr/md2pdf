package converter

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// TestValidateMermaidScale covers the accepted range for -mermaid-scale. The
// bounds exist because either extreme stops being a diagram: too small and the
// boxes merge into a smudge, too large and almost none of it is on screen.
func TestValidateMermaidScale(t *testing.T) {
	tests := []struct {
		name    string
		scale   float64
		wantErr bool
	}{
		{"unset", 0, false},
		{"minimum", mermaidScaleMin, false},
		{"half", 0.5, false},
		{"natural", 1, false},
		{"double", 2, false},
		{"maximum", mermaidScaleMax, false},
		{"below the minimum", mermaidScaleMin / 2, true},
		{"above the maximum", mermaidScaleMax * 2, true},
		{"negative", -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMermaidScale(tc.scale)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateMermaidScale(%v) error = %v, wantErr %t", tc.scale, err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "mermaid-scale") {
				t.Errorf("error %q does not name the flag", err)
			}
		})
	}
}

// TestScaleColumnBudget covers how the scale turns the wrap width into the
// columns an image may occupy. An unset scale has to leave the budget exactly as
// it was, or every existing diagram would move.
func TestScaleColumnBudget(t *testing.T) {
	tests := []struct {
		name  string
		width int
		scale float64
		want  int
	}{
		{"unset keeps the width", 118, 0, 118},
		{"natural keeps the width", 118, 1, 118},
		{"half", 118, 0.5, 59},
		{"rounds to the nearest column", 101, 0.5, 51},
		{"double", 60, 2, 120},
		{"a tiny scale still leaves one column", 4, 0.1, 1},
		{"no width means no budget", 0, 0.5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scaleColumnBudget(tc.width, tc.scale); got != tc.want {
				t.Errorf("scaleColumnBudget(%d, %v) = %d, want %d", tc.width, tc.scale, got, tc.want)
			}
		})
	}
}

// TestEncodeTerminalImage_HonoursTheScaledBudget covers the knob doing its job:
// the same diagram, asked to occupy fewer columns, encodes fewer columns.
func TestEncodeTerminalImage_HonoursTheScaledBudget(t *testing.T) {
	// 2000px is far wider than any of these budgets, so every case clamps and
	// the encoded column count is the budget itself.
	data := stubPNG(t, 2000, 400)
	transport := terminalImageTransport{protocol: imageProtocolITerm2}

	full, err := encodeTerminalImage(transport, data, scaleColumnBudget(118, 1))
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	half, err := encodeTerminalImage(transport, data, scaleColumnBudget(118, 0.5))
	if err != nil {
		t.Fatalf("half: %v", err)
	}

	fullCols, halfCols := iterm2Columns(t, full), iterm2Columns(t, half)
	if fullCols != 118 {
		t.Errorf("unscaled image occupies %d columns, want the full 118", fullCols)
	}
	if halfCols != 59 {
		t.Errorf("halved image occupies %d columns, want 59", halfCols)
	}
}

// iterm2Columns pulls the cell width out of an iTerm2 inline image sequence.
func iterm2Columns(t *testing.T, sequence string) int {
	t.Helper()
	_, rest, ok := strings.Cut(sequence, "width=")
	if !ok {
		t.Fatalf("no width parameter in the sequence: %q", sequence[:min(len(sequence), 120)])
	}
	digits := rest
	for i, r := range rest {
		if r < '0' || r > '9' {
			digits = rest[:i]
			break
		}
	}
	cols, err := strconv.Atoi(digits)
	if err != nil {
		t.Fatalf("width parameter %q is not a number (from %q)", digits, rest[:min(len(rest), 40)])
	}
	return cols
}

// stubWideRasterizer stands in for mmdc with a diagram far wider than any wrap
// width, so every scale clamps and the encoded column count is the budget.
func stubWideRasterizer(t *testing.T, c *Converter, dir string) {
	t.Helper()
	c.mermaidAvailable = func() bool { return true }
	c.rasterizeMermaid = func(int, string) (string, error) {
		path := filepath.Join(dir, "wide.png")
		if err := os.WriteFile(path, stubPNG(t, 2000, 400), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}
}

// TestPrepareConsoleMermaid_ScalesInlineImages walks the scale through the plan
// to the emitted sequence. An unset scale has to leave the diagram exactly where
// it was, or the flag would move every existing image.
func TestPrepareConsoleMermaid_ScalesInlineImages(t *testing.T) {
	const width = 118
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	dir := t.TempDir()

	columnsAt := func(t *testing.T, scale float64) int {
		t.Helper()
		c := newTestConverter(t, &Config{Format: FormatConsole})
		stubWideRasterizer(t, c, dir)

		plan := consoleMermaidPlan{
			mode:      MermaidRenderImage,
			transport: terminalImageTransport{protocol: imageProtocolITerm2},
			scale:     scale,
		}
		_, diagrams, err := c.prepareConsoleMermaid(md, plan, width)
		if err != nil {
			t.Fatalf("prepareConsoleMermaid: %v", err)
		}
		if len(diagrams) != 1 {
			t.Fatalf("got %d diagrams, want 1", len(diagrams))
		}
		return iterm2Columns(t, diagrams[0].content)
	}

	tests := []struct {
		name  string
		scale float64
		want  int
	}{
		{"unset fills the wrap width", 0, width},
		{"natural fills the wrap width", 1, width},
		{"half", 0.5, 59},
		{"quarter", 0.25, 30},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := columnsAt(t, tc.scale); got != tc.want {
				t.Errorf("scale %v gave %d columns, want %d", tc.scale, got, tc.want)
			}
		})
	}
}

// TestPrepareConsoleMermaid_ReportsAScaleWiderThanTheScreen covers the one
// scale that cannot deliver what it promises. An inline image cannot be
// scrolled, so anything past the wrap width is simply off screen — the run says
// so rather than letting the diagram look mysteriously cropped.
func TestPrepareConsoleMermaid_ReportsAScaleWiderThanTheScreen(t *testing.T) {
	const width = 118
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")

	logsFor := func(t *testing.T, scale float64) string {
		t.Helper()
		var stderr bytes.Buffer
		c := newTestConverter(t, &Config{Format: FormatConsole, Verbose: true})
		c.stderr = &stderr
		stubWideRasterizer(t, c, t.TempDir())

		plan := consoleMermaidPlan{
			mode:      MermaidRenderImage,
			transport: terminalImageTransport{protocol: imageProtocolITerm2},
			scale:     scale,
		}
		if _, _, err := c.prepareConsoleMermaid(md, plan, width); err != nil {
			t.Fatalf("prepareConsoleMermaid: %v", err)
		}
		return stderr.String()
	}

	if logged := logsFor(t, 2); !strings.Contains(logged, "off screen") {
		t.Errorf("a scale past the wrap width was not reported: %q", logged)
	}
	for _, scale := range []float64{0, 1, 0.5} {
		if logged := logsFor(t, scale); strings.Contains(logged, "off screen") {
			t.Errorf("scale %v reported going off screen: %q", scale, logged)
		}
	}
}
