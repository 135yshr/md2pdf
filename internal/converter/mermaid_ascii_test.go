package converter

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// errStubRasterFailure stands in for an mmdc invocation that fails.
var errStubRasterFailure = errors.New("stub rasterizer failure")

func TestASCIIDiagramSupported(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   bool
	}{
		{"flowchart LR", "flowchart LR\n  A --> B\n", true},
		{"flowchart TD", "flowchart TD\n  A --> B\n", true},
		{"graph", "graph TD\n  A --> B\n", true},
		{"bare graph", "graph\n  A --> B\n", true},
		{"sequenceDiagram", "sequenceDiagram\n  A->>B: hi\n", true},
		{"leading blank lines", "\n\n  flowchart LR\n  A --> B\n", true},
		{"leading comment", "%% a note\nflowchart LR\n  A --> B\n", true},
		// Frontmatter must be stripped before the type check, or a titled
		// diagram looks like an unsupported one.
		{"yaml frontmatter", "---\ntitle: My Flow\n---\nflowchart LR\n  A --> B\n", true},
		{"frontmatter with config", "---\ntitle: T\nconfig:\n  theme: dark\n---\nsequenceDiagram\n  A->>B: hi\n", true},
		{"frontmatter over an unsupported type", "---\ntitle: T\n---\ngantt\n  title A\n", false},
		// erDiagram parses but its output is visibly misaligned, so it is excluded
		// on purpose rather than because the library rejects it.
		{"erDiagram is excluded", "erDiagram\n  CUSTOMER ||--o{ ORDER : places\n", false},
		{"gantt", "gantt\n  title A\n", false},
		{"pie", "pie title Pets\n  \"Dogs\" : 1\n", false},
		{"stateDiagram", "stateDiagram-v2\n  [*] --> Idle\n", false},
		{"classDiagram", "classDiagram\n  Animal <|-- Duck\n", false},
		{"empty", "", false},
		{"whitespace only", "   \n\n", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := asciiDiagramSupported(tc.source); got != tc.want {
				t.Errorf("asciiDiagramSupported(%q) = %t, want %t", tc.source, got, tc.want)
			}
		})
	}
}

func TestRenderMermaidASCII_Flowchart(t *testing.T) {
	art, err := renderMermaidASCII("flowchart LR\n  A[Start] --> B[Process]\n", 80, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	if !strings.Contains(art, "Start") || !strings.Contains(art, "Process") {
		t.Errorf("node labels missing from the art:\n%s", art)
	}
	if !strings.ContainsAny(art, "┌┐└┘─│►") {
		t.Errorf("no box-drawing characters in the art:\n%s", art)
	}
	if strings.Contains(art, "-->") {
		t.Errorf("Mermaid source leaked into the art:\n%s", art)
	}
}

func TestRenderMermaidASCII_SequenceDiagram(t *testing.T) {
	art, err := renderMermaidASCII("sequenceDiagram\n  Alice->>Bob: Hello\n", 80, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	for _, want := range []string{"Alice", "Bob", "Hello"} {
		if !strings.Contains(art, want) {
			t.Errorf("art missing %q:\n%s", want, art)
		}
	}
	if !strings.ContainsAny(art, "┌┐└┘─│") {
		t.Errorf("no box-drawing characters in the sequence art:\n%s", art)
	}
}

// TestRenderMermaidASCII_PureASCIIUsesPlainCharacters covers -style ascii and
// terminals that cannot show box-drawing characters.
func TestRenderMermaidASCII_PureASCIIUsesPlainCharacters(t *testing.T) {
	art, err := renderMermaidASCII("flowchart LR\n  A[Start] --> B[End]\n", 80, true)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	if strings.ContainsAny(art, "┌┐└┘─│►┈") {
		t.Errorf("box-drawing characters present despite pure ASCII being requested:\n%s", art)
	}
	if !strings.ContainsAny(art, "+-|") {
		t.Errorf("no ASCII box characters in the art:\n%s", art)
	}
	for _, r := range art {
		if r > 127 && r != '\n' {
			t.Errorf("non-ASCII rune %q in pure ASCII art:\n%s", r, art)
			break
		}
	}
}

func TestRenderMermaidASCII_RejectsUnsupportedDiagramTypes(t *testing.T) {
	for _, src := range []string{
		"gantt\n  title A\n",
		"pie title Pets\n  \"Dogs\" : 1\n",
		"erDiagram\n  A ||--o{ B : x\n",
		"stateDiagram-v2\n  [*] --> Idle\n",
	} {
		if _, err := renderMermaidASCII(src, 80, false); err == nil {
			t.Errorf("renderMermaidASCII accepted unsupported source %q", src)
		}
	}
}

// TestRenderMermaidASCII_InvalidSourceDoesNotPanic covers the acceptance
// criterion that a malformed diagram reports an error instead of crashing.
func TestRenderMermaidASCII_InvalidSourceDoesNotPanic(t *testing.T) {
	for _, src := range []string{
		"flowchart LR\n  A[[[ -->|||| B]]]\n  -->\n",
		"flowchart LR\n",
		"sequenceDiagram\n  ->>: \n",
		"graph TD\n  " + strings.Repeat("A-->B\n  ", 200),
		"flowchart LR\n  A --> A\n",
	} {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("renderMermaidASCII panicked on %q: %v", src, r)
				}
			}()
			// Either outcome is acceptable; crashing is not.
			_, _ = renderMermaidASCII(src, 80, false)
		}()
	}
}

