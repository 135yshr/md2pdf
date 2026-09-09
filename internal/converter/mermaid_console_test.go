package converter

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// stubPNG returns the bytes of a small opaque PNG usable as a stand-in for an
// mmdc-rendered diagram.
func stubPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{R: uint8(x % 256), G: uint8(y % 256), B: 0x80, A: 0xff})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode stub png: %v", err)
	}
	return buf.Bytes()
}

// useStubRasterizer replaces the converter's Mermaid rasterizer with one that
// writes a stub PNG for every diagram, so tests never depend on mmdc being
// installed. It reports how many times the rasterizer ran through calls.
func useStubRasterizer(t *testing.T, c *Converter, dir string, calls *int) {
	t.Helper()
	c.mermaidAvailable = func() bool { return true }
	c.rasterizeMermaid = func(int, string) (string, error) {
		*calls++
		path := filepath.Join(dir, "diagram.png")
		if err := os.WriteFile(path, stubPNG(t, 400, 200), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}
}

func TestValidateMermaidRenderMode(t *testing.T) {
	tests := []struct {
		value   string
		wantErr bool
	}{
		{"", false},
		{"auto", false},
		{"image", false},
		{"source", false},
		{"ascii", true}, // added by #44; rejected until then
		{"nope", true},
		{"AUTO", true},
	}
	for _, tc := range tests {
		t.Run(tc.value, func(t *testing.T) {
			err := ValidateMermaidRenderMode(tc.value)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateMermaidRenderMode(%q) = nil, want an error", tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateMermaidRenderMode(%q) = %v, want nil", tc.value, err)
			}
		})
	}
}

func TestDetectImageProtocol(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want terminalImageProtocol
	}{
		{"kitty via TERM", map[string]string{"TERM": "xterm-kitty"}, imageProtocolKitty},
		{"kitty via KITTY_WINDOW_ID", map[string]string{"KITTY_WINDOW_ID": "1"}, imageProtocolKitty},
		{"ghostty speaks kitty", map[string]string{"TERM_PROGRAM": "ghostty"}, imageProtocolKitty},
		{"iTerm2", map[string]string{"TERM_PROGRAM": "iTerm.app"}, imageProtocolITerm2},
		{"WezTerm", map[string]string{"TERM_PROGRAM": "WezTerm"}, imageProtocolITerm2},
		{"sixel via TERM", map[string]string{"TERM": "xterm-sixel"}, imageProtocolSixel},
		{"foot speaks sixel", map[string]string{"TERM": "foot"}, imageProtocolSixel},
		{"plain xterm has none", map[string]string{"TERM": "xterm-256color"}, imageProtocolNone},
		{"empty environment has none", map[string]string{}, imageProtocolNone},
		{"kitty wins over iTerm2", map[string]string{"TERM": "xterm-kitty", "TERM_PROGRAM": "iTerm.app"}, imageProtocolKitty},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			getenv := func(k string) string { return tc.env[k] }
			if got := detectImageProtocol(getenv); got != tc.want {
				t.Errorf("detectImageProtocol() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestResolveConsoleMermaidPlan(t *testing.T) {
	kitty := func(k string) string {
		if k == "TERM" {
			return "xterm-kitty"
		}
		return ""
	}
	plain := func(string) string { return "" }

	tests := []struct {
		name         string
		mode         string
		isTTY        bool
		noColor      bool
		getenv       func(string) string
		wantImages   bool
		wantProtocol terminalImageProtocol
		wantErr      bool
	}{
		{"auto on kitty emits images", "auto", true, false, kitty, true, imageProtocolKitty, false},
		{"empty mode behaves like auto", "", true, false, kitty, true, imageProtocolKitty, false},
		{"auto on a plain terminal falls back", "auto", true, false, plain, false, imageProtocolNone, false},
		{"source never emits images", "source", true, false, kitty, false, imageProtocolNone, false},
		{"piped output never emits images", "auto", false, false, kitty, false, imageProtocolNone, false},
		{"NO_COLOR never emits images", "auto", true, true, kitty, false, imageProtocolNone, false},
		{"explicit image on kitty", "image", true, false, kitty, true, imageProtocolKitty, false},
		{"explicit image on a plain terminal errors", "image", true, false, plain, false, imageProtocolNone, true},
		{"explicit image when piped errors", "image", false, false, kitty, false, imageProtocolNone, true},
		{"explicit image with NO_COLOR errors", "image", true, true, kitty, false, imageProtocolNone, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := resolveConsoleMermaidPlan(tc.mode, tc.isTTY, tc.noColor, tc.getenv)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveConsoleMermaidPlan() = %+v, want an error", plan)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveConsoleMermaidPlan() error = %v", err)
			}
			if got := plan.emitsImages(); got != tc.wantImages {
				t.Errorf("emitsImages() = %t, want %t", got, tc.wantImages)
			}
			if plan.protocol != tc.wantProtocol {
				t.Errorf("protocol = %v, want %v", plan.protocol, tc.wantProtocol)
			}
		})
	}
}

