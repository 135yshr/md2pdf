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
	if scale < mermaidScaleMin || scale > mermaidScaleMax {
		return fmt.Errorf("-mermaid-scale must be between %g and %g, got %g",
			mermaidScaleMin, mermaidScaleMax, scale)
	}
	return nil
}

// scaleColumnBudget turns the wrap width into the number of columns an inline
// image may occupy.
//
// An unset scale returns the width untouched, so every diagram drawn before this
// flag existed keeps its size. A scaled budget is at least one column, because a
// budget of zero would mean "no limit" to fitColumns and quietly undo the
// shrinking that was asked for.
func scaleColumnBudget(width int, scale float64) int {
	if scale == 0 || scale == 1 || width <= 0 {
		return width
	}
	return max(1, int(math.Round(float64(width)*scale)))
}
