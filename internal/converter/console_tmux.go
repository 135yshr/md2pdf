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

// tmuxClientFormat asks tmux for everything about the current client and pane
// that image detection depends on, in one invocation. The separator cannot occur
// in a TERM name, and splitting on it keeps the field positions even when the
// client name is empty because no client is attached.
const tmuxClientFormat = "#{client_termname}|#{window_active}|#{session_attached}|" +
	"#{window_zoomed_flag}|#{pane_active}"

// tmuxClient describes the client and pane md2pdf is drawing into.
type tmuxClient struct {
	// termName is TERM as the attached client reports it, empty when no client
	// is attached.
	termName string
	// visible reports whether tmux is currently showing this pane, which is what
	// allow-passthrough=on depends on.
	visible bool
}

// tmuxClientState reads the client and pane md2pdf is drawing into. It reports
// ok=false when tmux cannot be asked or answers something unparseable.
//
// A pane is visible when its session has a client attached, its window is the
// current one, and it is not hidden behind another pane zoomed over it.
func tmuxClientState(query tmuxQuery) (tmuxClient, bool) {
	out, err := query("display-message", "-p", tmuxClientFormat)
	if err != nil {
		return tmuxClient{}, false
	}
	fields := strings.Split(strings.TrimSpace(out), "|")
	if len(fields) != 5 {
		return tmuxClient{}, false
	}
	termName, windowActive, attached, zoomed, paneActive :=
		fields[0], fields[1], fields[2], fields[3], fields[4]

	visible := attached != "0" && attached != "" && windowActive == "1" &&
		!(zoomed == "1" && paneActive != "1")
	return tmuxClient{termName: termName, visible: visible}, true
}

// tmuxForwardsPassthrough reports whether tmux will forward DCS passthrough for
// this pane.
//
// tmux discards passthrough unless allow-passthrough is set, and the two
// settings differ in a way that matters: tmux(1) defines "on" as forwarding only
// while the pane is visible, and "all" as forwarding even when it is not. An
// image written into a hidden pane under "on" is dropped with no error anywhere,
// so the pane has to be checked rather than assumed. A tmux too old to know the
// option reports a failure, which is also a no.
func tmuxForwardsPassthrough(query tmuxQuery) (blockedReason string, forwards bool) {
	out, err := query("show", "-gv", "allow-passthrough")
	if err != nil {
		return "tmux could not be asked whether it forwards escape sequences; " +
			"a tmux older than 3.3 cannot", false
	}
	switch strings.TrimSpace(out) {
	case "all":
		return "", true
	case "on":
		client, ok := tmuxClientState(query)
		if ok && client.visible {
			return "", true
		}
		return "allow-passthrough is on, but tmux only forwards escape sequences for a " +
			"visible pane and this one is not; run `tmux set -g allow-passthrough all`", false
	default:
		return "allow-passthrough is off; run `tmux set -g allow-passthrough on`", false
	}
}

// tmuxDetectEnv returns the environment the outer terminal's protocol is read
// from inside a session.
//
// The attached client's TERM is authoritative, because tmux's global snapshot
// describes whatever started the server: attach the same server from a different
// terminal and that snapshot names the wrong one. KITTY_WINDOW_ID is dropped
// entirely for the same reason — it identifies a window of the terminal that
// started the server, and letting it through would report kitty for a client
// that is not kitty. Everything else comes from the snapshot, then the process
// environment.
func tmuxDetectEnv(query tmuxQuery, getenv func(string) string) func(string) string {
	global := tmuxGlobalEnv(query, getenv)
	client, ok := tmuxClientState(query)

	return func(key string) string {
		switch key {
		case "KITTY_WINDOW_ID":
			return ""
		case "TERM":
			if ok && client.termName != "" {
				return client.termName
			}
		}
		return global(key)
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
	// tmuxBlockedReason records why no protocol was offered when tmux is the
	// thing standing in the way, and is empty otherwise. It is worth telling
	// apart from a terminal that simply cannot show images: this one is a
	// setting away from working, and the setting depends on the reason.
	tmuxBlockedReason string
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
	if reason, forwards := tmuxForwardsPassthrough(query); !forwards {
		return terminalImageTransport{tmuxBlockedReason: reason}
	}

	protocol := detectImageProtocol(tmuxDetectEnv(query, getenv))
	if !slices.Contains(tmuxTunnelledProtocols, protocol) {
		return terminalImageTransport{}
	}
	return terminalImageTransport{protocol: protocol, viaTmux: true}
}