// TestSpliceConsoleDiagrams_NeverClipsImageSequences guards the escape sequence
// for an image, which occupies no columns and must be passed through untouched.
func TestSpliceConsoleDiagrams_NeverClipsImageSequences(t *testing.T) {
	sequence := "\x1b_Gf=100,a=T;" + strings.Repeat("A", 500) + "\x1b\\"
	out, _ := spliceConsoleDiagrams("  MD2PDFDG0\n", []consoleDiagram{
		{placeholder: "MD2PDFDG0", content: sequence, kind: diagramImage},
	}, 20)
	if !strings.Contains(out, sequence) {
		t.Error("image escape sequence was altered by the splice")
	}
}

// TestRenderMermaidASCII_HonorsSourceDirection pins that the direction in the
// diagram header reaches the layout. The parser in mermaid-ascii reads it from
// the source, so this guards against a future change here overriding it.
func TestRenderMermaidASCII_HonorsSourceDirection(t *testing.T) {
	lr, err := renderMermaidASCII("flowchart LR\n  A --> B\n", 80, false)
	if err != nil {
		t.Fatalf("LR: %v", err)
	}
	td, err := renderMermaidASCII("flowchart TD\n  A --> B\n", 80, false)
	if err != nil {
		t.Fatalf("TD: %v", err)
	}
	lrLines := len(strings.Split(strings.TrimRight(lr, "\n"), "\n"))
	tdLines := len(strings.Split(strings.TrimRight(td, "\n"), "\n"))
	if tdLines <= lrLines {
		t.Errorf("TD art (%d lines) is not taller than LR art (%d lines); "+
			"the direction in the source is being ignored\nLR:\n%s\nTD:\n%s",
			tdLines, lrLines, lr, td)
	}
}

// TestRenderMermaidASCII_BareHeaderLaysOutTopDown covers a header with no
// direction, which Mermaid itself defaults to top-down.
func TestRenderMermaidASCII_BareHeaderLaysOutTopDown(t *testing.T) {
	for _, src := range []string{"graph\n  A --> B\n", "flowchart\n  A --> B\n"} {
		art, err := renderMermaidASCII(src, 80, false)
		if err != nil {
			t.Fatalf("%q: %v", src, err)
		}
		lines := strings.Split(strings.TrimRight(art, "\n"), "\n")
		if len(lines) < 10 {
			t.Errorf("%q rendered %d lines, expected a tall top-down layout:\n%s",
				src, len(lines), art)
		}
	}
}

