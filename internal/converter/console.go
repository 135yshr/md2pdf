package converter

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"

	"charm.land/glamour/v2"
	"charm.land/glamour/v2/styles"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/charmbracelet/x/term"
)

// Word-wrap bounds for console output. The width follows the terminal but stops
// at consoleMaxWidth so lines stay comfortable to read on wide displays.
const (
	consoleMaxWidth      = 120
	consoleFallbackWidth = 80
	consoleMinWidth      = 20
	consoleRightMargin   = 2
)

// consoleStyleAuto resolves to a dark or light style based on the detected
// terminal background color.
const consoleStyleAuto = "auto"

// builtinConsoleStyles lists the style names accepted by -style, in addition to
// a path to a JSON stylesheet.
var builtinConsoleStyles = []string{
	consoleStyleAuto,
	styles.AsciiStyle,
	styles.DarkStyle,
	styles.DraculaStyle,
	styles.LightStyle,
	styles.NoTTYStyle,
	styles.PinkStyle,
	styles.TokyoNightStyle,
}

// defaultPagerArgv is the pager used when $PAGER is unset. -R keeps ANSI color
// escapes intact and -F skips paging for documents that fit on one screen.
var defaultPagerArgv = []string{"less", "-R", "-F"}

// ValidateConsoleStyle reports whether value names a built-in console style or
// points at an existing JSON stylesheet. An empty value selects the default.
func ValidateConsoleStyle(value string) error {
	if value == "" || slices.Contains(builtinConsoleStyles, value) {
		return nil
	}
	if info, err := os.Stat(value); err == nil && info.Mode().IsRegular() {
		return nil
	}
	return fmt.Errorf("unknown console style %q: use one of %s, or a path to a JSON stylesheet",
		value, strings.Join(builtinConsoleStyles, ", "))
}

// consoleEnv captures the terminal properties that affect console rendering.
type consoleEnv struct {
	isTTY    bool
	width    int
	noColor  bool
	envStyle string
	profile  colorprofile.Profile
	darkBG   func() bool
}

// detectConsoleEnv inspects out and the environment to decide how the rendered
// document should be styled and wrapped.
func detectConsoleEnv(out *os.File) consoleEnv {
	env := consoleEnv{
		noColor:  os.Getenv("NO_COLOR") != "",
		envStyle: os.Getenv("GLAMOUR_STYLE"),
		profile:  colorprofile.Detect(out, os.Environ()),
		darkBG:   func() bool { return lipgloss.HasDarkBackground(os.Stdin, out) },
	}
	if term.IsTerminal(out.Fd()) {
		env.isTTY = true
		if w, _, err := term.GetSize(out.Fd()); err == nil {
			env.width = w
		}
	}
	return env
}

