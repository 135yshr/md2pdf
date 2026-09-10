package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
	"github.com/charmbracelet/x/ansi"
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

// TestSpliceConsoleDiagrams_NonPannedArtNeverExceedsTheWidth pins the invariant
// that made clipping unnecessary. Art not being panned has already been fitted
// inside the wrap width, and the splice gives up the indent before it gives up
// columns, so a spliced line cannot overrun. Panned art is the deliberate
// exception.
func TestSpliceConsoleDiagrams_NonPannedArtNeverExceedsTheWidth(t *testing.T) {
	const width = 20
	tests := []struct {
		name      string
		art       string
		pan       bool
		wantWider bool
	}{
		{"art narrower than the width keeps its indent", strings.Repeat("X", 10), false, false},
		{"art exactly the width gives up the indent", strings.Repeat("X", width), false, false},
		{"panned art is allowed to overrun", strings.Repeat("X", 60), true, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, missing := spliceConsoleDiagrams("  MD2PDFDG0\n", []consoleDiagram{
				{placeholder: "MD2PDFDG0", content: tc.art, kind: diagramTextArt, pan: tc.pan},
			}, width)
			if len(missing) != 0 {
				t.Fatalf("unplaced diagrams: %v", missing)
			}
			if !strings.Contains(out, tc.art) {
				t.Errorf("art was altered by the splice: %q", out)
			}
			wider := false
			for _, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
				if ansi.StringWidth(line) > width {
					wider = true
				}
			}
			if wider != tc.wantWider {
				t.Errorf("spliced output wider than %d = %t, want %t: %q", width, wider, tc.wantWider, out)
			}
		})
	}
}

// TestPagerArgvFor covers the pager command line a panned document needs: lines
// chopped rather than folded, and -F given up so less stays interactive long
// enough to scroll.
func TestPagerArgvFor(t *testing.T) {
	tests := []struct {
		name   string
		argv   []string
		panned bool
		want   []string
	}{
		{"no panning leaves the default alone", []string{"less", "-R", "-F"}, false, []string{"less", "-R", "-F"}},
		{"panning chops lines and cancels -F", []string{"less", "-R", "-F"}, true, []string{"less", "-R", "-S", "-+F"}},
		// -S is appended, never detected. less takes a repeated boolean option
		// as "on", and inspecting the existing flags cannot be done safely: the
		// S in -PSTATUS is prompt text, not a cluster of boolean options.
		{"an existing -S is simply re-set", []string{"less", "-S"}, true, []string{"less", "-S", "-S", "-+F"}},
		{"a prompt argument is never read as flags", []string{"less", "-PSTATUS"}, true, []string{"less", "-PSTATUS", "-S", "-+F"}},
		// -F can also arrive through $LESS, which no argv rewrite can reach, so
		// it is cancelled with -+F rather than carved out of a cluster.
		{"-F inside a cluster is cancelled, not carved out", []string{"less", "-RF"}, true, []string{"less", "-RF", "-S", "-+F"}},
		{"panning through a non-less pager is left alone", []string{"more"}, true, []string{"more"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := pagerArgvFor(append([]string(nil), tc.argv...), tc.panned)
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("pagerArgvFor(%v, %t) = %v, want %v", tc.argv, tc.panned, got, tc.want)
			}
		})
	}
}

