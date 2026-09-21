package converter

import (
	"bytes"
	"image/png"
	"path/filepath"
	"strings"
	"testing"
)

// convertPPTX converts content to a .pptx with a real browser and returns its
// parts and what the converter wrote to stderr.
func convertPPTX(t *testing.T, content string) (map[string][]byte, string) {
	t.Helper()
	if _, err := chromiumPath(); err != nil {
		t.Skipf("no Chromium available: %v", err)
	}
	input := writeDoc(t, content)
	out := filepath.Join(t.TempDir(), "deck.pptx")
	c := newTestConverter(t, &Config{Format: FormatPPTX})
	var stderr bytes.Buffer
	c.stderr = &stderr
	if err := c.Convert(t.Context(), []string{input}, out); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return readPPTX(t, out), stderr.String()
}

func TestPPTXOutput_OneImageSlidePerSlide(t *testing.T) {
	parts, _ := convertPPTX(t, "---\nmarp: true\ntitle: My deck\n---\n\n"+
		"# One\n\n---\n\n# Two\n\n<!-- Say hello -->\n\n---\n\n# Three\n")
	if got := len(sldIDRE.FindAllString(string(parts["ppt/presentation.xml"]), -1)); got != 3 {
		t.Fatalf("presentation has %d slides, want 3", got)
	}
	var images [][]byte
	for i := 1; i <= 3; i++ {
		data := parts["ppt/media/image"+string(rune('0'+i))+".png"]
		cfg, err := png.DecodeConfig(bytes.NewReader(data))
		if err != nil {
			t.Fatalf("slide %d image is not a PNG: %v", i, err)
		}
		if cfg.Width != 2560 || cfg.Height != 1440 {
			t.Errorf("slide %d image is %dx%d, want 2560x1440", i, cfg.Width, cfg.Height)
		}
		images = append(images, data)
	}
	if bytes.Equal(images[0], images[1]) || bytes.Equal(images[1], images[2]) {
		t.Error("two slides captured the same image; the clip did not move")
	}
	if !strings.Contains(string(parts["ppt/notesSlides/notesSlide2.xml"]), "Say hello") {
		t.Error("slide 2's presenter note is missing")
	}
	if !strings.Contains(string(parts["docProps/core.xml"]), "<dc:title>My deck</dc:title>") {
		t.Error("the front-matter title is not the presentation title")
	}
}

func TestPPTXOutput_FourByThree(t *testing.T) {
	parts, _ := convertPPTX(t, "---\nsize: 4:3\n---\n\n# One\n")
	if !strings.Contains(string(parts["ppt/presentation.xml"]), `cx="9144000" cy="6858000"`) {
		t.Error("slide size is not 4:3")
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(parts["ppt/media/image1.png"]))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if cfg.Width != 1920 || cfg.Height != 1440 {
		t.Errorf("image is %dx%d, want 1920x1440", cfg.Width, cfg.Height)
	}
}

func TestPPTXOutput_IsAlwaysADeck(t *testing.T) {
	parts, _ := convertPPTX(t, threeSlides)
	if got := len(sldIDRE.FindAllString(string(parts["ppt/presentation.xml"]), -1)); got != 3 {
		t.Errorf("a file without marp: true gave %d slides, want 3", got)
	}
}

func TestPPTXOutput_WarnsAboutOverflow(t *testing.T) {
	var items strings.Builder
	for range 60 {
		items.WriteString("- item\n")
	}
	_, stderr := convertPPTX(t, items.String())
	if !strings.Contains(stderr, "slide 1 overflows the slide (height)") {
		t.Errorf("stderr = %q, want an overflow warning", stderr)
	}
}

func TestPPTXOutput_RejectsPaperFlags(t *testing.T) {
	input := writeDoc(t, "# One\n")
	c := newTestConverter(t, &Config{Format: FormatPPTX, PaperFlagsSet: []string{"-page-size"}})
	err := c.Convert(t.Context(), []string{input}, filepath.Join(t.TempDir(), "o.pptx"))
	if err == nil || !strings.Contains(err.Error(), "-page-size") {
		t.Fatalf("Convert error = %v, want the paper flag rejected", err)
	}
}
