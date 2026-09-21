package converter

import "fmt"

// slideOverflow reports that one slide's content does not fit the slide.
// Field names are exported only so the browser's JSON result decodes into it.
type slideOverflow struct {
	// Slide is the 1-based slide number.
	Slide int `json:"slide"`
	// Height is set when the content is taller than the slide.
	Height bool `json:"height"`
	// Width is set when the content is wider than the slide.
	Width bool `json:"width"`
}

// measureSlidesJS finds the slides whose content is larger than their box.
// A slide has overflow: hidden, so what does not fit is simply not drawn; the
// scroll size still counts it. The same goes for any element inside the slide
// that clips its own overflow — a code block does — and that element's
// overflow never reaches the slide's scroll size, so each clipping descendant
// is measured too. One pixel of slack absorbs sub-pixel rounding in layout,
// which would otherwise report boxes that fit exactly.
const measureSlidesJS = `(() => {
  const clips = (el) => {
    const cs = getComputedStyle(el);
    return cs.overflowX !== 'visible' || cs.overflowY !== 'visible';
  };
  const over = (el) => ({
    height: el.scrollHeight > el.clientHeight + 1,
    width: el.scrollWidth > el.clientWidth + 1,
  });
  return Array.from(document.querySelectorAll('section.slide')).map((s, i) => {
    const o = {slide: i + 1, ...over(s)};
    for (const el of s.querySelectorAll('*')) {
      if (!(el instanceof HTMLElement) || !clips(el)) continue;
      const d = over(el);
      o.height = o.height || d.height;
      o.width = o.width || d.width;
    }
    return o;
  }).filter(o => o.height || o.width);
})()`

// overflowWarnings turns measured overflows into the lines written to stderr.
// They are printed whether or not -v is set: the content past the edge is
// missing from the output, and nothing else would say so.
func overflowWarnings(input string, overflows []slideOverflow) []string {
	lines := make([]string, 0, len(overflows))
	for _, o := range overflows {
		var what string
		switch {
		case o.Height && o.Width:
			what = "height and width"
		case o.Height:
			what = "height"
		default:
			what = "width"
		}
		lines = append(lines, fmt.Sprintf("warning: %s: slide %d overflows the slide (%s); the rest is cut off",
			input, o.Slide, what))
	}
	return lines
}