// TestRenderMermaidASCII_JapaneseLabels checks CJK labels, which occupy two
// columns each, do not break the layout.
func TestRenderMermaidASCII_JapaneseLabels(t *testing.T) {
	art, err := renderMermaidASCII("flowchart LR\n  A[開始] --> B[終了]\n", 80, false)
	if err != nil {
		t.Fatalf("renderMermaidASCII: %v", err)
	}
	for _, want := range []string{"開始", "終了"} {
		if !strings.Contains(art, want) {
			t.Errorf("art missing %q:\n%s", want, art)
		}
	}
}

func TestResolveConsoleMermaidPlan_ASCIIModes(t *testing.T) {
	kitty := func(k string) string {
		if k == "TERM" {
			return "xterm-kitty"
		}
		return ""
	}
	plain := func(string) string { return "" }

	tests := []struct {
		name     string
		mode     string
		isTTY    bool
		noColor  bool
		getenv   func(string) string
		wantMode string
	}{
		{"auto on a plain terminal now draws text art", "auto", true, false, plain, MermaidRenderASCII},
		{"auto when piped draws text art", "auto", false, false, kitty, MermaidRenderASCII},
		{"auto with NO_COLOR draws text art", "auto", true, true, kitty, MermaidRenderASCII},
		{"auto on kitty still prefers images", "auto", true, false, kitty, MermaidRenderImage},
		{"explicit ascii on kitty wins over images", "ascii", true, false, kitty, MermaidRenderASCII},
		{"explicit ascii when piped is fine", "ascii", false, false, plain, MermaidRenderASCII},
		{"explicit source stays source", "source", true, false, kitty, MermaidRenderSource},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := resolveConsoleMermaidPlan(t.Context(), tc.mode, tc.isTTY, tc.noColor, tc.getenv, nil)
			if err != nil {
				t.Fatalf("resolveConsoleMermaidPlan: %v", err)
			}
			if plan.mode != tc.wantMode {
				t.Errorf("mode = %q, want %q", plan.mode, tc.wantMode)
			}
		})
	}
}

// TestPrepareConsoleMermaid_ASCIIRendersWithoutMmdc is the point of this change:
// text art needs no external tool at all.
func TestPrepareConsoleMermaid_ASCIIRendersWithoutMmdc(t *testing.T) {
	md := []byte("# T\n\n```mermaid\nflowchart LR\n  A[Start] --> B[End]\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }
	c.rasterizeMermaid = func(context.Context, int, string) (string, error) {
		t.Fatal("rasterizer must not run in ascii mode")
		return "", nil
	}

	plan := consoleMermaidPlan{mode: MermaidRenderASCII}
	rewritten, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1", len(diagrams))
	}
	if !strings.ContainsAny(diagrams[0].content, "┌┐└┘─│►") {
		t.Errorf("diagram is not text art:\n%s", diagrams[0].content)
	}
	if strings.Contains(string(rewritten), "A[Start]") {
		t.Errorf("Mermaid source survived in the markdown:\n%s", rewritten)
	}
}

