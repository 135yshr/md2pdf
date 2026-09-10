package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolvePrintOptions(t *testing.T) {
	tests := []struct {
		name       string
		cfg        *Config
		wantW      float64
		wantTop    float64
		wantLeft   float64
		wantErrSub string
	}{
		{
			name:     "the shipped defaults",
			cfg:      &Config{PageSize: "A4", MarginTop: "18mm", MarginBottom: "18mm", MarginLeft: "14mm", MarginRight: "14mm"},
			wantW:    8.2677,
			wantTop:  0.70866,
			wantLeft: 0.55118,
		},
		{
			name:     "an empty config is A4 with no margins",
			cfg:      &Config{},
			wantW:    8.2677,
			wantTop:  0,
			wantLeft: 0,
		},
		{
			name:     "mixed units",
			cfg:      &Config{PageSize: "Letter", MarginTop: "1in", MarginLeft: "36pt"},
			wantW:    8.5,
			wantTop:  1,
			wantLeft: 0.5,
		},
		{
			name:       "a bad page size is reported",
			cfg:        &Config{PageSize: "B5"},
			wantErrSub: "B5",
		},
		{
			name:       "a bad margin names which one",
			cfg:        &Config{PageSize: "A4", MarginTop: "18furlong"},
			wantErrSub: "margin-top",
		},
		{
			name:       "a bad bottom margin names which one",
			cfg:        &Config{PageSize: "A4", MarginBottom: "nope"},
			wantErrSub: "margin-bottom",
		},
		{
			name:       "a bad left margin names which one",
			cfg:        &Config{PageSize: "A4", MarginLeft: "nope"},
			wantErrSub: "margin-left",
		},
		{
			name:       "a bad right margin names which one",
			cfg:        &Config{PageSize: "A4", MarginRight: "nope"},
			wantErrSub: "margin-right",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolvePrintOptions(tc.cfg)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("resolvePrintOptions() = %+v, want an error", got)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolvePrintOptions() error = %v", err)
			}
			if !closeTo(got.widthInches, tc.wantW) {
				t.Errorf("width = %.4f, want %.4f", got.widthInches, tc.wantW)
			}
			if !closeTo(got.marginTopInches, tc.wantTop) {
				t.Errorf("top margin = %.5f, want %.5f", got.marginTopInches, tc.wantTop)
			}
			if !closeTo(got.marginLeftInches, tc.wantLeft) {
				t.Errorf("left margin = %.5f, want %.5f", got.marginLeftInches, tc.wantLeft)
			}
		})
	}
}

// TestPrintPDF_ProducesAPDF drives a real browser, so it skips when none is
// installed. This is the only place the CDP wiring is exercised end to end.
func TestPrintPDF_ProducesAPDF(t *testing.T) {
	if _, err := chromiumPath(); err != nil {
		t.Skipf("no Chromium available: %v", err)
	}

	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "doc.html")
	pdfPath := filepath.Join(dir, "doc.pdf")
	page := `<!DOCTYPE html><html><head><meta charset="UTF-8"><title>T</title>
<style>body{font-family:sans-serif}</style></head>
<body><h1>Heading</h1><p>Body text and 日本語.</p></body></html>`
	if err := os.WriteFile(htmlPath, []byte(page), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}

	c := newTestConverter(t, &Config{
		PageSize: "A4", MarginTop: "18mm", MarginBottom: "18mm",
		MarginLeft: "14mm", MarginRight: "14mm",
	})
	if err := c.printPDF(t.Context(), htmlPath, pdfPath); err != nil {
		t.Fatalf("printPDF: %v", err)
	}

	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Errorf("output is not a PDF: first bytes %q", data[:min(8, len(data))])
	}
	if len(data) < 1000 {
		t.Errorf("PDF is suspiciously small (%d bytes); it may be blank", len(data))
	}
}

// TestPrintPDF_ReportsABadPageSizeBeforeLaunching checks configuration errors
// surface without paying to start a browser.
func TestPrintPDF_ReportsABadPageSizeBeforeLaunching(t *testing.T) {
	c := newTestConverter(t, &Config{PageSize: "B5"})
	err := c.printPDF(t.Context(), filepath.Join(t.TempDir(), "none.html"), filepath.Join(t.TempDir(), "out.pdf"))
	if err == nil {
		t.Fatal("expected an error for an unsupported page size")
	}
	if !strings.Contains(err.Error(), "B5") {
		t.Errorf("error does not name the bad size: %v", err)
	}
}

// TestFileURL covers the two ways concatenating "file://" with a path goes
// wrong: a Windows drive letter read as a host, and "#" starting a fragment.
func TestFileURL(t *testing.T) {
	tests := []struct {
		name string
		path string
		want string
	}{
		{"unix path", "/tmp/doc.html", "file:///tmp/doc.html"},
		{"windows path gets a root and forward slashes", `C:\Users\me\doc.html`, "file:///C:/Users/me/doc.html"},
		{"spaces are escaped", "/tmp/my doc.html", "file:///tmp/my%20doc.html"},
		{"a hash is escaped rather than starting a fragment", "/tmp/a#b.html", "file:///tmp/a%23b.html"},
		{"a question mark is escaped", "/tmp/a?b.html", "file:///tmp/a%3Fb.html"},
		{"non-ASCII is percent-encoded", "/tmp/\u65e5\u672c\u8a9e.html", "file:///tmp/%E6%97%A5%E6%9C%AC%E8%AA%9E.html"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fileURL(tc.path); got != tc.want {
				t.Errorf("fileURL(%q) = %q, want %q", tc.path, got, tc.want)
			}
		})
	}
}

// TestPrintPDF_HandlesAwkwardFilenames prints from a path containing a space
// and a "#", which the previous string concatenation would have truncated.
func TestPrintPDF_HandlesAwkwardFilenames(t *testing.T) {
	if _, err := chromiumPath(); err != nil {
		t.Skipf("no Chromium available: %v", err)
	}

	dir := t.TempDir()
	htmlPath := filepath.Join(dir, "my #1 report.html")
	pdfPath := filepath.Join(dir, "out.pdf")
	if err := os.WriteFile(htmlPath, []byte("<!DOCTYPE html><html><body><h1>X</h1></body></html>"), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}

	c := newTestConverter(t, &Config{PageSize: "A4"})
	if err := c.printPDF(t.Context(), htmlPath, pdfPath); err != nil {
		t.Fatalf("printPDF with an awkward filename: %v", err)
	}
	data, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatalf("read pdf: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Error("output is not a PDF")
	}
}
