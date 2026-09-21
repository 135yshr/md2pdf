package converter

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/chromedp/chromedp"
)

// mmdcLikeSVG returns an SVG shaped like the ones mmdc writes: width="100%"
// with the natural width as an inline max-width, and the natural size only in
// the viewBox.
func mmdcLikeSVG(w, h int) string {
	return fmt.Sprintf(`<svg xmlns="http://www.w3.org/2000/svg" width="100%%" viewBox="0 0 %[1]d %[2]d" style="max-width: %[1]dpx;" role="graphics-document">`+
		`<rect x="0" y="0" width="%[1]d" height="%[2]d" fill="#cde"/></svg>`, w, h)
}

// slideDiagramPage builds the slide-mode HTML for src, with every Mermaid
// block replaced by an mmdc-like SVG of the given size, and returns its path.
func slideDiagramPage(t *testing.T, src string, w, h int) string {
	t.Helper()
	doc, err := parseSlides([]byte(src), nil)
	if err != nil {
		t.Fatalf("parseSlides: %v", err)
	}
	for _, b := range doc.mermaidBlocks {
		b.SVGContent = mmdcLikeSVG(w, h)
	}
	size := slideSizes[0]
	doc.deck = &size
	path := filepath.Join(t.TempDir(), "deck.html")
	c := newTestConverter(t, &Config{})
	if err := c.buildHTML(doc, path); err != nil {
		t.Fatalf("buildHTML: %v", err)
	}
	return path
}

// diagramBox returns the rendered size of the first diagram's SVG, and the
// slide overflows, from a real browser laid out as print media.
func diagramBox(t *testing.T, htmlPath string) (w, h float64, overflows []slideOverflow) {
	t.Helper()
	browser, err := chromiumPath()
	if err != nil {
		t.Skipf("no Chromium available: %v", err)
	}
	pdf := filepath.Join(t.TempDir(), "out.pdf")
	c := newTestConverter(t, &Config{})
	size := slideSizes[0]
	if overflows, err = c.printPDF(t.Context(), htmlPath, pdf, &size); err != nil {
		t.Fatalf("printPDF: %v", err)
	}

	alloc, cancelAlloc := chromedp.NewExecAllocator(t.Context(),
		append(chromedp.DefaultExecAllocatorOptions[:], chromedp.ExecPath(browser), chromedp.NoSandbox)...)
	defer cancelAlloc()
	ctx, cancel := chromedp.NewContext(alloc)
	defer cancel()
	var box struct{ W, H float64 }
	err = chromedp.Run(ctx,
		chromedp.EmulateViewport(int64(size.widthPx), int64(size.heightPx)),
		chromedp.Navigate(fileURL(htmlPath)),
		chromedp.ActionFunc(func(ctx context.Context) error {
			return chromedp.Evaluate(`(() => {
				const r = document.querySelector('.diagram-wrapper svg rect').getBoundingClientRect();
				return {W: r.width, H: r.height};
			})()`, &box).Do(ctx)
		}),
	)
	if err != nil {
		t.Fatalf("measure diagram: %v", err)
	}
	return box.W, box.H, overflows
}

const diagramSlide = "# Architecture\n\n```mermaid\ngraph TD\n  A --> B\n```\n"

func TestSlideDiagram_TallDiagramIsScaledToFit(t *testing.T) {
	w, h, overflows := diagramBox(t, slideDiagramPage(t, diagramSlide, 400, 1500))
	if len(overflows) != 0 {
		t.Errorf("slide overflows: %+v", overflows)
	}
	if h > 720 || h < 200 {
		t.Errorf("diagram drawn %.0fpx tall, want scaled down to the space under the heading", h)
	}
	if ratio := h / w; ratio < 3.6 || ratio > 3.9 {
		t.Errorf("aspect ratio %.2f, want the original 3.75", ratio)
	}
}

func TestSlideDiagram_WideDiagramIsScaledToFit(t *testing.T) {
	w, h, overflows := diagramBox(t, slideDiagramPage(t, diagramSlide, 3000, 600))
	if len(overflows) != 0 {
		t.Errorf("slide overflows: %+v", overflows)
	}
	if w > 1280 {
		t.Errorf("diagram drawn %.0fpx wide, want it within the slide", w)
	}
	if ratio := w / h; ratio < 4.8 || ratio > 5.2 {
		t.Errorf("aspect ratio %.2f, want the original 5", ratio)
	}
}

func TestSlideDiagram_SmallDiagramKeepsItsNaturalSize(t *testing.T) {
	w, h, overflows := diagramBox(t, slideDiagramPage(t, diagramSlide, 300, 200))
	if len(overflows) != 0 {
		t.Errorf("slide overflows: %+v", overflows)
	}
	if w < 299 || w > 301 || h < 199 || h > 201 {
		t.Errorf("diagram drawn %.0fx%.0f, want its natural 300x200", w, h)
	}
}

func TestSlideDiagram_TextAfterTheDiagramStaysOnTheSlide(t *testing.T) {
	_, _, overflows := diagramBox(t, slideDiagramPage(t, diagramSlide+"\nA caption under the diagram.\n", 400, 1500))
	if len(overflows) != 0 {
		t.Errorf("slide overflows: %+v", overflows)
	}
}

// TestSlideDiagram_NestedDiagramIsCappedAtTheSlideHeight covers a diagram in a
// list item or blockquote. It is not a flex item of the slide, so it cannot be
// given exactly the space left; it is held to the slide's content height
// instead, which keeps a tall one from running far past the edge.
func TestSlideDiagram_NestedDiagramIsCappedAtTheSlideHeight(t *testing.T) {
	for name, src := range map[string]string{
		"list item":  "- Steps\n\n  ```mermaid\n  graph TD\n    A --> B\n  ```\n",
		"blockquote": "> ```mermaid\n> graph TD\n>   A --> B\n> ```\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, h, _ := diagramBox(t, slideDiagramPage(t, src, 400, 1500))
			// 720px slide less 64px of padding top and bottom.
			if h > 592.5 {
				t.Errorf("nested diagram drawn %.0fpx tall, want at most the 592px content height", h)
			}
		})
	}
}
