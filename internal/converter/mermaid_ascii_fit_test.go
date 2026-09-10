package converter

import "testing"

// TestWrapLabelText_LeavesShortLabelsAlone pins that wrapping is a no-op for a
// label that already fits. Fitting must not alter a diagram that renders
// correctly today.
func TestWrapLabelText_LeavesShortLabelsAlone(t *testing.T) {
	for _, text := range []string{"Start", "AI Index", "開始"} {
		if got := wrapLabelText(text, 20); got != text {
			t.Errorf("wrapLabelText(%q, 20) = %q, want it unchanged", text, got)
		}
	}
}
