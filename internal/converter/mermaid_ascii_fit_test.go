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

// TestFitMermaidLabels_RewritesOnlyLabelText is the core of the source rewrite:
// the text inside the brackets is wrapped, and everything structural — the node
// id, the brackets, the arrow, the indent — comes out byte-for-byte as written.
func TestFitMermaidLabels_RewritesOnlyLabelText(t *testing.T) {
	const source = "graph TB\n    A[AI Enablement Index Summary] --> B[Done]\n"
	const want = "graph TB\n    A[AI<br>Enablement<br>Index<br>Summary] --> B[Done]\n"
	if got := fitMermaidLabels(source, 12); got != want {
		t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, want)
	}
}

// TestFitMermaidLabels_RewritesEveryNodeOnALine covers an edge written on one
// line where both ends need wrapping, so the scan must not stop at the first
// label it finds.
func TestFitMermaidLabels_RewritesEveryNodeOnALine(t *testing.T) {
	const source = "graph LR\n  A[Alpha Beta Gamma] --> B[Delta Epsilon Zeta]\n"
	const want = "graph LR\n  A[Alpha<br>Beta<br>Gamma] --> B[Delta<br>Epsilon<br>Zeta]\n"
	if got := fitMermaidLabels(source, 8); got != want {
		t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, want)
	}
}

// TestFitMermaidLabels_HandlesParsedShapes covers the node shapes
// mermaid-ascii's own parseNode recognises. Rewriting only square brackets would
// leave the other shapes over-wide.
func TestFitMermaidLabels_HandlesParsedShapes(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"square", "graph LR\n  A[Alpha Beta Gamma]\n", "graph LR\n  A[Alpha<br>Beta<br>Gamma]\n"},
		{"rounded", "graph LR\n  A(Alpha Beta Gamma)\n", "graph LR\n  A(Alpha<br>Beta<br>Gamma)\n"},
		{"stadium", "graph LR\n  A[(Alpha Beta Gamma)]\n", "graph LR\n  A[(Alpha<br>Beta<br>Gamma)]\n"},
		{"hexagon", "graph LR\n  A{{Alpha Beta Gamma}}\n", "graph LR\n  A{{Alpha<br>Beta<br>Gamma}}\n"},
		{"rhombus", "graph LR\n  A{Alpha Beta Gamma}\n", "graph LR\n  A{Alpha<br>Beta<br>Gamma}\n"},
		// A shape the library cannot parse must be left alone rather than
		// rewritten into something it parses differently.
		{"nested brackets are left alone", "graph LR\n  A[Alpha Beta [Gamma]]\n", "graph LR\n  A[Alpha Beta [Gamma]]\n"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fitMermaidLabels(tc.source, 8); got != tc.want {
				t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestFitMermaidLabels_QuotedLabelKeepsItsBracket covers a label holding the
// closing delimiter inside quotes, which Mermaid allows. Treating that bracket
// as the end of the label would truncate the text.
func TestFitMermaidLabels_QuotedLabelKeepsItsBracket(t *testing.T) {
	const source = `graph LR` + "\n" + `  A["Alpha] Beta Gamma"] --> B[Done]` + "\n"
	const want = `graph LR` + "\n" + `  A["Alpha]<br>Beta<br>Gamma"] --> B[Done]` + "\n"
	if got := fitMermaidLabels(source, 8); got != want {
		t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, want)
	}
}

// TestFitMermaidLabels_LeavesNonNodeLinesAlone covers the statements that are
// not node definitions. A comment or a styling directive that happens to
// contain brackets must come out untouched, and subgraph labels are out of
// scope for this rewrite.
func TestFitMermaidLabels_LeavesNonNodeLinesAlone(t *testing.T) {
	tests := []struct {
		name string
		line string
	}{
		{"comment", "%% A[Alpha Beta Gamma] is only a note"},
		{"subgraph header", "  subgraph S[Alpha Beta Gamma]"},
		{"classDef", "  classDef primary fill:#f9f,stroke:#333"},
		{"style", "  style A fill:#f9f,stroke:#333"},
		{"click", `  click A "https://example.com/a/very/long/path"`},
		{"blank", "   "},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			source := "graph LR\n" + tc.line + "\n"
			if got := fitMermaidLabels(source, 8); got != source {
				t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, source)
			}
		})
	}
}