// renderConsole renders each input document as styled ANSI text, in the order
// given, separated by a rule, and sends the result through the user's pager when
// writing to an interactive terminal.
//
// Mermaid blocks walk a fallback chain: an inline image on terminals that
// support an image protocol, then box-drawing text art, then the fenced Mermaid
// source. Only the image step needs anything external (mmdc) or a capable
// terminal, so a plain terminal and a redirected stream both still get diagrams.
//
// The pager is skipped only when an inline image was actually drawn: neither the
// kitty graphics sequences nor the iTerm2 inline-image sequences survive a trip
// through less. Text art is plain text and pages normally, including when an
// image plan fell back to it.
//
// The pager is resolved before the diagrams are drawn, not after, because it
// decides what happens to a diagram no label cap could fit: an output that can
// scroll sideways gets the diagram in full, and one that cannot gets its Mermaid
// source. Nothing is ever shown with its right edge cut off.
func (c *Converter) renderConsole(inputs []string, out *os.File) error {
	env := detectConsoleEnv(out)
	width := resolveConsoleWidth(c.cfg.ConsoleWidth, env.isTTY, env.width)
	style := resolveConsoleStyle(c.cfg.ConsoleStyle, env.envStyle, env.isTTY, env.noColor, env.darkBG)
	c.logf("Rendering %d document(s) for the console (style: %s, width: %d)...",
		len(inputs), style, width)

	// The pager is resolved here rather than at delivery time because the
	// diagrams need to know about it: whether the output can scroll sideways
	// decides what happens to a diagram too wide for the wrap width, and that
	// is settled while the diagrams are being drawn.
	var pagerArgv []string
	pagerAvailable := false
	if c.cfg.ConsolePager && env.isTTY {
		pagerArgv, pagerAvailable = resolvePager()
	}

	// The plan depends on the terminal, not on the document, so it is resolved
	// once and shared. What the diagrams turned out to be is not: that is
	// collected across documents, because the pager is started once for all of
	// them.
	plan, err := resolveConsoleMermaidPlan(c.cfg.MermaidRender, env.isTTY, env.noColor, os.Getenv, runTmux)
	if err != nil {
		return err
	}
	plan.pureASCII = consoleWantsPureASCII(c.cfg.ConsoleStyle, env.envStyle, style)
	plan.canPan = resolveConsolePan(c.cfg.ConsolePager, env.isTTY, pagerArgv, pagerAvailable)
	plan.scale = c.cfg.MermaidScale

	docs := make([][]byte, 0, len(inputs))
	var drawn consoleDrawn
	for _, input := range inputs {
		md, err := c.readInput(input)
		if err != nil {
			return err
		}
		rendered, documentDrawn, err := c.renderConsoleDocument(md, style, width, plan)
		if err != nil {
			return err
		}
		docs = append(docs, rendered)
		drawn = drawn.or(documentDrawn)
	}

	separator, err := c.consoleSeparator(len(docs), style, width)
	if err != nil {
		return err
	}
	rendered := joinConsoleDocuments(docs, separator)

	if c.cfg.ConsolePager && env.isTTY {
		switch {
		case drawn.images:
			c.logf("Skipping the pager so the inline images survive; " +
				"use -mermaid-render ascii or source to page the document instead.")
		case pagerAvailable:
			argv := pagerArgvFor(pagerArgv, drawn.panned)
			c.logf("Paging output through %s...", strings.Join(argv, " "))
			return runPager(argv, rendered, env.profile)
		default:
			c.logf("No pager found; writing straight to the terminal.")
		}
	}
	return writeConsole(out, rendered, env.profile)
}

// consoleDrawn records what the diagrams in a rendered document turned out to
// be, which decides how the document reaches the reader.
type consoleDrawn struct {
	// images is set when a diagram was written as an inline image escape
	// sequence, which does not survive the pager.
	images bool
	// panned is set when a diagram is wider than the wrap width, which the
	// pager has to be told to chop rather than fold.
	panned bool
}

// or combines what two documents drew. Both facts are aggregated across a run
// because the pager is started once for the whole output.
func (d consoleDrawn) or(other consoleDrawn) consoleDrawn {
	return consoleDrawn{
		images: d.images || other.images,
		panned: d.panned || other.panned,
	}
}

// renderConsoleDocument renders one Markdown document to styled ANSI text,
// drawing its Mermaid blocks and splicing them into place. Documents are
// rendered independently so their Mermaid placeholder tokens cannot collide.
func (c *Converter) renderConsoleDocument(md []byte, style string, width int, plan consoleMermaidPlan) ([]byte, consoleDrawn, error) {
	doc, diagrams, err := c.prepareConsoleMermaid(md, plan, width)
	if err != nil {
		return nil, consoleDrawn{}, err
	}

	rendered, err := renderConsoleMarkdown(doc, style, width)
	if err != nil {
		return nil, consoleDrawn{}, err
	}
	if len(diagrams) > 0 {
		spliced, missing := spliceConsoleDiagrams(string(rendered), diagrams, width)
		for _, token := range missing {
			c.logf("  warning: could not place diagram %s in the rendered output", token)
		}
		rendered = []byte(spliced)
	}
	return rendered, consoleDrawn{
		images: anyImageDiagram(diagrams),
		panned: anyPanDiagram(diagrams),
	}, nil
}

// consoleSeparator returns the rule placed between consecutive documents, or
// nil when there is nothing to separate.
//
// The rule is glamour's own horizontal rule, produced by rendering "---" at the
// same style and width as the documents. That keeps it in step with the theme
// for free — dimmed under a dark style, plain ASCII under -style ascii, colorless
// under NO_COLOR — and makes the separator look exactly like a rule the author
// wrote in the Markdown themselves.
func (c *Converter) consoleSeparator(docs int, style string, width int) ([]byte, error) {
	if docs < 2 {
		return nil, nil
	}
	rule, err := renderConsoleMarkdown([]byte("---\n"), style, width)
	if err != nil {
		return nil, err
	}
	return rule, nil
}

