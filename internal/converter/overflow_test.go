package converter

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverflowWarnings(t *testing.T) {
	got := overflowWarnings("deck.md", []slideOverflow{
		{Slide: 2, Height: true},
		{Slide: 4, Width: true},
		{Slide: 5, Height: true, Width: true},
	})
	want := []string{
		"warning: deck.md: slide 2 overflows the slide (height); the rest is cut off",
		"warning: deck.md: slide 4 overflows the slide (width); the rest is cut off",
		"warning: deck.md: slide 5 overflows the slide (height and width); the rest is cut off",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("warnings =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
	if len(overflowWarnings("deck.md", nil)) != 0 {
		t.Error("no overflow should produce no warning")
	}
}

// convertPDFStderr converts content to PDF with a real browser and returns
// what the converter wrote to stderr. Verbose is off, as by default.
func convertPDFStderr(t *testing.T, content string) (string, string) {
	t.Helper()
	if _, err := chromiumPath(); err != nil {
		t.Skipf("no Chromium available: %v", err)
	}
	input := writeDoc(t, content)
	c := newTestConverter(t, &Config{Format: FormatPDF, PageSize: "A4"})
	var stderr bytes.Buffer
	c.stderr = &stderr
	if err := c.Convert(t.Context(), []string{input}, filepath.Join(t.TempDir(), "out.pdf")); err != nil {
		t.Fatalf("Convert: %v", err)
	}
	return input, stderr.String()
}

func TestPDFOutput_WarnsAboutAnOverflowingSlide(t *testing.T) {
	var items strings.Builder
	for i := range 60 {
		items.WriteString("- item ")
		items.WriteString(strings.Repeat("x", i%7+1))
		items.WriteString("\n")
	}
	input, stderr := convertPDFStderr(t, "---\nmarp: true\n---\n\n# One\n\n---\n\n"+items.String()+"\n---\n\n# Three\n")
	want := "warning: " + input + ": slide 2 overflows the slide (height)"
	if !strings.Contains(stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr, want)
	}
	if strings.Contains(stderr, "slide 1 ") || strings.Contains(stderr, "slide 3 ") {
		t.Errorf("a slide that fits was reported: %q", stderr)
	}
}

func TestPDFOutput_WarnsAboutAWidthOverflow(t *testing.T) {
	cell := strings.Repeat("W", 200)
	_, stderr := convertPDFStderr(t, "---\nmarp: true\n---\n\n| a | b |\n|---|---|\n| "+cell+" | x |\n")
	if !strings.Contains(stderr, "slide 1 overflows the slide (width)") {
		t.Errorf("stderr = %q, want a width overflow for slide 1", stderr)
	}
}

func TestPDFOutput_NoWarningWhenEverySlideFits(t *testing.T) {
	_, stderr := convertPDFStderr(t, "---\nmarp: true\n---\n\n"+threeSlides)
	if stderr != "" {
		t.Errorf("stderr = %q, want nothing", stderr)
	}
}

func TestPDFOutput_DocumentModeIsNotMeasured(t *testing.T) {
	var long strings.Builder
	for range 200 {
		long.WriteString("- a long document\n")
	}
	_, stderr := convertPDFStderr(t, long.String())
	if strings.Contains(stderr, "overflows") {
		t.Errorf("document mode reported an overflow: %q", stderr)
	}
}
