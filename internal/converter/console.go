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
	if _, err := os.Stat(value); err == nil {
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

// renderConsole renders the Markdown document as styled ANSI text, sending the
// result through the user's pager when writing to an interactive terminal.
// Mermaid blocks are left as fenced code so their source stays readable.
func (c *Converter) renderConsole(md []byte, out *os.File) error {
	env := detectConsoleEnv(out)
	width := resolveConsoleWidth(c.cfg.ConsoleWidth, env.isTTY, env.width)
	style := resolveConsoleStyle(c.cfg.ConsoleStyle, env.envStyle, env.isTTY, env.noColor, env.darkBG)
	c.logf("Rendering for the console (style: %s, width: %d)...", style, width)

	rendered, err := renderConsoleMarkdown(md, style, width)
	if err != nil {
		return err
	}

	if c.cfg.ConsolePager && env.isTTY {
		if argv, ok := resolvePager(); ok {
			c.logf("Paging output through %s...", strings.Join(argv, " "))
			return runPager(argv, rendered, env.profile)
		}
		c.logf("No pager found; writing straight to the terminal.")
	}
	return writeConsole(out, rendered, env.profile)
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