// joinConsoleDocuments concatenates rendered documents, placing the separator
// between consecutive pairs but never before the first or after the last.
func joinConsoleDocuments(docs [][]byte, separator []byte) []byte {
	return bytes.Join(docs, separator)
}

// resolveConsoleWidth picks the word-wrap width. An explicit width always wins;
// otherwise the terminal width is used, less a small right margin and capped at
// consoleMaxWidth. Non-terminal output falls back to a fixed width so piped
// output stays reproducible.
func resolveConsoleWidth(explicit int, isTTY bool, termWidth int) int {
	if explicit > 0 {
		return explicit
	}
	if !isTTY || termWidth <= 0 {
		return consoleFallbackWidth
	}
	switch w := termWidth - consoleRightMargin; {
	case w > consoleMaxWidth:
		return consoleMaxWidth
	case w < consoleMinWidth:
		return consoleMinWidth
	default:
		return w
	}
}

// resolveConsoleStyle decides which glamour style to render with. Color is
// dropped entirely when NO_COLOR is set or the output is not a terminal. The
// -style flag wins over GLAMOUR_STYLE, and "auto" resolves to dark or light by
// inspecting the terminal background.
func resolveConsoleStyle(explicit, envStyle string, isTTY, noColor bool, darkBG func() bool) string {
	if noColor || !isTTY {
		return styles.NoTTYStyle
	}

	style := explicit
	if style == "" {
		style = envStyle
	}
	if style == "" {
		style = consoleStyleAuto
	}
	if style != consoleStyleAuto {
		return style
	}

	if darkBG == nil || darkBG() {
		return styles.DarkStyle
	}
	return styles.LightStyle
}

// renderConsoleMarkdown renders Markdown to styled ANSI text at the given wrap
// width. The style is either a built-in style name or a JSON stylesheet path.
func renderConsoleMarkdown(md []byte, style string, width int) ([]byte, error) {
	r, err := glamour.NewTermRenderer(
		glamour.WithStylePath(style),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return nil, fmt.Errorf("create terminal renderer: %w", err)
	}

	out, err := r.RenderBytes(md)
	if err != nil {
		return nil, fmt.Errorf("render markdown for console: %w", err)
	}
	return out, nil
}

// resolvePager returns the pager command line to pipe rendered output through,
// preferring $PAGER over the less default. It reports ok=false when no pager
// binary is available, leaving the caller to write to the terminal directly.
func resolvePager() (argv []string, ok bool) {
	argv = slices.Clone(defaultPagerArgv)
	if fields := strings.Fields(os.Getenv("PAGER")); len(fields) > 0 {
		argv = ensureLessRawControl(fields)
	}

	path, err := exec.LookPath(argv[0])
	if err != nil {
		return nil, false
	}
	argv[0] = path
	return argv, true
}

// ensureLessRawControl appends -R when the configured pager is less without a
// raw control character flag; without it ANSI escapes show up as literal text.
func ensureLessRawControl(argv []string) []string {
	if filepath.Base(argv[0]) != "less" {
		return argv
	}
	for _, arg := range argv[1:] {
		switch {
		case strings.HasPrefix(arg, "--raw"), strings.HasPrefix(arg, "--RAW"):
			return argv
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.ContainsAny(arg, "Rr"):
			return argv
		}
	}
	return append(argv, "-R")
}

// runPager pipes rendered output through argv, downsampling colors to what the
// terminal supports before the pager ever sees them.
func runPager(argv []string, rendered []byte, profile colorprofile.Profile) error {
	var buf bytes.Buffer
	if err := writeConsole(&buf, rendered, profile); err != nil {
		return err
	}

	cmd := exec.Command(argv[0], argv[1:]...) //nolint:gosec // G204: the pager comes from $PAGER or the less default
	cmd.Stdin = &buf
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("run pager %s: %w", filepath.Base(argv[0]), err)
	}
	return nil
}

// writeConsole writes rendered output to w, downsampling colors to what the
// terminal supports. Color escapes are stripped entirely for non-terminals.
func writeConsole(w io.Writer, rendered []byte, profile colorprofile.Profile) error {
	cw := &colorprofile.Writer{Forward: w, Profile: profile}
	if _, err := cw.Write(rendered); err != nil {
		return fmt.Errorf("write console output: %w", err)
	}
	return nil
}