// TestFitMermaidLabels_LeavesFrontmatterAlone covers the YAML title/config block
// Mermaid allows above a diagram. It is data for the renderer, not diagram text,
// so a bracket inside it is not a node label. Leading blank lines and an
// unterminated block follow the renderer's own reading (diagram.StripFrontmatter).
func TestFitMermaidLabels_LeavesFrontmatterAlone(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "title block",
			source: "---\ntitle: A[Alpha Beta Gamma] board\n---\ngraph LR\n  A[Alpha Beta Gamma]\n",
			want:   "---\ntitle: A[Alpha Beta Gamma] board\n---\ngraph LR\n  A[Alpha<br>Beta<br>Gamma]\n",
		},
		{
			name:   "leading blank line before the block",
			source: "\n---\ntitle: A[Alpha Beta Gamma] board\n---\ngraph LR\n  A[Alpha Beta Gamma]\n",
			want:   "\n---\ntitle: A[Alpha Beta Gamma] board\n---\ngraph LR\n  A[Alpha<br>Beta<br>Gamma]\n",
		},
		{
			// Unterminated: the renderer keeps the whole input as diagram text,
			// so this rewrite must read it the same way.
			name:   "unterminated block is not frontmatter",
			source: "---\ngraph LR\n  A[Alpha Beta Gamma]\n",
			want:   "---\ngraph LR\n  A[Alpha<br>Beta<br>Gamma]\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := fitMermaidLabels(tc.source, 8); got != tc.want {
				t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, tc.want)
			}
		})
	}
}

// TestFitMermaidLabels_LeavesEdgeLabelsAlone pins that arrow labels are out of
// scope. They contribute little to a diagram's width and sit outside the shape
// delimiters this rewrite understands.
func TestFitMermaidLabels_LeavesEdgeLabelsAlone(t *testing.T) {
	const source = "graph LR\n  A[Alpha Beta Gamma] -->|a very long edge label| B\n"
	const want = "graph LR\n  A[Alpha<br>Beta<br>Gamma] -->|a very long edge label| B\n"
	if got := fitMermaidLabels(source, 8); got != want {
		t.Errorf("fitMermaidLabels()\n got: %q\nwant: %q", got, want)
	}
}

// TestMermaidArtWidth_MeasuresDisplayColumns pins that art is measured in
// terminal columns. mermaid-ascii's own displayWidth goes through go-runewidth,
// which counts a box-drawing "─" as two columns under an East Asian locale and
// so reports an inflated width; the fit loop must not inherit that reading.
func TestMermaidArtWidth_MeasuresDisplayColumns(t *testing.T) {
	tests := []struct {
		name string
		art  string
		want int
	}{
		{"empty", "", 0},
		{"latin", "abcd", 4},
		{"widest line wins", "ab\nabcdef\nabc", 6},
		{"CJK counts two columns", "開始", 4},
		{"box drawing counts one column", "┌────┐", 6},
		{"colour escapes occupy no columns", "\x1b[31mred\x1b[0m", 3},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := mermaidArtWidth(tc.art); got != tc.want {
				t.Errorf("mermaidArtWidth(%q) = %d, want %d", tc.art, got, tc.want)
			}
		})
	}
}

// TestRenderMermaidASCII_LeavesFittingDiagramsUntouched is the non-regression
// pin for fitting: a diagram already narrower than the wrap width must render
// exactly as it did before, with each label on one line.
func TestRenderMermaidASCII_LeavesFittingDiagramsUntouched(t *testing.T) {
	art, err := renderMermaidASCII("flowchart LR\n  A[Alpha Beta] --> B[Gamma Delta]\n", 80, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	for _, want := range []string{"Alpha Beta", "Gamma Delta"} {
		if !strings.Contains(art, want) {
			t.Errorf("label %q was wrapped even though the diagram already fits:\n%s", want, art)
		}
	}
	if w := mermaidArtWidth(art); w > 80 {
		t.Errorf("art is %d columns wide, want at most 80:\n%s", w, art)
	}
}

// TestRenderMermaidASCII_FitsAnOverWideDiagram is the reported defect. Four
// independent chains laid side by side came to 152 columns, and clipping to the
// wrap width dropped the fourth one entirely with no warning. Wrapping the
// labels has to bring the whole diagram inside the width instead.
func TestRenderMermaidASCII_FitsAnOverWideDiagram(t *testing.T) {
	const source = "graph TB\n" +
		"    A[メインダッシュボード<br>/] --> B[AI Enablement Index・利用サマリー・リスク・成果]\n" +
		"    E[セキュリティダッシュボード<br>/dashboard/security] --> F[リスク分析・アラート監視]\n" +
		"    C[ユースケースダッシュボード<br>/dashboard/use-case] --> D[ユースケース分析・カテゴリ分析]\n" +
		"    G[優先改善プロセスダッシュボード<br>/dashboard/priority-process] --> H[業務プロセス改善・利用促進]\n"
	const width = 118

	art, err := renderMermaidASCII(source, width, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	if w := mermaidArtWidth(art); w > width {
		t.Errorf("art is %d columns wide, want at most %d:\n%s", w, width, art)
	}
	// The paths are unbreakable tokens, so each surviving chain still shows its
	// own path in full. All four must be there.
	for _, want := range []string{"/dashboard/security", "/dashboard/use-case", "/dashboard/priority-process"} {
		if !strings.Contains(art, want) {
			t.Errorf("chain %q is missing from the art:\n%s", want, art)
		}
	}
	if boxes := strings.Count(strings.SplitN(art, "\n", 2)[0], "┌"); boxes != 4 {
		t.Errorf("top row has %d boxes, want 4:\n%s", boxes, art)
	}
}
