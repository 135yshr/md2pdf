package converter

import (
	"strings"
	"testing"
)

// TestPagerCanPan pins which pagers can scroll horizontally. Only less is
// known to chop long lines and scroll right; anything else would fold or
// truncate an over-wide diagram with no way to reach its right edge.
func TestPagerCanPan(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want bool
	}{
		{"less", []string{"less", "-R", "-F"}, true},
		{"absolute less", []string{"/usr/bin/less", "-R"}, true},
		{"more", []string{"more"}, false},
		{"cat", []string{"/bin/cat"}, false},
		{"bat", []string{"bat", "--paging=always"}, false},
		{"empty", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pagerCanPan(tc.argv); got != tc.want {
				t.Errorf("pagerCanPan(%v) = %t, want %t", tc.argv, got, tc.want)
			}
		})
	}
}

// TestEnsureLessNoWrap covers the -S that turns folding into horizontal
// scrolling. Without it less wraps an over-wide diagram into fragments, which
// is the state fitting set out to avoid.
func TestEnsureLessNoWrap(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"adds -S", []string{"less", "-R"}, []string{"less", "-R", "-S"}},
		{"keeps an existing -S", []string{"less", "-S"}, []string{"less", "-S"}},
		{"keeps -S in a cluster", []string{"less", "-RS"}, []string{"less", "-RS"}},
		{"keeps --chop-long-lines", []string{"less", "--chop-long-lines"}, []string{"less", "--chop-long-lines"}},
		// -s squeezes blank lines and is not -S; the check must be case
		// sensitive or an over-wide diagram would still fold.
		{"does not mistake -s for -S", []string{"less", "-s"}, []string{"less", "-s", "-S"}},
		{"leaves other pagers alone", []string{"more"}, []string{"more"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureLessNoWrap(append([]string(nil), tc.argv...))
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("ensureLessNoWrap(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}

// TestStripQuitIfOneScreen covers -F, which makes less print the file and exit
// when it fits on one screen. That leaves the terminal to fold the wide lines
// with no chance to scroll, so panning has to give it up.
func TestStripQuitIfOneScreen(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"drops a standalone -F", []string{"less", "-R", "-F"}, []string{"less", "-R"}},
		{"drops --quit-if-one-screen", []string{"less", "--quit-if-one-screen"}, []string{"less"}},
		{"drops F from a cluster", []string{"less", "-RF"}, []string{"less", "-R"}},
		{"drops a cluster that was only F", []string{"less", "-F", "-R"}, []string{"less", "-R"}},
		{"leaves argv without -F alone", []string{"less", "-R", "-S"}, []string{"less", "-R", "-S"}},
		{"leaves other pagers alone", []string{"more", "-F"}, []string{"more", "-F"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripQuitIfOneScreen(append([]string(nil), tc.argv...))
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("stripQuitIfOneScreen(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}

// TestResolveConsolePan covers the decision that replaces clipping: whether the
// rendered document reaches the reader through something that can show a line
// wider than the screen.
func TestResolveConsolePan(t *testing.T) {
	less := []string{"/usr/bin/less", "-R", "-F"}
	tests := []struct {
		name           string
		pagerEnabled   bool
		isTTY          bool
		argv           []string
		pagerAvailable bool
		want           bool
	}{
		// Redirected output has no terminal to fold the lines, so the whole
		// diagram can be written and whatever consumes it decides what to do.
		{"redirected", true, false, nil, false, true},
		{"redirected with the pager off", false, false, nil, false, true},
		{"terminal paged through less", true, true, less, true, true},
		{"terminal with the pager off", false, true, less, true, false},
		{"terminal paged through more", true, true, []string{"/usr/bin/more"}, true, false},
		{"terminal with no pager installed", true, true, nil, false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveConsolePan(tc.pagerEnabled, tc.isTTY, tc.argv, tc.pagerAvailable)
			if got != tc.want {
				t.Errorf("resolveConsolePan(%t, %t, %v, %t) = %t, want %t",
					tc.pagerEnabled, tc.isTTY, tc.argv, tc.pagerAvailable, got, tc.want)
			}
		})
	}
}

// TestPrepareConsoleMermaid_OverWideArtPansWhenTheOutputCan is the Phase 2
// policy. A diagram no label cap can fit is written at its full width and
// marked for panning, so the reader scrolls to the right edge instead of losing
// it.
func TestPrepareConsoleMermaid_OverWideArtPansWhenTheOutputCan(t *testing.T) {
	const width = 40 // four chains cannot fit here at any label cap
	md := []byte("```mermaid\n" + overWideDashboardDiagram + "```\n")

	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }

	plan := consoleMermaidPlan{mode: MermaidRenderASCII, canPan: true}
	_, diagrams, err := c.prepareConsoleMermaid(md, plan, width)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1", len(diagrams))
	}
	if !diagrams[0].pan {
		t.Error("over-wide art was not marked for panning")
	}
	if w := mermaidArtWidth(diagrams[0].content); w <= width {
		t.Errorf("art is %d columns wide; this test needs art wider than %d", w, width)
	}
	if !strings.Contains(diagrams[0].content, "/dashboard/priority-process") {
		t.Errorf("the fourth chain is missing:\n%s", diagrams[0].content)
	}
}

// TestPrepareConsoleMermaid_OverWideArtFallsBackToSourceWhenItCannotPan is the
// other half of the policy. With nowhere to scroll, the Mermaid source is more
// use than a diagram with its right edge cut off.
func TestPrepareConsoleMermaid_OverWideArtFallsBackToSourceWhenItCannotPan(t *testing.T) {
	const width = 40
	md := []byte("```mermaid\n" + overWideDashboardDiagram + "```\n")

	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }

	plan := consoleMermaidPlan{mode: MermaidRenderASCII, canPan: false}
	rewritten, diagrams, err := c.prepareConsoleMermaid(md, plan, width)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 0 {
		t.Fatalf("got %d diagrams, want 0 (the block keeps its source)", len(diagrams))
	}
	if !strings.Contains(string(rewritten), "優先改善プロセスダッシュボード") {
		t.Errorf("the block did not fall back to its Mermaid source:\n%s", rewritten)
	}
}

// TestPrepareConsoleMermaid_FittingArtNeverPans is the non-regression pin: a
// diagram that fits is drawn the same way whether or not the output could pan.
func TestPrepareConsoleMermaid_FittingArtNeverPans(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A[Start] --> B[End]\n```\n")

	for _, canPan := range []bool{true, false} {
		c := newTestConverter(t, &Config{Format: FormatConsole})
		c.mermaidAvailable = func() bool { return false }

		plan := consoleMermaidPlan{mode: MermaidRenderASCII, canPan: canPan}
		_, diagrams, err := c.prepareConsoleMermaid(md, plan, 80)
		if err != nil {
			t.Fatalf("canPan=%t: prepareConsoleMermaid: %v", canPan, err)
		}
		if len(diagrams) != 1 {
			t.Fatalf("canPan=%t: got %d diagrams, want 1", canPan, len(diagrams))
		}
		if diagrams[0].pan {
			t.Errorf("canPan=%t: art that fits was marked for panning", canPan)
		}
	}
}

// TestAnyPanDiagram covers the aggregate the pager needs: one panned diagram
// anywhere in the run means the pager has to stop folding lines.
func TestAnyPanDiagram(t *testing.T) {
	tests := []struct {
		name     string
		diagrams []consoleDiagram
		want     bool
	}{
		{"none", nil, false},
		{"text art that fits", []consoleDiagram{{kind: diagramTextArt}}, false},
		{"one panned", []consoleDiagram{{kind: diagramTextArt, pan: true}}, true},
		{"mixed", []consoleDiagram{{kind: diagramTextArt}, {kind: diagramTextArt, pan: true}}, true},
		{"images do not pan", []consoleDiagram{{kind: diagramImage}}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyPanDiagram(tc.diagrams); got != tc.want {
				t.Errorf("anyPanDiagram() = %t, want %t", got, tc.want)
			}
		})
	}
}
