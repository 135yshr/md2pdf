package converter

import (
	"slices"
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

// TestWrapLabelText_BreaksCJKAnywhere covers a Japanese label with no spaces to
// break at. Mermaid's own renderer breaks CJK between characters, and measuring
// columns rather than bytes is what keeps the resulting box aligned.
func TestWrapLabelText_BreaksCJKAnywhere(t *testing.T) {
	const (
		text     = "優先改善プロセスダッシュボード"
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
	if joined := strings.Join(lines, ""); joined != text {
		t.Errorf("characters were altered: %q reassembles to %q", got, joined)
	}
}

// TestWrapLabelText_KeepsUnbreakableTokenWhole covers a path longer than the cap.
// Breaking it would make it unreadable, so its line is allowed to overflow: the
// diagram comes out wider than asked for rather than corrupted.
func TestWrapLabelText_KeepsUnbreakableTokenWhole(t *testing.T) {
	const path = "/dashboard/priority-process"
	got := wrapLabelText("Priority "+path, 12)
	lines := strings.Split(got, "<br>")
	if !slices.Contains(lines, path) {
		t.Errorf("wrapLabelText broke the unbreakable token: %q, lines %q", got, lines)
	}
}

// TestWrapLabelText_RespectsAuthoredBreaks covers a label the author already
// broke by hand. Each segment is wrapped on its own so the authored boundary
// survives, and every <br> spelling mermaid-ascii accepts is recognised.
func TestWrapLabelText_RespectsAuthoredBreaks(t *testing.T) {
	for _, br := range []string{"<br>", "<br/>", "<br />", "<BR>"} {
		t.Run(br, func(t *testing.T) {
			got := wrapLabelText("メインダッシュボード"+br+"/dashboard/main", 12)
			lines := strings.Split(got, labelBreak)
			if !slices.Contains(lines, "/dashboard/main") {
				t.Errorf("authored break at %q was not honoured: %q", br, got)
			}
			for _, line := range lines {
				if strings.ContainsAny(line, "<>") {
					t.Errorf("a break tag leaked into the text: %q", got)
				}
			}
		})
	}
}

// TestWrapLabelText_Degenerate covers the inputs that must never be touched:
// an unset cap, a nonsensical one and an empty label.
func TestWrapLabelText_Degenerate(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		maxWidth int
	}{
		{"zero cap", "優先改善プロセスダッシュボード", 0},
		{"negative cap", "優先改善プロセスダッシュボード", -5},
		{"empty label", "", 12},
		{"empty label, zero cap", "", 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := wrapLabelText(tc.text, tc.maxWidth); got != tc.text {
				t.Errorf("wrapLabelText(%q, %d) = %q, want it unchanged", tc.text, tc.maxWidth, got)
			}
		})
	}
}
