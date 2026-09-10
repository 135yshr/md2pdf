package converter

import (
	"bytes"
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"charm.land/glamour/v2/styles"
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
// survives, and every <br> spelling mermaid-ascii accepts is recognized.
func TestWrapLabelText_RespectsAuthoredBreaks(t *testing.T) {
	for _, br := range []string{"<br>", "<br/>", "<br />", "<BR>"} {
		t.Run(br, func(t *testing.T) {
			got := wrapLabelText("メインダッシュボード"+br+"/dashboard/main", 12)
			lines := strings.Split(got, labelBreak)
			if !slices.Contains(lines, "/dashboard/main") {
				t.Errorf("authored break at %q was not honored: %q", br, got)
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
// mermaid-ascii's own parseNode recognizes. Rewriting only square brackets would
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
// terminal columns. The width mermaid-ascii reports goes through go-runewidth,
// which counts a box-drawing "─" as two columns under an East Asian locale and
// so is inflated; the fit loop must not inherit that reading.
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
		{"color escapes occupy no columns", "\x1b[31mred\x1b[0m", 3},
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

// overWideDashboardDiagram is the diagram from the bug report: four independent
// chains that mermaid-ascii lays side by side, coming to 152 columns.
const overWideDashboardDiagram = "graph TB\n" +
	"    A[メインダッシュボード<br>/] --> B[AI Enablement Index・利用サマリー・リスク・成果]\n" +
	"    E[セキュリティダッシュボード<br>/dashboard/security] --> F[リスク分析・アラート監視]\n" +
	"    C[ユースケースダッシュボード<br>/dashboard/use-case] --> D[ユースケース分析・カテゴリ分析]\n" +
	"    G[優先改善プロセスダッシュボード<br>/dashboard/priority-process] --> H[業務プロセス改善・利用促進]\n"

// TestRenderMermaidASCII_FitsAnOverWideDiagram is the reported defect. Four
// independent chains laid side by side came to 152 columns, and clipping to the
// wrap width dropped the fourth one entirely with no warning. Wrapping the
// labels has to bring the whole diagram inside the width instead.
func TestRenderMermaidASCII_FitsAnOverWideDiagram(t *testing.T) {
	const width = 118

	art, err := renderMermaidASCII(overWideDashboardDiagram, width, false)
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

// TestSpliceConsoleDiagrams_HangsWideArtIntoTheMargin covers art fitted to the
// full wrap width. Glamour indents the document, so keeping that indent would
// cost the diagram its rightmost columns — exactly the clipping fitting set out
// to avoid. Text art is a figure, not a paragraph: it may hang into the margin.
func TestSpliceConsoleDiagrams_HangsWideArtIntoTheMargin(t *testing.T) {
	const width = 20
	art := strings.Repeat("X", width)
	out, missing := spliceConsoleDiagrams("  MD2PDFDG0\n", []consoleDiagram{
		{placeholder: "MD2PDFDG0", content: art, kind: diagramTextArt},
	}, width)
	if len(missing) != 0 {
		t.Fatalf("unplaced diagrams: %v", missing)
	}
	if !strings.Contains(out, art) {
		t.Errorf("art that exactly fits the wrap width was clipped: %q", out)
	}
	for i, line := range strings.Split(strings.TrimRight(out, "\n"), "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("line %d is %d columns wide, want at most %d: %q", i, w, width, line)
		}
	}
}

// TestRenderMermaidASCII_ReturnsNarrowestArtWhenNothingFits covers a diagram no
// label cap can bring inside the width. Fitting must still hand back the most
// compact art it managed rather than failing: a diagram too wide to fit is not
// an error, and the caller can still clip it.
func TestRenderMermaidASCII_ReturnsNarrowestArtWhenNothingFits(t *testing.T) {
	var b strings.Builder
	b.WriteString("graph TB\n")
	for i := range 8 {
		fmt.Fprintf(&b, "  N%d[Node %d with a fairly long label] --> M%d[Target %d]\n", i, i, i, i)
	}
	source := b.String()
	const width = 30

	art, err := renderMermaidASCII(source, width, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	unfitted, err := renderMermaidArt(source, width, false)
	if err != nil {
		t.Fatalf("renderMermaidArt: %v", err)
	}
	if mermaidArtWidth(art) > width {
		t.Logf("still %d columns wide, which this diagram cannot avoid", mermaidArtWidth(art))
	}
	if mermaidArtWidth(art) >= mermaidArtWidth(unfitted) {
		t.Errorf("art was not compacted at all: %d columns, unfitted is %d",
			mermaidArtWidth(art), mermaidArtWidth(unfitted))
	}
}

// TestDiagramSupportsLabelFitting pins which diagram types may have their labels
// rewritten. A sequence diagram has no bracketed node labels, so a rewrite there
// could only corrupt the source.
func TestDiagramSupportsLabelFitting(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{"graph", "graph TB\n  A --> B\n", true},
		{"flowchart", "flowchart LR\n  A --> B\n", true},
		{"bare graph", "graph\n  A --> B\n", true},
		{"frontmatter over a graph", "---\ntitle: T\n---\ngraph LR\n  A --> B\n", true},
		{"sequence diagram", "sequenceDiagram\n  A->>B: hi\n", false},
		{"empty", "", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := diagramSupportsLabelFitting(tc.source); got != tc.want {
				t.Errorf("diagramSupportsLabelFitting(%q) = %t, want %t", tc.source, got, tc.want)
			}
		})
	}
}

// TestRenderMermaidASCII_DoesNotFitSequenceDiagrams checks the gate end to end:
// a sequence diagram wider than the width comes out exactly as the renderer drew
// it, with no rewrite attempted.
func TestRenderMermaidASCII_DoesNotFitSequenceDiagrams(t *testing.T) {
	const source = "sequenceDiagram\n" +
		"  participant A as A participant with a very long name\n" +
		"  participant B as Another participant with a very long name\n" +
		"  A->>B: a message\n"
	const width = 40

	art, err := renderMermaidASCII(source, width, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	unfitted, err := renderMermaidArt(source, width, false)
	if err != nil {
		t.Fatalf("renderMermaidArt: %v", err)
	}
	if art != unfitted {
		t.Errorf("sequence diagram art was altered by fitting:\n%s", art)
	}
}

// TestFitMermaidArt_KeepsWorkingArtWhenNothingToWrap covers the guarantee that
// fitting never costs a diagram: with every label already inside the narrowest
// cap there is no candidate to render, so the art handed in comes back as it is.
func TestFitMermaidArt_KeepsWorkingArtWhenNothingToWrap(t *testing.T) {
	const source = "graph TB\n  A[a] --> B[b]\n"
	const art = "sentinel art that is far wider than the width it is given"
	if got := fitMermaidArt(source, art, 10, false); got != art {
		t.Errorf("fitMermaidArt() = %q, want the art it was given", got)
	}
}

// TestRenderMermaidASCII_FitsWithPureASCII covers -style ascii: a terminal that
// cannot show box-drawing characters still gets the whole diagram.
func TestRenderMermaidASCII_FitsWithPureASCII(t *testing.T) {
	const width = 118

	art, err := renderMermaidASCII(overWideDashboardDiagram, width, true)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	if w := mermaidArtWidth(art); w > width {
		t.Errorf("art is %d columns wide, want at most %d:\n%s", w, width, art)
	}
	if !strings.Contains(art, "/dashboard/priority-process") {
		t.Errorf("the fourth chain is missing from the art:\n%s", art)
	}
	if strings.ContainsAny(art, "┌┐└┘─│►") {
		t.Errorf("box-drawing characters present despite pure ASCII being requested:\n%s", art)
	}
}

// TestPrepareConsoleMermaid_FitsTheReportedDocument walks the console path end to
// end for the document from the bug report, through the same functions
// renderConsoleDocument uses. Every chain has to survive into the spliced output,
// and no line may overrun the wrap width.
func TestPrepareConsoleMermaid_FitsTheReportedDocument(t *testing.T) {
	const width = 118
	md := []byte("## ダッシュボード構成\n\n```mermaid\n" + overWideDashboardDiagram + "```\n")

	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }
	c.rasterizeMermaid = func(context.Context, int, string) (string, error) {
		t.Fatal("rasterizer must not run in ascii mode")
		return "", nil
	}

	doc, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, consoleMermaidPlan{mode: MermaidRenderASCII}, width)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1", len(diagrams))
	}

	rendered, err := renderConsoleMarkdown(doc, styles.NoTTYStyle, width)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}
	spliced, missing := spliceConsoleDiagrams(string(rendered), diagrams, width)
	if len(missing) != 0 {
		t.Fatalf("unplaced diagrams: %v", missing)
	}

	for _, want := range []string{"/dashboard/security", "/dashboard/use-case", "/dashboard/priority-process"} {
		if !strings.Contains(spliced, want) {
			t.Errorf("chain %q is missing from the rendered document:\n%s", want, spliced)
		}
	}
	for i, line := range strings.Split(spliced, "\n") {
		if w := ansi.StringWidth(line); w > width {
			t.Errorf("line %d is %d columns wide, want at most %d: %q", i, w, width, line)
		}
	}
}

// TestClippedArtWarning covers the end of the silent loss. A diagram no label
// cap could fit still loses its right edge to clipping, and the log has to say
// so — naming the width it needed and the two ways out — instead of leaving a
// missing branch with nothing in the output to show for it.
func TestClippedArtWarning(t *testing.T) {
	tests := []struct {
		name     string
		artWidth int
		width    int
		want     bool
	}{
		{"narrower than the width", 40, 80, false},
		{"exactly the width", 80, 80, false},
		{"no width limit set", 200, 0, false},
		{"overruns the width", 152, 118, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := clippedArtWarning(0, tc.artWidth, tc.width)
			if (got != "") != tc.want {
				t.Errorf("clippedArtWarning(0, %d, %d) = %q, want warning=%t",
					tc.artWidth, tc.width, got, tc.want)
			}
		})
	}

	msg := clippedArtWarning(2, 152, 118)
	for _, want := range []string{"2", "152", "118", "-width", "-mermaid-render source"} {
		if !strings.Contains(msg, want) {
			t.Errorf("warning %q does not mention %q", msg, want)
		}
	}
}

// TestFitMermaidLabels_LeavesQuotedEdgeLabelsAlone covers a bracket inside a
// quoted edge label. The characters before it look like a node id, but the
// bracket is text the author quoted, not a shape delimiter, so rewriting it
// would change the diagram instead of narrowing it.
func TestFitMermaidLabels_LeavesQuotedEdgeLabelsAlone(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "bracket inside a quoted edge label",
			source: "graph LR\n  A -- \"read [the long documentation]\" --> B\n",
			want:   "graph LR\n  A -- \"read [the long documentation]\" --> B\n",
		},
		{
			// A quoted node label still has its delimiter outside the quotes,
			// so it must keep being rewritten.
			name:   "quoted node label is still rewritten",
			source: "graph LR\n  A[\"Alpha Beta Gamma\"] --> B\n",
			want:   "graph LR\n  A[\"Alpha<br>Beta<br>Gamma\"] --> B\n",
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

// TestPrepareConsoleMermaid_WarnsAboutClippingWithoutVerbose is the point of the
// warning. A diagram that still loses its right edge has to say so even with -v
// off: routing it through the verbose log would leave the loss exactly as silent
// as the clipping this change set out to replace.
func TestPrepareConsoleMermaid_WarnsAboutClippingWithoutVerbose(t *testing.T) {
	tests := []struct {
		name     string
		width    int
		wantWarn bool
	}{
		// 40 columns cannot hold four chains at any label cap.
		{"art that cannot be fitted warns", 40, true},
		{"art that fits stays quiet", 118, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var stderr bytes.Buffer
			c := newTestConverter(t, &Config{Format: FormatConsole})
			c.stderr = &stderr
			c.mermaidAvailable = func() bool { return false }

			md := []byte("```mermaid\n" + overWideDashboardDiagram + "```\n")
			_, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, consoleMermaidPlan{mode: MermaidRenderASCII}, tc.width)
			if err != nil {
				t.Fatalf("prepareConsoleMermaid: %v", err)
			}
			if len(diagrams) != 1 {
				t.Fatalf("got %d diagrams, want 1", len(diagrams))
			}
			if c.cfg.Verbose {
				t.Fatal("this test only means something with verbose logging off")
			}

			logged := stderr.String()
			if warned := strings.Contains(logged, "clipped"); warned != tc.wantWarn {
				t.Errorf("stderr = %q, want a clipping warning=%t", logged, tc.wantWarn)
			}
		})
	}
}

// TestFitMermaidLabels_LeavesPipeEdgeLabelsAlone covers the other way an edge
// label can hold a bracket: unquoted, between pipes. The characters before the
// bracket still look like a node id, so the pipe span has to be stepped over the
// same way a quoted one is.
func TestFitMermaidLabels_LeavesPipeEdgeLabelsAlone(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{
			name:   "bracket inside a pipe edge label",
			source: "graph LR\n  A -->|read [the long documentation]| B\n",
			want:   "graph LR\n  A -->|read [the long documentation]| B\n",
		},
		{
			name:   "node labels around a pipe edge label are still rewritten",
			source: "graph LR\n  A[Alpha Beta Gamma] -->|read [the docs]| B[Delta Epsilon]\n",
			want:   "graph LR\n  A[Alpha<br>Beta<br>Gamma] -->|read [the docs]| B[Delta<br>Epsilon]\n",
		},
		{
			// A lone pipe is not a span; swallowing the rest of the line would
			// lose every node after it.
			name:   "an unmatched pipe does not swallow the line",
			source: "graph LR\n  A[Alpha Beta Gamma] --> B | C\n",
			want:   "graph LR\n  A[Alpha<br>Beta<br>Gamma] --> B | C\n",
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