// TestPrepareConsoleMermaid_ASCIIFallsBackPerDiagramType covers a mixed document:
// the supported diagram becomes art, the unsupported one keeps its source.
func TestPrepareConsoleMermaid_ASCIIFallsBackPerDiagramType(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n\nmid\n\n```mermaid\ngantt\n  title Roadmap\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})

	plan := consoleMermaidPlan{mode: MermaidRenderASCII}
	rewritten, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1 (only the flowchart)", len(diagrams))
	}
	if !strings.Contains(string(rewritten), "title Roadmap") {
		t.Errorf("unsupported gantt block did not fall back to its source:\n%s", rewritten)
	}
	if strings.Contains(string(rewritten), "A --> B") {
		t.Errorf("supported flowchart leaked its source:\n%s", rewritten)
	}
}

// TestPrepareConsoleMermaid_ImagePlanFallsBackToASCIIWithoutMmdc walks the full
// chain for a plan auto chose: images are planned, mmdc is absent, so text art
// is used rather than dropping straight to the source. A strict plan errors
// instead — see TestPrepareConsoleMermaid_StrictImageModeErrorsWithoutMmdc.
func TestPrepareConsoleMermaid_ImagePlanFallsBackToASCIIWithoutMmdc(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }

	plan := consoleMermaidPlan{mode: MermaidRenderImage, transport: terminalImageTransport{protocol: imageProtocolKitty}}
	_, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1", len(diagrams))
	}
	if strings.Contains(diagrams[0].content, "\x1b_G") {
		t.Error("emitted a kitty image without mmdc")
	}
	if !strings.ContainsAny(diagrams[0].content, "┌┐└┘─│►") {
		t.Errorf("did not fall back to text art:\n%s", diagrams[0].content)
	}
}

// TestPrepareConsoleMermaid_ImageFailureFallsBackToASCII covers a per-diagram
// rasterization failure dropping one step down the chain rather than to source.
func TestPrepareConsoleMermaid_ImageFailureFallsBackToASCII(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return true }
	c.rasterizeMermaid = func(context.Context, int, string) (string, error) {
		return "", errStubRasterFailure
	}

	plan := consoleMermaidPlan{mode: MermaidRenderImage, transport: terminalImageTransport{protocol: imageProtocolKitty}}
	_, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1", len(diagrams))
	}
	if !strings.ContainsAny(diagrams[0].content, "┌┐└┘─│►") {
		t.Errorf("did not fall back to text art:\n%s", diagrams[0].content)
	}
}

// TestPrepareConsoleMermaid_SourceModeSkipsASCIIToo re-checks the escape hatch
// now that ascii sits in the chain.
func TestPrepareConsoleMermaid_SourceModeSkipsASCIIToo(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})

	plan := consoleMermaidPlan{mode: MermaidRenderSource}
	rewritten, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 0 {
		t.Errorf("got %d diagrams in source mode, want 0", len(diagrams))
	}
	if string(rewritten) != string(md) {
		t.Errorf("markdown was rewritten in source mode:\n%q", rewritten)
	}
}

func TestWantsPureASCIIStyle(t *testing.T) {
	tests := []struct {
		name     string
		explicit string
		envStyle string
		resolved string
		want     bool
	}{
		{"explicit ascii", "ascii", "", "ascii", true},
		// Redirected output resolves to notty, but an explicit -style ascii must
		// still win: this is the case that regressed.
		{"explicit ascii survives notty resolution", "ascii", "", "notty", true},
		{"explicit dark", "dark", "ascii", "dark", false},
		{"env ascii with no flag", "", "ascii", "notty", true},
		{"env dark with no flag", "", "dark", "dark", false},
		{"resolved ascii with no flag or env", "", "", "ascii", true},
		{"plain defaults", "", "", "dark", false},
		{"notty is not pure ascii", "", "", "notty", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := consoleWantsPureASCII(tc.explicit, tc.envStyle, tc.resolved)
			if got != tc.want {
				t.Errorf("consoleWantsPureASCII(%q, %q, %q) = %t, want %t",
					tc.explicit, tc.envStyle, tc.resolved, got, tc.want)
			}
		})
	}
}

func TestAnyImageDiagram(t *testing.T) {
	tests := []struct {
		name     string
		diagrams []consoleDiagram
		want     bool
	}{
		{"none", nil, false},
		{"text art only", []consoleDiagram{{kind: diagramTextArt}}, false},
		{"one image", []consoleDiagram{{kind: diagramImage}}, true},
		{"mixed", []consoleDiagram{{kind: diagramTextArt}, {kind: diagramImage}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := anyImageDiagram(tc.diagrams); got != tc.want {
				t.Errorf("anyImageDiagram() = %t, want %t", got, tc.want)
			}
		})
	}
}

// TestPrepareConsoleMermaid_ImageFallbackIsPageable is the regression test for
// the pager being skipped on an image-capable terminal whose diagrams actually
// came out as text art. The decision follows the emitted diagrams, not the plan,
// so nothing here may report itself as an image.
func TestPrepareConsoleMermaid_ImageFallbackIsPageable(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }

	plan := consoleMermaidPlan{mode: MermaidRenderImage, transport: terminalImageTransport{protocol: imageProtocolKitty}}
	_, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err != nil {
		t.Fatalf("prepareConsoleMermaid: %v", err)
	}
	if len(diagrams) != 1 {
		t.Fatalf("got %d diagrams, want 1", len(diagrams))
	}
	if anyImageDiagram(diagrams) {
		t.Error("a text-art fallback reported itself as an image, which would skip the pager")
	}
	if diagrams[0].pan {
		t.Error("text art that fits the wrap width must not be marked for panning")
	}
}

// TestPrepareConsoleMermaid_StrictImageModeErrorsWithoutMmdc pins that
// -mermaid-render image is a guarantee: without the rasterizer it fails rather
// than quietly producing something that is not an image.
func TestPrepareConsoleMermaid_StrictImageModeErrorsWithoutMmdc(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return false }

	plan := consoleMermaidPlan{mode: MermaidRenderImage, transport: terminalImageTransport{protocol: imageProtocolKitty}, strict: true}
	_, diagrams, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err == nil {
		t.Fatalf("expected an error in strict image mode, got %d diagrams", len(diagrams))
	}
	for _, want := range []string{"mermaid-render image", "mmdc"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

// TestPrepareConsoleMermaid_StrictImageModeErrorsOnRasterFailure covers the same
// guarantee for a diagram mmdc refuses to render.
func TestPrepareConsoleMermaid_StrictImageModeErrorsOnRasterFailure(t *testing.T) {
	md := []byte("```mermaid\nflowchart LR\n  A --> B\n```\n")
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.mermaidAvailable = func() bool { return true }
	c.rasterizeMermaid = func(context.Context, int, string) (string, error) { return "", errStubRasterFailure }

	plan := consoleMermaidPlan{mode: MermaidRenderImage, transport: terminalImageTransport{protocol: imageProtocolKitty}, strict: true}
	_, _, err := c.prepareConsoleMermaid(t.Context(), md, plan, 80)
	if err == nil {
		t.Fatal("expected an error in strict image mode when rasterization fails")
	}
	if !errors.Is(err, errStubRasterFailure) {
		t.Errorf("error does not wrap the rasterization failure: %v", err)
	}
}

// TestResolveConsoleMermaidPlan_StrictOnlyForExplicitImage checks the strict flag
// is set by the user naming "image", not by auto happening to choose it.
func TestResolveConsoleMermaidPlan_StrictOnlyForExplicitImage(t *testing.T) {
	kitty := func(k string) string {
		if k == "TERM" {
			return "xterm-kitty"
		}
		return ""
	}
	tests := []struct {
		mode       string
		wantStrict bool
	}{
		{"image", true},
		{"auto", false},
		{"", false},
	}
	for _, tc := range tests {
		t.Run(tc.mode, func(t *testing.T) {
			plan, err := resolveConsoleMermaidPlan(t.Context(), tc.mode, true, false, kitty, nil)
			if err != nil {
				t.Fatalf("resolveConsoleMermaidPlan: %v", err)
			}
			if plan.mode != MermaidRenderImage {
				t.Fatalf("mode = %q, want image", plan.mode)
			}
			if plan.strict != tc.wantStrict {
				t.Errorf("strict = %t, want %t", plan.strict, tc.wantStrict)
			}
		})
	}
}
