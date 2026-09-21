package converter

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/chromedp/cdproto/emulation"
	"github.com/chromedp/cdproto/page"
	"github.com/chromedp/chromedp"
)

// pptxImageScale is the device scale factor slides are captured at. Marp CLI
// uses 2 for PPTX too: a slide shown full screen on a large display is
// stretched well past 1280 pixels, and at 1x the text visibly softens.
const pptxImageScale = 2

// slideRect is a slide's box in page coordinates, in CSS pixels.
type slideRect struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
	W float64 `json:"w"`
	H float64 `json:"h"`
}

// slideRectsJS returns every slide's box relative to the document, which is
// what a screenshot clip is expressed in.
const slideRectsJS = `Array.from(document.querySelectorAll('section.slide')).map(s => {
  const r = s.getBoundingClientRect();
  return {x: r.left + window.scrollX, y: r.top + window.scrollY, w: r.width, h: r.height};
})`

// renderSlidePNGs opens a deck's HTML in a headless browser and captures each
// slide as a PNG at scale times its CSS size into outDir, returning the files'
// paths in slide order, and measures overflow on the way. Each image is
// written out as soon as it is captured, so memory holds one slide at a time
// however long or image-heavy the deck is.
//
// The page is laid out exactly as printPDF lays it out — print media, fonts
// settled — so each image matches the corresponding page of the PDF. The
// viewport is the slide size, and each capture is clipped to one slide's box
// with captureBeyondViewport, so slides below the first are captured too.
func renderSlidePNGs(ctx context.Context, browser, htmlPath, outDir string, size slideSize, scale float64, timeout time.Duration) ([]string, []slideOverflow, error) {
	ctx, cancel := startBrowser(ctx, browser, timeout)
	defer cancel()

	var (
		rects     []slideRect
		overflows []slideOverflow
		images    []string
	)
	err := chromedp.Run(ctx,
		emulation.SetDeviceMetricsOverride(int64(size.widthPx), int64(size.heightPx), scale, false),
		emulation.SetEmulatedMedia().WithMedia("print"),
		chromedp.Navigate(fileURL(htmlPath)),
		awaitFonts(),
		chromedp.Evaluate(measureSlidesJS, &overflows),
		chromedp.Evaluate(slideRectsJS, &rects),
		chromedp.ActionFunc(func(ctx context.Context) error {
			for i, r := range rects {
				img, err := page.CaptureScreenshot().
					WithFormat(page.CaptureScreenshotFormatPng).
					WithClip(&page.Viewport{X: r.X, Y: r.Y, Width: r.W, Height: r.H, Scale: 1}).
					WithCaptureBeyondViewport(true).
					WithFromSurface(true).
					Do(ctx)
				if err != nil {
					return fmt.Errorf("capture slide %d: %w", i+1, err)
				}
				path := filepath.Join(outDir, fmt.Sprintf("slide-%03d.png", i+1))
				if err := os.WriteFile(path, img, 0o600); err != nil {
					return fmt.Errorf("save slide %d: %w", i+1, err)
				}
				images = append(images, path)
			}
			return nil
		}),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("drive headless browser: %w", err)
	}
	return images, overflows, nil
}
