package converter

import (
	"fmt"
	"strings"
)

// cssPixelsPerInch is the fixed ratio CSS defines between px and in. Slide
// sizes are given in CSS pixels, as Marp's are, and printed in inches.
const cssPixelsPerInch = 96

// slideSize is a named slide size preset in CSS pixels.
type slideSize struct {
	name     string
	widthPx  int
	heightPx int
}

// slideSizes are the presets the size front-matter key accepts, the ones every
// Marp built-in theme defines. The first is the default.
var slideSizes = []slideSize{
	{name: "16:9", widthPx: 1280, heightPx: 720},
	{name: "4:3", widthPx: 960, heightPx: 720},
}

// resolveSlideSize returns the preset named by the front-matter size key, or
// the default when there is none. An unknown or non-string value is an error
// listing the presets, since falling back to the default would print the deck
// at a size the author did not ask for.
func resolveSlideSize(meta frontMatter) (slideSize, error) {
	value, present := meta["size"]
	if !present {
		return slideSizes[0], nil
	}
	if name, ok := value.(string); ok {
		for _, s := range slideSizes {
			if s.name == strings.TrimSpace(name) {
				return s, nil
			}
		}
	}
	return slideSize{}, fmt.Errorf("unknown slide size %v: use one of %s", value, slideSizeNames())
}

// slideSizeNames lists the presets for messages, in declaration order.
func slideSizeNames() string {
	names := make([]string, len(slideSizes))
	for i, s := range slideSizes {
		names[i] = s.name
	}
	return strings.Join(names, ", ")
}

// slidePrintOptions is the page a deck prints on: exactly one slide, with no
// margin, so a slide can reach the edge of the page as it does on screen.
func slidePrintOptions(s slideSize) printOptions {
	return printOptions{
		widthInches:  float64(s.widthPx) / cssPixelsPerInch,
		heightInches: float64(s.heightPx) / cssPixelsPerInch,
	}
}

// slideSizeCSS fixes each slide to the page it prints on. Hiding overflow keeps
// a slide that is too full from spilling onto an extra page. The size is
// declared in @page too, so the HTML output printed straight from a browser
// gets slide-sized pages rather than the printer's paper; printPDF passes the
// same size through CDP.
func slideSizeCSS(s slideSize) string {
	return fmt.Sprintf(`
@page { size: %[1]dpx %[2]dpx; margin: 0; }
section.slide {
  width: %[1]dpx;
  height: %[2]dpx;
}
`, s.widthPx, s.heightPx)
}

// paperFlagsError reports document-only print flags given for a deck. They are
// rejected rather than ignored because a deck's page is its slide size.
func paperFlagsError(flags []string) error {
	verb := "applies"
	if len(flags) > 1 {
		verb = "apply"
	}
	return fmt.Errorf("%s %s only to documents: a slide deck prints at its slide size, "+
		"set with size: in the front-matter (%s)", strings.Join(flags, ", "), verb, slideSizeNames())
}