// TestRenderConsoleDocument_PansTheReportedDiagramEndToEnd walks the whole
// console path for a diagram no label cap can fit, through the same function
// renderConsole calls. Every chain has to reach the rendered document, the run
// has to report that it panned so the pager stops folding, and with nowhere to
// scroll the same document must fall back to the Mermaid source instead.
func TestRenderConsoleDocument_PansTheReportedDiagramEndToEnd(t *testing.T) {
	const width = 40
	md := []byte("## Structure\n\n```mermaid\n" + overWideDashboardDiagram + "```\n")
	chains := []string{"/dashboard/security", "/dashboard/use-case", "/dashboard/priority-process"}

	t.Run("output that can pan gets the whole diagram", func(t *testing.T) {
		c := newTestConverter(t, &Config{Format: FormatConsole})
		c.mermaidAvailable = func() bool { return false }

		plan := consoleMermaidPlan{mode: MermaidRenderASCII, canPan: true}
		rendered, drawn, err := c.renderConsoleDocument(md, styles.NoTTYStyle, width, plan)
		if err != nil {
			t.Fatalf("renderConsoleDocument: %v", err)
		}
		if !drawn.panned {
			t.Error("the run did not report panning, so the pager would fold the diagram")
		}
		if drawn.images {
			t.Error("text art reported itself as an image")
		}
		for _, want := range chains {
			if !strings.Contains(string(rendered), want) {
				t.Errorf("chain %q is missing from the rendered document:\n%s", want, rendered)
			}
		}
	})

	t.Run("output that cannot pan gets the source", func(t *testing.T) {
		c := newTestConverter(t, &Config{Format: FormatConsole})
		c.mermaidAvailable = func() bool { return false }

		plan := consoleMermaidPlan{mode: MermaidRenderASCII, canPan: false}
		rendered, drawn, err := c.renderConsoleDocument(md, styles.NoTTYStyle, width, plan)
		if err != nil {
			t.Fatalf("renderConsoleDocument: %v", err)
		}
		if drawn.panned {
			t.Error("a source fallback reported itself as panned")
		}
		// The source keeps the arrows the art would have drawn as boxes.
		if !strings.Contains(string(rendered), "-->") {
			t.Errorf("the block did not fall back to its Mermaid source:\n%s", rendered)
		}
	})
}

// TestPrepareConsoleMermaid_DoesNotPanWhenImagesMaySkipThePager covers a mixed
// run on an image-capable terminal: one diagram rasterises, another falls back
// to text art too wide for the width.
//
// Any image at all makes renderConsole skip the pager, so the art would be
// written straight to the terminal and folded into fragments — panning is not
// actually available on that path. The source is, so that is what the block
// gets. The decision cannot wait for the outcome: the pager is chosen after
// every diagram has been drawn.
func TestPrepareConsoleMermaid_DoesNotPanWhenImagesMaySkipThePager(t *testing.T) {
	const width = 40
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n\nmid\n\n```mermaid\n" +
		overWideDashboardDiagram + "```\n")

	dir := t.TempDir()
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return true }
	c.rasterizeMermaid = func(idx int, _ string) (string, error) {
		if idx != 0 {
			return "", errStubRasterFailure
		}
		path := filepath.Join(dir, "diagram.png")
		if err := os.WriteFile(path, stubPNG(t, 100, 100), 0o644); err != nil {
			return "", err
		}
		return path, nil
	}

	plan := consoleMermaidPlan{
		mode:      MermaidRenderImage,
		transport: terminalImageTransport{protocol: imageProtocolKitty},
		canPan:    true,
	}
	rewritten, diagrams, err := c.prepareConsoleMermaid(md, plan, width)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if anyPanDiagram(diagrams) {
		t.Error("a diagram was marked for panning on a run that skips the pager")
	}
	if !anyImageDiagram(diagrams) {
		t.Fatal("the first diagram did not come out as an image; this test needs it to")
	}
	if !strings.Contains(string(rewritten), "優先改善プロセスダッシュボード") {
		t.Errorf("the over-wide block did not fall back to its Mermaid source:\n%s", rewritten)
	}
}

// TestPanBlockedReason pins both conditions panning depends on, and that the
// image one wins the explanation: it is the surprising half, since the pager is
// available and still will not be used.
func TestPanBlockedReason(t *testing.T) {
	tests := []struct {
		name          string
		canPan        bool
		mayDrawImages bool
		wantAllowed   bool
		wantReason    string
	}{
		{"can scroll, no images", true, false, true, ""},
		{"cannot scroll", false, false, false, "cannot scroll sideways"},
		{"images bypass the pager", true, true, false, "bypass the pager"},
		{"neither", false, true, false, "bypass the pager"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			reason, allowed := panBlockedReason(tc.canPan, tc.mayDrawImages)
			if allowed != tc.wantAllowed {
				t.Errorf("allowed = %t, want %t", allowed, tc.wantAllowed)
			}
			if !strings.Contains(reason, tc.wantReason) {
				t.Errorf("reason = %q, want it to mention %q", reason, tc.wantReason)
			}
		})
	}
}
