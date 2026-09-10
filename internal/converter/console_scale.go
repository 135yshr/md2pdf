package converter

import (
	"fmt"
	"math"
)

// Bounds for -mermaid-scale. Below the minimum a diagram's boxes merge into a
// smudge; above the maximum almost none of it is on screen, since an inline
// image cannot be scrolled the way text art can.
const (
	mermaidScaleMin = 0.1
	mermaidScaleMax = 4.0
)

// ValidateMermaidScale reports whether scale is a usable -mermaid-scale factor.
// Zero selects the default, which is to draw a diagram at its natural size
// within the wrap width.
func ValidateMermaidScale(scale float64) error {
	if scale == 0 {
		return nil
	}
	// NaN has to be rejected on its own: every ordered comparison against it is
	// false, so the range check below would pass it through, and math.Round
	// would then convert it to an implementation-defined integer instead of
	// anything reporting a problem. The flag parser accepts the spelling.
	if math.IsNaN(scale) || math.IsInf(scale, 0) {
		return fmt.Errorf("-mermaid-scale must be a finite number between %g and %g, got %g",
			mermaidScaleMin, mermaidScaleMax, scale)
	}
	if scale < mermaidScaleMin || scale > mermaidScaleMax {
		return fmt.Errorf("-mermaid-scale must be between %g and %g, got %g",
			mermaidScaleMin, mermaidScaleMax, scale)
	}
	return nil
}
