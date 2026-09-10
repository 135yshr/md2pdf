package converter

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

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
func (c *Converter) printPDF(ctx context.Context, htmlPath, pdfPath string) error {
	opts, err := resolvePrintOptions(c.cfg)
	if err != nil {
		return err
	}

	browser, err := chromiumPath()
	if err != nil {
		return err
	}
	c.logf("  driving %s over the DevTools Protocol", browser)

	pdf, err := renderPDF(ctx, browser, htmlPath, opts, printTimeout)
	if err != nil {
		return err
	}
	if err := os.WriteFile(pdfPath, pdf, 0o644); err != nil { //nolint:gosec // G306: a document the caller asked to be written
		return fmt.Errorf("write pdf: %w", err)
	}
	c.logf("  printed %d bytes of PDF", len(pdf))
	return nil
}

// renderPDF opens htmlPath in a headless browser and returns the printed PDF.
//
// Fonts are waited on explicitly: the page declares CJK faces with @font-face,
// and printing before document.fonts settles produces a PDF laid out with
// fallback metrics.
func renderPDF(ctx context.Context, browser, htmlPath string, opts printOptions, timeout time.Duration) ([]byte, error) {
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
	)
	err := chromedp.Run(ctx,
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
		return nil, fmt.Errorf("drive headless browser: %w", err)
	}
	return pdf, nil
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
