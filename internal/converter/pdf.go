package converter

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/cdproto/runtime"
	"github.com/chromedp/chromedp"
)

// printTimeout bounds the whole browser session. A document that cannot be
// printed within it is treated as a failure rather than hanging the CLI.
const printTimeout = 120 * time.Second

// browserStartTimeout is how long to wait for the browser to publish its
// DevTools websocket URL.
//
// The chromedp default of 20s is not enough for a genuinely cold start: on
// a fresh CI runner the first Chrome launch sets up a profile and lost the race,
// while a second launch in the same run finished comfortably. The overall
// printTimeout still bounds the operation, so a browser that never comes up
// fails rather than hangs.
const browserStartTimeout = 60 * time.Second

// printOptions holds everything Page.printToPDF needs, in the inches it expects.
type printOptions struct {
	widthInches        float64
	heightInches       float64
	marginTopInches    float64
	marginBottomInches float64
	marginLeftInches   float64
	marginRightInches  float64
}

// resolvePrintOptions converts the paper size and margins from the command line
// into the inches Chromium's print API takes. Each margin is reported by its
// flag name when it cannot be parsed, so the message points at what to fix.
func resolvePrintOptions(cfg *Config) (printOptions, error) {
	width, height, err := paperSizeInches(cfg.PageSize)
	if err != nil {
		return printOptions{}, err
	}

	margins := []struct {
		flag  string
		value string
		out   *float64
	}{}
	opts := printOptions{widthInches: width, heightInches: height}
	margins = append(margins,
		struct {
			flag  string
			value string
			out   *float64
		}{"-margin-top", cfg.MarginTop, &opts.marginTopInches},
		struct {
			flag  string
			value string
			out   *float64
		}{"-margin-bottom", cfg.MarginBottom, &opts.marginBottomInches},
		struct {
			flag  string
			value string
			out   *float64
		}{"-margin-left", cfg.MarginLeft, &opts.marginLeftInches},
		struct {
			flag  string
			value string
			out   *float64
		}{"-margin-right", cfg.MarginRight, &opts.marginRightInches},
	)

	for _, m := range margins {
		inches, err := lengthInches(m.value)
		if err != nil {
			return printOptions{}, fmt.Errorf("%s: %w", m.flag, err)
		}
		*m.out = inches
	}
	return opts, nil
}

// printPDF renders htmlPath to a PDF at pdfPath by driving a headless Chromium
// over the DevTools Protocol.
//
// The browser is the one chromiumPath finds, which is also the browser handed to
// mmdc through the generated Puppeteer config — so a single Chromium serves both
// the diagram and the print stage, and CHROME_PATH overrides both.
//
// A non-nil deck prints each page at the slide size with no margin, ignoring
// -page-size and -margin-*; Convert has already refused those flags when they
// were given explicitly.
//
// For a deck it also returns the slides whose content does not fit, measured in
// the same page load the PDF is printed from, so the report matches the output.
func (c *Converter) printPDF(ctx context.Context, htmlPath, pdfPath string, deck *slideSize) ([]slideOverflow, error) {
	var opts printOptions
	if deck != nil {
		opts = slidePrintOptions(*deck)
	} else {
		var err error
		if opts, err = resolvePrintOptions(c.cfg); err != nil {
			return nil, err
		}
	}

	browser, err := chromiumPath()
	if err != nil {
		return nil, err
	}
	c.logf("  driving %s over the DevTools Protocol", browser)

	pdf, overflows, err := renderPDF(ctx, browser, htmlPath, opts, deck != nil, printTimeout)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil { //nolint:gosec // G306: a document the caller asked to be written
		return nil, fmt.Errorf("write pdf: %w", err)
	}
	c.logf("  printed %d bytes of PDF", len(pdf))
	return overflows, nil
}

// renderPDF opens htmlPath in a headless browser and returns the printed PDF.
//
// Fonts are waited on explicitly: the page declares CJK faces with @font-face,
// and printing before document.fonts settles produces a PDF laid out with
// fallback metrics.
//
// With measure set, the page is laid out as print media and every slide is
// checked for overflow before printing, after the fonts have settled so the
// measurements use the final metrics.
func renderPDF(ctx context.Context, browser, htmlPath string, opts printOptions, measure bool, timeout time.Duration) ([]byte, []slideOverflow, error) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:],
		chromedp.ExecPath(browser),
		// The sandbox cannot be used as root, which is the normal case in CI
		// containers, and this browser only ever opens a local file md2pdf
		// generated itself.
		chromedp.NoSandbox,
		chromedp.DisableGPU,
		chromedp.WSURLReadTimeout(browserStartTimeout),
	)

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()
	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()
	ctx, cancelTimeout := context.WithTimeout(browserCtx, timeout)
	defer cancelTimeout()

	var (
		pdf         []byte
		fontsLoaded bool
		overflows   []slideOverflow
	)
	err := chromedp.Run(ctx,
		emulation.SetEmulatedMedia().WithMedia("print"),
		chromedp.Navigate(fileURL(htmlPath)),
		// document.fonts.ready resolves to a Promise, so it has to be awaited
		// rather than merely evaluated. It resolves *to the FontFaceSet*, which
		// CDP cannot reliably serialize by value, so the promise is mapped to a
		// primitive before it comes back.
		chromedp.Evaluate("document.fonts.ready.then(() => true)", &fontsLoaded,
			func(p *runtime.EvaluateParams) *runtime.EvaluateParams {
				return p.WithAwaitPromise(true)
			}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			if !measure {
				return nil
			}
			return chromedp.Evaluate(measureSlidesJS, &overflows).Do(ctx)
		}),
		chromedp.ActionFunc(func(ctx context.Context) error {
			var err error
			pdf, _, err = page.PrintToPDF().
				WithPrintBackground(true).
				WithPaperWidth(opts.widthInches).
				WithPaperHeight(opts.heightInches).
				WithMarginTop(opts.marginTopInches).
				WithMarginBottom(opts.marginBottomInches).
				WithMarginLeft(opts.marginLeftInches).
				WithMarginRight(opts.marginRightInches).
				Do(ctx)
			if err != nil {
				return fmt.Errorf("print to pdf: %w", err)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("drive headless browser: %w", err)
	}
	return pdf, overflows, nil
}

// fileURL turns a local path into a file:// URL.
//
// Concatenating "file://" with the path is wrong in two ways that matter: a
// Windows path yields file://C:\... , where Chromium reads the drive letter as
// a host, and any "#" in a filename would start a URL fragment and truncate the
// path. Building the URL escapes both.
func fileURL(path string) string {
	slashed := strings.ReplaceAll(path, `\`, "/")
	if !strings.HasPrefix(slashed, "/") {
		slashed = "/" + slashed
	}
	return (&url.URL{Scheme: "file", Path: slashed}).String()
}
