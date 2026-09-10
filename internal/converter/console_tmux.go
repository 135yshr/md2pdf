package converter

import (
	"os"
	"os/exec"
	"slices"
	"strings"
)

// tmuxBinary is the multiplexer md2pdf knows how to tunnel images through.
const tmuxBinary = "tmux"

// tmuxQuery runs a tmux command and returns its standard output. It is a
// function so tests can answer without a tmux server running.
type tmuxQuery func(args ...string) (string, error)

// runTmux is the production tmuxQuery.
func runTmux(args ...string) (string, error) {
	out, err := exec.Command(tmuxBinary, args...).Output()
	if err != nil {
		return "", err //nolint:wrapcheck // the caller only distinguishes success from failure
	}
	return string(out), nil
}

// insideTmux reports whether md2pdf is running inside a tmux session.
//
// $TMUX is the marker rather than TERM. tmux sets TERM to screen-256color, but
// so does GNU screen and so do other multiplexers, and only tmux offers the
// passthrough an inline image depends on.
func insideTmux(getenv func(string) string) bool {
	return getenv("TMUX") != ""
}

// tmuxGlobalEnv returns a getenv that answers from tmux's global environment
// first and the process environment second.
//
// Inside a session the process environment reports TERM_PROGRAM=tmux, which
// hides the terminal that actually has to draw the image. tmux keeps the
// original values, so the outer terminal can be identified from there. Keys tmux
// does not carry — a terminal that identifies itself through something other
// than TERM_PROGRAM, say — still come from the process environment, and a tmux
// that cannot be queried changes nothing.
func tmuxGlobalEnv(query tmuxQuery, getenv func(string) string) func(string) string {
	out, err := query("show-environment", "-g")
	if err != nil {
		return getenv
	}

	outer := make(map[string]string)
	for _, line := range strings.Split(out, "\n") {
		// tmux prints "-NAME" for a variable marked for removal, which is the
		// absence of a value rather than a value.
		if line == "" || strings.HasPrefix(line, "-") {
			continue
		}
		if key, value, ok := strings.Cut(line, "="); ok {
			outer[key] = value
		}
	}

	return func(key string) string {
		if value, ok := outer[key]; ok {
			return value
		}
		return getenv(key)
	}
}

// tmuxAllowsPassthrough reports whether the session forwards DCS passthrough to
// the outer terminal.
//
// tmux discards passthrough unless allow-passthrough is turned on, so an image
// emitted without it vanishes with no error anywhere. A tmux too old to know the
// option reports a failure, which is also a no.
func tmuxAllowsPassthrough(query tmuxQuery) bool {
	out, err := query("show", "-gv", "allow-passthrough")
	if err != nil {
		return false
	}
	switch strings.TrimSpace(out) {
	case "on", "all":
		return true
	default:
		return false
	}
}

// wrapTmuxPassthrough tunnels an escape sequence through tmux to the terminal
// hosting it.
//
// tmux forwards the body of a DCS "tmux;" sequence verbatim. Every ESC in the
// payload has to be doubled, or tmux reads the first one as the end of the
// sequence and the rest of the image leaks into the document as text.
func wrapTmuxPassthrough(sequence string) string {
	return "\x1bPtmux;" + strings.ReplaceAll(sequence, "\x1b", "\x1b\x1b") + "\x1b\\"
}

// resolveTmuxQuery returns the tmux runner to use, defaulting to the real one.
func resolveTmuxQuery(query tmuxQuery) tmuxQuery {
	if query != nil {
		return query
	}
	return runTmux
}

// osGetenv is the default environment lookup for the detectors.
func osGetenv(getenv func(string) string) func(string) string {
	if getenv != nil {
		return getenv
	}
	return os.Getenv
}

// terminalImageTransport describes how an image escape sequence reaches the
// terminal that draws it: which protocol that terminal speaks, and whether the
// sequence has to be tunnelled through tmux to get there.
type terminalImageTransport struct {
	protocol terminalImageProtocol
	viaTmux  bool
}

// tmuxTunnelledProtocols lists the protocols worth sending through tmux's
// passthrough.
//
// Sixel is deliberately absent. tmux draws Sixel itself when built with support
// for it, and a passthrough-wrapped Sixel is not reliably forwarded, so
// claiming it would emit a sequence that comes out as garbage rather than an
// image.
var tmuxTunnelledProtocols = []terminalImageProtocol{imageProtocolKitty, imageProtocolITerm2}

// detectImageTransport decides how an inline image would reach the terminal.
//
// Outside tmux this is just the protocol detection. Inside it, two more things
// have to hold: tmux must be forwarding passthrough at all, and the outer
// terminal — read from tmux's own environment, since the process environment
// only describes tmux — must speak a protocol worth tunnelling. Anything else
// reports no transport, so the caller falls back to text art rather than writing
// a sequence that would vanish or corrupt the output.
func detectImageTransport(getenv func(string) string, query tmuxQuery) terminalImageTransport {
	getenv = osGetenv(getenv)
	if !insideTmux(getenv) {
		return terminalImageTransport{protocol: detectImageProtocol(getenv)}
	}

	query = resolveTmuxQuery(query)
	if !tmuxAllowsPassthrough(query) {
		return terminalImageTransport{}
	}

	protocol := detectImageProtocol(tmuxGlobalEnv(query, getenv))
	if !slices.Contains(tmuxTunnelledProtocols, protocol) {
		return terminalImageTransport{}
	}
	return terminalImageTransport{protocol: protocol, viaTmux: true}
}
