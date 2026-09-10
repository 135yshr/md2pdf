package converter

import (
	"context"
	"image"
	"image/color"
	"math"
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
		// Every ordered comparison against NaN is false, so a range check alone
		// lets it through; math.Round then converts it to an implementation
		// defined integer rather than reporting anything.
		{"NaN", math.NaN(), true},
		{"positive infinity", math.Inf(1), true},
		{"negative infinity", math.Inf(-1), true},
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

// TestFitColumns covers how a scale turns a diagram's own width into the
// columns it occupies.
//
// The factor applies to the image, not to the terminal-width ceiling. Scaling
// only the ceiling would do nothing at all for a diagram already narrower than
// the wrap width, which is most of them: a 40-column diagram asked to halve
// would stay at 40 because 40 still fits under the lowered ceiling.
func TestFitColumns(t *testing.T) {
	// consoleImageCellWidthPx is 10, so pixels/10 is the natural column count.
	tests := []struct {
		name       string
		pixelWidth int
		maxColumns int
		scale      float64
		wantCols   int
		wantSized  bool
	}{
		{"natural size fits and is left alone", 400, 118, 0, 40, false},
		{"scale one is the same as unset", 400, 118, 1, 40, false},
		{"a diagram narrower than the width still halves", 400, 118, 0.5, 20, true},
		{"and still doubles", 400, 118, 2, 80, true},
		{"enlarging stops at the wrap width", 400, 118, 4, 118, true},
		{"a diagram wider than the width is brought back to it", 2000, 118, 0, 118, true},
		{"and scales from its own width, not the ceiling", 2000, 118, 0.5, 100, true},
		{"a tiny scale still leaves one column", 400, 118, 0.1, 4, true},
		{"no ceiling means no clamping", 400, 0, 2, 80, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cols, sized := fitColumns(tc.pixelWidth, tc.maxColumns, tc.scale)
			if cols != tc.wantCols {
				t.Errorf("cols = %d, want %d", cols, tc.wantCols)
			}
			if sized != tc.wantSized {
				t.Errorf("sized = %t, want %t", sized, tc.wantSized)
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

	full, err := encodeTerminalImage(transport, data, 118, 1)
	if err != nil {
		t.Fatalf("full: %v", err)
	}
	half, err := encodeTerminalImage(transport, data, 118, 0.5)
	if err != nil {
		t.Fatalf("half: %v", err)
	}

	fullCols, halfCols := iterm2Columns(t, full), iterm2Columns(t, half)
	if fullCols != 118 {
		t.Errorf("unscaled image occupies %d columns, want the full 118", fullCols)
	}
	// 2000px is 200 natural columns, halved to 100, which still fits 118.
	if halfCols != 100 {
		t.Errorf("halved image occupies %d columns, want 100", halfCols)
	}
}

// iterm2Columns pulls the cell width out of an iTerm2 inline image sequence.
func iterm2Columns(t *testing.T, sequence string) int {
	t.Helper()
	_, rest, ok := strings.Cut(sequence, "width=")
	if !ok {
		// No width parameter means the image is drawn at its natural size.
		return 0
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
	c.rasterizeMermaid = func(context.Context, int, string) (string, error) {
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
		_, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, width)
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
		{"unset clamps 200 natural columns to the width", 0, width},
		{"natural clamps to the width", 1, width},
		{"half of the diagram's own 200 columns", 0.5, 100},
		{"a quarter of them", 0.25, 50},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := columnsAt(t, tc.scale); got != tc.want {
				t.Errorf("scale %v gave %d columns, want %d", tc.scale, got, tc.want)
			}
		})
	}
}

// TestResampleImage_EnlargesAsWellAsShrinks covers the Sixel path, which is the
// only one that has to resize the pixels itself: kitty and iTerm2 are told a
// column count and scale the image on their side, so a resampler that refused
// to enlarge left -mermaid-scale 2 doing nothing in a Sixel terminal.
func TestResampleImage_EnlargesAsWellAsShrinks(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 40, 20))
	for y := range 20 {
		for x := range 40 {
			src.Set(x, y, color.RGBA{R: uint8(x * 6), G: uint8(y * 12), B: 0x40, A: 0xff})
		}
	}

	tests := []struct {
		name        string
		targetWidth int
		wantWidth   int
		wantHeight  int
	}{
		{"shrinks", 20, 20, 10},
		{"enlarges", 80, 80, 40},
		{"the same width is left alone", 40, 40, 20},
		{"a nonsensical target is left alone", 0, 40, 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resampleImage(src, tc.targetWidth)
			b := got.Bounds()
			if b.Dx() != tc.wantWidth || b.Dy() != tc.wantHeight {
				t.Errorf("resampleImage(src, %d) is %dx%d, want %dx%d",
					tc.targetWidth, b.Dx(), b.Dy(), tc.wantWidth, tc.wantHeight)
			}
		})
	}
}

// TestEncodeTerminalImage_SixelEnlarges is the end-to-end half: a Sixel payload
// for an enlarged diagram has to be bigger than the unscaled one, which it
// cannot be if the resampler declined to enlarge.
func TestEncodeTerminalImage_SixelEnlarges(t *testing.T) {
	data := stubPNG(t, 200, 100)
	transport := terminalImageTransport{protocol: imageProtocolSixel}

	natural, err := encodeTerminalImage(transport, data, 118, 1)
	if err != nil {
		t.Fatalf("natural: %v", err)
	}
	doubled, err := encodeTerminalImage(transport, data, 118, 2)
	if err != nil {
		t.Fatalf("doubled: %v", err)
	}
	if len(doubled) <= len(natural) {
		t.Errorf("doubling produced %d bytes of Sixel, not more than the %d bytes at natural size",
			len(doubled), len(natural))
	}
}