// TestResolveConsoleMermaidPlan_ImageErrorNamesCapability checks the error text
// explains what is missing rather than failing opaquely.
func TestResolveConsoleMermaidPlan_ImageErrorNamesCapability(t *testing.T) {
	_, err := resolveConsoleMermaidPlan("image", true, false, func(string) string { return "" })
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, want := range []string{"image", "mermaid-render"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestEncodeTerminalImage_ProtocolSequences(t *testing.T) {
	data := stubPNG(t, 40, 20)

	tests := []struct {
		name     string
		protocol terminalImageProtocol
		wantSub  string
	}{
		{"kitty uses an APC graphics sequence", imageProtocolKitty, "\x1b_G"},
		{"iTerm2 uses OSC 1337 File=", imageProtocolITerm2, "\x1b]1337;File="},
		{"sixel uses a DCS sequence", imageProtocolSixel, "\x1bP"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := encodeTerminalImage(tc.protocol, data, 40)
			if err != nil {
				t.Fatalf("encodeTerminalImage: %v", err)
			}
			if !strings.Contains(got, tc.wantSub) {
				t.Errorf("sequence for %v does not contain %q", tc.protocol, tc.wantSub)
			}
			if got == "" {
				t.Error("sequence is empty")
			}
		})
	}
}

func TestEncodeTerminalImage_NoneIsAnError(t *testing.T) {
	if _, err := encodeTerminalImage(imageProtocolNone, stubPNG(t, 8, 8), 40); err == nil {
		t.Error("expected an error for imageProtocolNone")
	}
}

func TestEncodeTerminalImage_RejectsInvalidPNG(t *testing.T) {
	for _, p := range []terminalImageProtocol{imageProtocolKitty, imageProtocolITerm2, imageProtocolSixel} {
		if _, err := encodeTerminalImage(p, []byte("not a png"), 40); err == nil {
			t.Errorf("protocol %v accepted invalid PNG data", p)
		}
	}
}

// TestEncodeTerminalImage_ConstrainsWideImages checks a diagram wider than the
// wrap width is scaled down to it instead of overflowing the line.
func TestEncodeTerminalImage_ConstrainsWideImages(t *testing.T) {
	wide := stubPNG(t, 4000, 400)

	kittySeq, err := encodeTerminalImage(imageProtocolKitty, wide, 40)
	if err != nil {
		t.Fatalf("kitty: %v", err)
	}
	if !strings.Contains(kittySeq, "c=40") {
		t.Errorf("kitty sequence does not clamp columns to 40: %q", firstBytes(kittySeq, 120))
	}

	itermSeq, err := encodeTerminalImage(imageProtocolITerm2, wide, 40)
	if err != nil {
		t.Fatalf("iterm2: %v", err)
	}
	if !strings.Contains(itermSeq, "width=40") {
		t.Errorf("iTerm2 sequence does not clamp width to 40 cells: %q", firstBytes(itermSeq, 160))
	}
}

// TestEncodeTerminalImage_LeavesNarrowImagesAtNaturalSize checks a diagram that
// already fits is not blown up to fill the terminal.
func TestEncodeTerminalImage_LeavesNarrowImagesAtNaturalSize(t *testing.T) {
	narrow := stubPNG(t, 40, 20)
	seq, err := encodeTerminalImage(imageProtocolKitty, narrow, 100)
	if err != nil {
		t.Fatalf("encodeTerminalImage: %v", err)
	}
	if strings.Contains(seq, "c=100") {
		t.Errorf("narrow image was stretched to the full width: %q", firstBytes(seq, 120))
	}
}

// firstBytes truncates s for error messages, since image escape sequences are
// far too long to print in full.
func firstBytes(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func TestSpliceConsoleDiagrams_ReplacesPlaceholderLine(t *testing.T) {
	rendered := "  # Title   \n\n  MD2PDFDG0        \n\n  after\n"
	out, missing := spliceConsoleDiagrams(rendered, []consoleDiagram{
		{placeholder: "MD2PDFDG0", content: "ART-A\nART-B"},
	})
	if len(missing) != 0 {
		t.Errorf("unexpected unplaced diagrams: %v", missing)
	}

	if strings.Contains(out, "MD2PDFDG0") {
		t.Errorf("placeholder survived the splice:\n%s", out)
	}
	for _, want := range []string{"  ART-A", "  ART-B", "  # Title", "  after"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
}

func TestSpliceConsoleDiagrams_KeepsDocumentOrder(t *testing.T) {
	rendered := "  MD2PDFDG0\n\n  mid\n\n  MD2PDFDG1\n"
	out, missing := spliceConsoleDiagrams(rendered, []consoleDiagram{
		{placeholder: "MD2PDFDG0", content: "FIRST"},
		{placeholder: "MD2PDFDG1", content: "SECOND"},
	})
	if len(missing) != 0 {
		t.Errorf("unexpected unplaced diagrams: %v", missing)
	}

	iFirst := strings.Index(out, "FIRST")
	iMid := strings.Index(out, "mid")
	iSecond := strings.Index(out, "SECOND")
	if iFirst < 0 || iMid < 0 || iSecond < 0 {
		t.Fatalf("missing content:\n%s", out)
	}
	if iFirst >= iMid || iMid >= iSecond {
		t.Errorf("diagrams are out of document order (%d, %d, %d):\n%s", iFirst, iMid, iSecond, out)
	}
}

// TestSpliceConsoleDiagrams_StripsStyledPlaceholderLine checks the splice works
// when glamour has wrapped the placeholder in ANSI color escapes.
func TestSpliceConsoleDiagrams_StripsStyledPlaceholderLine(t *testing.T) {
	rendered := "  \x1b[38;5;252mMD2PDFDG0\x1b[m\x1b[38;5;252m \x1b[m\n"
	out, _ := spliceConsoleDiagrams(rendered, []consoleDiagram{
		{placeholder: "MD2PDFDG0", content: "ART"},
	})

	if strings.Contains(out, "MD2PDFDG0") {
		t.Errorf("placeholder survived:\n%q", out)
	}
	if strings.Contains(out, "38;5;252") {
		t.Errorf("leftover style escapes from the placeholder line:\n%q", out)
	}
	if !strings.Contains(out, "  ART") {
		t.Errorf("art not indented to the placeholder column:\n%q", out)
	}
}

func TestSpliceConsoleDiagrams_NoDiagramsIsIdentity(t *testing.T) {
	rendered := "  # Title\n\n  body\n"
	if got, _ := spliceConsoleDiagrams(rendered, nil); got != rendered {
		t.Errorf("splice altered the document with no diagrams:\n%q", got)
	}
}

// newTestConverter builds a Converter with a real temporary working directory
// and registers its cleanup with t.
func newTestConverter(t *testing.T, cfg *Config) *Converter {
	t.Helper()
	c, err := New(cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	t.Cleanup(c.Close)
	return c
}

// TestPrepareConsoleMermaid_SourceModeLeavesMarkdownUntouched is the regression
// guard: source mode must hand glamour the original bytes.
func TestPrepareConsoleMermaid_SourceModeLeavesMarkdownUntouched(t *testing.T) {
	md := []byte("# T\n\n```mermaid\ngraph TD\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	calls := 0
	useStubRasterizer(t, c, t.TempDir(), &calls)

	plan := consoleMermaidPlan{mode: MermaidRenderSource}
	got, diagrams, err := c.prepareConsoleMermaid(md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if !bytes.Equal(got, md) {
		t.Errorf("markdown was rewritten in source mode:\n%q", got)
	}
	if len(diagrams) != 0 {
		t.Errorf("got %d diagrams in source mode, want 0", len(diagrams))
	}
	if calls != 0 {
		t.Errorf("rasterizer ran %d times in source mode, want 0", calls)
	}
}

// TestPrepareConsoleMermaid_NoBlocksSkipsRenderingEntirely covers the acceptance
// criterion that a document without Mermaid never starts a diagram render.
func TestPrepareConsoleMermaid_NoBlocksSkipsRenderingEntirely(t *testing.T) {
	md := []byte("# T\n\nJust prose and a ```go\nfmt.Println()\n``` block.\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	calls := 0
	useStubRasterizer(t, c, t.TempDir(), &calls)

	plan := consoleMermaidPlan{mode: MermaidRenderImage, protocol: imageProtocolKitty}
	got, diagrams, err := c.prepareConsoleMermaid(md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if !bytes.Equal(got, md) {
		t.Errorf("markdown was rewritten with no Mermaid blocks:\n%q", got)
	}
	if len(diagrams) != 0 || calls != 0 {
		t.Errorf("diagrams=%d rasterizerCalls=%d, want 0 and 0", len(diagrams), calls)
	}
}

func TestPrepareConsoleMermaid_RendersEachBlockInOrder(t *testing.T) {
	md := []byte("```mermaid\ngraph TD\n A-->B\n```\n\ntext\n\n```mermaid\ngraph LR\n C-->D\n```\n\n```mermaid\ngraph TD\n E-->F\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	calls := 0
	useStubRasterizer(t, c, t.TempDir(), &calls)

	plan := consoleMermaidPlan{mode: MermaidRenderImage, protocol: imageProtocolKitty}
	rewritten, diagrams, err := c.prepareConsoleMermaid(md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if calls != 3 {
		t.Errorf("rasterizer ran %d times, want 3", calls)
	}
	if len(diagrams) != 3 {
		t.Fatalf("got %d diagrams, want 3", len(diagrams))
	}
	for i, d := range diagrams {
		if !strings.Contains(d.content, "\x1b_G") {
			t.Errorf("diagram %d is not a kitty sequence", i)
		}
		if !strings.Contains(string(rewritten), d.placeholder) {
			t.Errorf("token %q missing from rewritten markdown", d.placeholder)
		}
	}
	if strings.Contains(string(rewritten), "A-->B") {
		t.Errorf("Mermaid source survived in the markdown handed to glamour:\n%s", rewritten)
	}
}

// TestPrepareConsoleMermaid_FailedBlockFallsBackToItsSource covers the mixed
// case: one diagram fails, the others still render, and the run succeeds.
func TestPrepareConsoleMermaid_FailedBlockFallsBackToItsSource(t *testing.T) {
	md := []byte("```mermaid\ngraph TD\n GOOD-->X\n```\n\n```mermaid\nBROKEN\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	dir := t.TempDir()
	c.mermaidAvailable = func() bool { return true }
	c.rasterizeMermaid = func(_ int, src string) (string, error) {
		if strings.Contains(src, "BROKEN") {
			return "", os.ErrInvalid
		}
		path := filepath.Join(dir, "ok.png")
		if err := os.WriteFile(path, stubPNG(t, 100, 50), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}

	plan := consoleMermaidPlan{mode: MermaidRenderImage, protocol: imageProtocolKitty}
	rewritten, diagrams, err := c.prepareConsoleMermaid(md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid returned an error for a single bad block: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1 (the good one)", len(diagrams))
	}
	if !strings.Contains(string(rewritten), "BROKEN") {
		t.Errorf("failed block did not fall back to its source:\n%s", rewritten)
	}
	if strings.Contains(string(rewritten), "GOOD-->X") {
		t.Errorf("successful block leaked its source:\n%s", rewritten)
	}
}

// TestPrepareConsoleMermaid_MissingMmdcFallsBackToSource covers an image-capable
// terminal without mmdc installed: no error, and the source is shown.
func TestPrepareConsoleMermaid_MissingMmdcFallsBackToSource(t *testing.T) {
	md := []byte("```mermaid\ngraph TD\n A-->B\n```\n")
	c := newTestConverter(t, &Config{
		Format:   FormatConsole,
		MmdcPath: filepath.Join(t.TempDir(), "definitely-not-installed-mmdc"),
	})
	// Leave rasterizeMermaid at its real implementation so the mmdc lookup runs.

	plan := consoleMermaidPlan{mode: MermaidRenderImage, protocol: imageProtocolKitty}
	rewritten, diagrams, err := c.prepareConsoleMermaid(md, plan, 80)
	if err != nil {
		t.Fatalf("missing mmdc must not be an error: %v", err)
	}
	if len(diagrams) != 0 {
		t.Errorf("got %d diagrams without mmdc, want 0", len(diagrams))
	}
	if !strings.Contains(string(rewritten), "A-->B") {
		t.Errorf("Mermaid source missing after mmdc fallback:\n%s", rewritten)
	}
}

// TestRenderConsole_PipedOutputEmitsNoImageSequences is the end-to-end guard for
// redirected output.
func TestRenderConsole_PipedOutputEmitsNoImageSequences(t *testing.T) {
	t.Setenv("TERM", "xterm-kitty")
	t.Setenv("NO_COLOR", "")

	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.txt")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{Format: FormatConsole, ConsolePager: false})
	calls := 0
	useStubRasterizer(t, c, dir, &calls)

	md := []byte("# T\n\n```mermaid\ngraph TD\n A-->B\n```\n")
	if err := c.renderConsole(md, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)
	for _, seq := range []string{"\x1b_G", "\x1b]1337;", "\x1bP"} {
		if strings.Contains(got, seq) {
			t.Errorf("image escape sequence %q leaked into piped output", seq)
		}
	}
	if calls != 0 {
		t.Errorf("rasterizer ran %d times for piped output, want 0", calls)
	}
	if !strings.Contains(got, "A-->B") {
		t.Errorf("Mermaid source missing from piped output:\n%s", got)
	}
}

// TestRenderConsole_ByteIdenticalToSourceModeWhenNoCapability is the v0.7.0
// regression guard: with no image capability the output must match what the
// pre-feature code produced, which is exactly source mode.
func TestRenderConsole_ByteIdenticalToSourceModeWhenNoCapability(t *testing.T) {
	md := []byte("# Diagram\n\nIntro.\n\n```mermaid\ngraph TD\n  A[Start] --> B[End]\n```\n\nOutro.\n")

	want, err := renderConsoleMarkdown(md, "notty", consoleFallbackWidth)
	if err != nil {
		t.Fatalf("baseline render: %v", err)
	}

	t.Setenv("TERM", "dumb")
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.txt")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{Format: FormatConsole, ConsolePager: false})
	if err := c.renderConsole(md, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("output drifted from the pre-feature rendering.\n got: %q\nwant: %q", got, want)
	}
}

// TestRenderConsole_ExplicitImageOnPlainTerminalFails covers the acceptance
// criterion that forcing images names the missing capability.
func TestRenderConsole_ExplicitImageOnPlainTerminalFails(t *testing.T) {
	t.Setenv("TERM", "dumb")

	f, err := os.Create(filepath.Join(t.TempDir(), "out.txt"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{
		Format:        FormatConsole,
		ConsolePager:  false,
		MermaidRender: MermaidRenderImage,
	})
	err = c.renderConsole([]byte("```mermaid\ngraph TD\n A-->B\n```\n"), f)
	if err == nil {
		t.Fatal("expected renderConsole to fail with -mermaid-render image on a plain terminal")
	}
	if !strings.Contains(err.Error(), "image") {
		t.Errorf("error does not name the missing capability: %v", err)
	}
}

// TestTokenFitsWrapWidth pins the width guard that keeps glamour from hard-wrapping
// a diagram token.
func TestTokenFitsWrapWidth(t *testing.T) {
	tests := []struct {
		tokenLen int
		width    int
		want     bool
	}{
		{9, 20, true},
		{9, 13, true},
		{9, 12, false},
		{11, 15, true},
		{11, 14, false},
	}
	for _, tc := range tests {
		if got := consoleTokenFits(tc.tokenLen, tc.width); got != tc.want {
			t.Errorf("consoleTokenFits(%d, %d) = %t, want %t", tc.tokenLen, tc.width, got, tc.want)
		}
	}
}

// TestPrepareConsoleMermaid_TokensSurviveNarrowGlamourWidths is the regression
// test for a token being split across lines: at every supported width the token
// must still be findable in glamour's output.
func TestPrepareConsoleMermaid_TokensSurviveNarrowGlamourWidths(t *testing.T) {
	md := []byte("# T\n\nIntro.\n\n```mermaid\ngraph TD\n A-->B\n```\n\nOutro.\n")

	for _, width := range []int{consoleMinWidth, 24, 40, 80, consoleMaxWidth} {
		t.Run(fmt.Sprintf("width%d", width), func(t *testing.T) {
			c := newTestConverter(t, &Config{Format: FormatConsole})
			calls := 0
			useStubRasterizer(t, c, t.TempDir(), &calls)

			plan := consoleMermaidPlan{mode: MermaidRenderImage, protocol: imageProtocolKitty}
			doc, diagrams, err := c.prepareConsoleMermaid(md, plan, width)
			if err != nil {
				t.Fatalf("prepareConsoleMermaid: %v", err)
			}
			if len(diagrams) != 1 {
				t.Fatalf("got %d diagrams at width %d, want 1", len(diagrams), width)
			}

			rendered, err := renderConsoleMarkdown(doc, "notty", width)
			if err != nil {
				t.Fatalf("renderConsoleMarkdown: %v", err)
			}
			out, missing := spliceConsoleDiagrams(string(rendered), diagrams)
			if len(missing) != 0 {
				t.Fatalf("width %d: diagram token was not placed (glamour wrapped it): %v", width, missing)
			}
			if strings.Contains(out, consoleTokenPrefix) {
				t.Errorf("width %d: a raw token leaked into the output:\n%s", width, out)
			}
			if !strings.Contains(out, "\x1b_G") {
				t.Errorf("width %d: kitty sequence missing from the output", width)
			}
		})
	}
}

// TestPrepareConsoleMermaid_TooNarrowFallsBackToSource covers an explicit -width
// so small that no token could survive: the document must fall back to source
// rather than leak a wrapped token.
func TestPrepareConsoleMermaid_TooNarrowFallsBackToSource(t *testing.T) {
	md := []byte("```mermaid\ngraph TD\n A-->B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	calls := 0
	useStubRasterizer(t, c, t.TempDir(), &calls)

	plan := consoleMermaidPlan{mode: MermaidRenderImage, protocol: imageProtocolKitty}
	doc, diagrams, err := c.prepareConsoleMermaid(md, plan, 6)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 0 {
		t.Errorf("got %d diagrams at width 6, want 0", len(diagrams))
	}
	if !bytes.Equal(doc, md) {
		t.Errorf("markdown was rewritten despite falling back:\n%q", doc)
	}
	if calls != 0 {
		t.Errorf("rasterizer ran %d times at an unusable width, want 0", calls)
	}
}
