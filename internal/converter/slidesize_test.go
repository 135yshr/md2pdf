package converter

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestResolveSlideSize(t *testing.T) {
	tests := []struct {
		name       string
		meta       frontMatter
		wantW      int
		wantH      int
		wantErrSub string
	}{
		{"default is 16:9", nil, 1280, 720, ""},
		{"explicit 16:9", frontMatter{"size": "16:9"}, 1280, 720, ""},
		{"4:3", frontMatter{"size": "4:3"}, 960, 720, ""},
		{"unknown preset lists the known ones", frontMatter{"size": "16:10"}, 0, 0, "16:9, 4:3"},
		{"a non-string is rejected", frontMatter{"size": 43}, 0, 0, "16:9, 4:3"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveSlideSize(tt.meta)
			if tt.wantErrSub != "" {
				if err == nil {
					t.Fatalf("resolveSlideSize = %+v, want an error", got)
				}
				if !strings.Contains(err.Error(), tt.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tt.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveSlideSize: %v", err)
			}
			if got.widthPx != tt.wantW || got.heightPx != tt.wantH {
				t.Errorf("size = %dx%d, want %dx%d", got.widthPx, got.heightPx, tt.wantW, tt.wantH)
			}
		})
	}
}

func TestSlidePrintOptions_AreTheSlideWithNoMargins(t *testing.T) {
	opts := slidePrintOptions(slideSize{name: "16:9", widthPx: 1280, heightPx: 720})
	if !closeTo(opts.widthInches, 13.3333) || !closeTo(opts.heightInches, 7.5) {
		t.Errorf("paper = %.4fx%.4f in, want 13.3333x7.5", opts.widthInches, opts.heightInches)
	}
	if opts.marginTopInches != 0 || opts.marginBottomInches != 0 || opts.marginLeftInches != 0 || opts.marginRightInches != 0 {
		t.Errorf("slide margins = %+v, want all zero", opts)
	}
}

func TestHTMLOutput_SlideSectionsHaveTheSlideSize(t *testing.T) {
	page := convertHTML(t, &Config{}, "---\nmarp: true\nsize: 4:3\n---\n\n"+threeSlides)
	for _, want := range []string{"width: 960px", "height: 720px"} {
		if !strings.Contains(page, want) {
			t.Errorf("slide CSS lacks %q", want)
		}
	}
}

func TestConvert_UnknownSlideSizeFails(t *testing.T) {
	input := writeDoc(t, "---\nmarp: true\nsize: 16:10\n---\n\n# A\n")
	c := newTestConverter(t, &Config{Format: FormatHTML})
	err := c.Convert(t.Context(), []string{input}, filepath.Join(t.TempDir(), "o.html"))
	if err == nil || !strings.Contains(err.Error(), "4:3") {
		t.Fatalf("Convert error = %v, want one listing the known sizes", err)
	}
}

func TestConvert_ExplicitPaperFlagsAreRejectedInSlideMode(t *testing.T) {
	for _, flag := range []string{"-page-size", "-margin-top"} {
		t.Run(flag, func(t *testing.T) {
			input := writeDoc(t, "---\nmarp: true\n---\n\n# A\n")
			c := newTestConverter(t, &Config{Format: FormatHTML, PaperFlagsSet: []string{flag}})
			err := c.Convert(t.Context(), []string{input}, filepath.Join(t.TempDir(), "o.html"))
			if err == nil {
				t.Fatal("Convert accepted a paper flag in slide mode")
			}
			if !strings.Contains(err.Error(), flag) {
				t.Errorf("error %q does not name %s", err, flag)
			}
		})
	}
}

func TestConvert_ExplicitPaperFlagsStillWorkForDocuments(t *testing.T) {
	input := writeDoc(t, "# A\n")
	c := newTestConverter(t, &Config{Format: FormatHTML, PaperFlagsSet: []string{"-page-size"}})
	if err := c.Convert(t.Context(), []string{input}, filepath.Join(t.TempDir(), "o.html")); err != nil {
		t.Fatalf("Convert: %v", err)
	}
}

var mediaBoxRE = regexp.MustCompile(`/MediaBox \[0 0 ([0-9.]+) ([0-9.]+)\]`)

func TestPDFOutput_SlidePagesHaveTheSlideSize(t *testing.T) {
	if _, err := chromiumPath(); err != nil {
		t.Skipf("no Chromium available: %v", err)
	}
	tests := []struct {
		name         string
		frontMatter  string
		wantW, wantH float64
	}{
		{"16:9", "marp: true", 960, 540},
		{"4:3", "marp: true\nsize: 4:3", 720, 540},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			input := writeDoc(t, "---\n"+tt.frontMatter+"\n---\n\n"+threeSlides)
			out := filepath.Join(t.TempDir(), "out.pdf")
			// The document-mode defaults are set, as the CLI sets them, to show
			// slide mode does not use them.
			c := newTestConverter(t, &Config{
				Format: FormatPDF, PageSize: "A4",
				MarginTop: "18mm", MarginBottom: "18mm", MarginLeft: "14mm", MarginRight: "14mm",
			})
			if err := c.Convert(t.Context(), []string{input}, out); err != nil {
				t.Fatalf("Convert: %v", err)
			}
			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("read pdf: %v", err)
			}
			boxes := mediaBoxRE.FindAllSubmatch(data, -1)
			if len(boxes) != 3 {
				t.Fatalf("PDF has %d pages, want 3 (an overflowing slide would add pages)", len(boxes))
			}
			for i, b := range boxes {
				w, _ := strconv.ParseFloat(string(b[1]), 64)
				h, _ := strconv.ParseFloat(string(b[2]), 64)
				if !closeToPt(w, tt.wantW) || !closeToPt(h, tt.wantH) {
					t.Errorf("page %d MediaBox = %gx%g pt, want %gx%g", i+1, w, h, tt.wantW, tt.wantH)
				}
			}
		})
	}
}

// closeToPt compares PDF points, where Chromium rounds to the nearest
// hundredth or so.
func closeToPt(a, b float64) bool {
	d := a - b
	return d < 0.5 && d > -0.5
}
