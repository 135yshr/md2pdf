package converter

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

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

// TestWrapLabelText_BreaksASCIIAtSpaces covers a Latin label wider than the cap:
// it must break between words rather than mid-word, so no word is mangled.
func TestWrapLabelText_BreaksASCIIAtSpaces(t *testing.T) {
	const (
		text     = "AI Enablement Index Summary"
		maxWidth = 12
	)
	got := wrapLabelText(text, maxWidth)
	lines := strings.Split(got, "<br>")
	if len(lines) < 2 {
		t.Fatalf("wrapLabelText(%q, %d) did not wrap: %q", text, maxWidth, got)
	}
	for i, line := range lines {
		if w := ansi.StringWidth(line); w > maxWidth {
			t.Errorf("line %d %q is %d columns wide, want at most %d", i, line, w, maxWidth)
		}
	}
	if joined := strings.Join(lines, " "); joined != text {
		t.Errorf("words were altered: %q reassembles to %q", got, joined)
	}
}
