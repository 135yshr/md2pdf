package converter

import (
	"strings"
	"testing"
)

// TestValidateMermaidScale covers the accepted range for -mermaid-scale. The
// bounds exist because either extreme stops being a diagram: too small and the
// boxes merge into a smudge, too large and almost none of it is on screen.
func TestValidateMermaidScale(t *testing.T) {
	tests := []struct {
		name    string
		scale   float64
		wantErr bool
	}{
		{"unset", 0, false},
		{"minimum", mermaidScaleMin, false},
		{"half", 0.5, false},
		{"natural", 1, false},
		{"double", 2, false},
		{"maximum", mermaidScaleMax, false},
		{"below the minimum", mermaidScaleMin / 2, true},
		{"above the maximum", mermaidScaleMax * 2, true},
		{"negative", -1, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateMermaidScale(tc.scale)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateMermaidScale(%v) error = %v, wantErr %t", tc.scale, err, tc.wantErr)
			}
			if err != nil && !strings.Contains(err.Error(), "mermaid-scale") {
				t.Errorf("error %q does not name the flag", err)
			}
		})
	}
}

// TestScaleColumnBudget covers how the scale turns the wrap width into the
// columns an image may occupy. An unset scale has to leave the budget exactly as
// it was, or every existing diagram would move.
func TestScaleColumnBudget(t *testing.T) {
	tests := []struct {
		name  string
		width int
		scale float64
		want  int
	}{
		{"unset keeps the width", 118, 0, 118},
		{"natural keeps the width", 118, 1, 118},
		{"half", 118, 0.5, 59},
		{"rounds to the nearest column", 101, 0.5, 51},
		{"double", 60, 2, 120},
		{"a tiny scale still leaves one column", 4, 0.1, 1},
		{"no width means no budget", 0, 0.5, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := scaleColumnBudget(tc.width, tc.scale); got != tc.want {
				t.Errorf("scaleColumnBudget(%d, %v) = %d, want %d", tc.width, tc.scale, got, tc.want)
			}
		})
	}
}
