package converter

import (
	"context"
	"os"
	"os/exec"
	"slices"
	"strings"
)

// tmuxBinary is the multiplexer md2pdf knows how to tunnel images through.
const tmuxBinary = "tmux"

// tmuxQuery runs a tmux command and returns its standard output. It is a
// function so tests can answer without a tmux server running.
type tmuxQuery func(ctx context.Context, args ...string) (string, error)

// runTmux is the production tmuxQuery.
func runTmux(ctx context.Context, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, tmuxBinary, args...).Output()
	if err != nil {
		return "", err //nolint:wrapcheck // the caller only distinguishes success from failure
	}
	return string(out), nil
}

// insideTmux reports whether md2pdf is running inside a tmux session.
//
// $TMUX is the marker rather than TERM, because TERM is set to screen-256color
// by tmux, by GNU screen and by other multiplexers alike — and only tmux offers
// the passthrough an inline image depends on.
func insideTmux(getenv func(string) string) bool {
	return getenv("TMUX") != ""
}

// tmuxClientFormat asks tmux for everything about the current client and pane
// that image detection depends on, in one invocation. The separator cannot occur
// in a TERM name, and splitting on it keeps the field positions even when the
// client name is empty because no client is attached.
const tmuxPaneFormat = "#{session_name}|#{window_active}|#{session_attached}|" +
	"#{window_zoomed_flag}|#{pane_active}"

// tmuxClientListFormat asks for the terminal identity of one attached client.
const tmuxClientListFormat = "#{client_termname}|#{client_termtype}"

// tmuxClient describes one terminal attached to the session.
type tmuxClient struct {
	// termName is TERM as the client reports it.
	termName string
	// termType is what the client answered tmux's terminal version query with,
	// such as "iTerm2 3.6.11" or "kitty(0.35.2)". It is empty for a terminal
	// that does not answer, and on a tmux too old to report it.
	termType string
}

// tmuxPane describes the pane md2pdf is drawing into.
type tmuxPane struct {
	// session names the session the pane belongs to, which is what the client
	// list has to be asked about.
	session string
	// visible reports whether tmux is currently showing this pane, which is what
	// allow-passthrough=on depends on.
	visible bool
}

// tmuxPaneState reads the pane md2pdf is drawing into. It reports ok=false when
// tmux cannot be asked or answers something unparseable.
//
// A pane is visible when its session has a client attached, its window is the
// current one, and it is not hidden behind another pane zoomed over it.
func tmuxPaneState(ctx context.Context, query tmuxQuery) (tmuxPane, bool) {
	out, err := query(ctx, "display-message", "-p", tmuxPaneFormat)
	if err != nil {
		return tmuxPane{}, false
	}
	fields := strings.Split(strings.TrimSpace(out), "|")
	if len(fields) != 5 {
		return tmuxPane{}, false
	}
	session, windowActive, attached, zoomed, paneActive :=
		fields[0], fields[1], fields[2], fields[3], fields[4]

	visible := attached != "0" && attached != "" && windowActive == "1" &&
		(zoomed != "1" || paneActive == "1")
	return tmuxPane{session: session, visible: visible}, true
}

// tmuxSessionClients lists every terminal attached to the session.
//
// One client's answer is not the whole picture: a pane's passthrough output is
// delivered to every client showing it, and session_attached is a count rather
// than a flag. It reports ok=false when tmux cannot be asked.
func tmuxSessionClients(ctx context.Context, query tmuxQuery, session string) ([]tmuxClient, bool) {
	out, err := query(ctx, "list-clients", "-t", session, "-F", tmuxClientListFormat)
	if err != nil {
		return nil, false
	}
	var clients []tmuxClient
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		termName, termType, _ := strings.Cut(line, "|")
		clients = append(clients, tmuxClient{termName: termName, termType: termType})
	}
	return clients, true
}

// tmuxForwardsPassthrough reports whether tmux will forward DCS passthrough for
// this pane.
//
// Passthrough is discarded unless allow-passthrough is set, and the two settings
// differ in a way that matters: tmux(1) defines "on" as forwarding only while the
// pane is visible, and "all" as forwarding even when it is not. An image written
// into a hidden pane under "on" is dropped with no error anywhere, so the pane
// has to be checked rather than assumed. A tmux too old to know the option
// reports a failure, which is also a no.
func tmuxForwardsPassthrough(ctx context.Context, query tmuxQuery) (blockedReason string, forwards bool) {
	out, err := query(ctx, "show", "-gv", "allow-passthrough")
	if err != nil {
		return "tmux could not be asked whether it forwards escape sequences; " +
			"a tmux older than 3.3 cannot", false
	}
	switch strings.TrimSpace(out) {
	case "all":
		return "", true
	case "on":
		pane, ok := tmuxPaneState(ctx, query)
		if ok && pane.visible {
			return "", true
		}
		return "allow-passthrough is on, but tmux only forwards escape sequences for a " +
			"visible pane and this one is not; run `tmux set -g allow-passthrough all`", false
	default:
		return "allow-passthrough is off; run `tmux set -g allow-passthrough on`", false
	}
}

// tmuxClientTermTypes maps a substring of the client's answer to tmux's
// terminal version query onto the image protocol that terminal speaks.
//
// The answer is the one signal about the attached client that identifies a
// terminal whose TERM says nothing useful — iTerm2 and WezTerm both report a
// plain xterm-256color.
var tmuxClientTermTypes = []struct {
	marker   string
	protocol terminalImageProtocol
}{
	{marker: "kitty", protocol: imageProtocolKitty},
	{marker: "ghostty", protocol: imageProtocolKitty},
	{marker: "iterm2", protocol: imageProtocolITerm2},
	{marker: "wezterm", protocol: imageProtocolITerm2},
}

// tmuxClientsProtocol reports the image protocol every attached client speaks,
// and imageProtocolNone unless they all speak the same one.
//
// Agreement is required because passthrough is not addressed to a client: tmux
// delivers it to every client showing the pane. Claiming kitty because one
// viewer is kitty would emit kitty graphics to an iTerm2 viewer as well, which
// shows nothing — and the Mermaid source it would have read has already been
// replaced by then. Text art suits every client, so disagreement falls back to
// it rather than picking a winner.
func tmuxClientsProtocol(clients []tmuxClient) terminalImageProtocol {
	if len(clients) == 0 {
		return imageProtocolNone
	}
	agreed := tmuxClientProtocol(clients[0])
	for _, client := range clients[1:] {
		if tmuxClientProtocol(client) != agreed {
			return imageProtocolNone
		}
	}
	return agreed
}

// tmuxClientProtocol reports the image protocol one client speaks, deciding
// entirely from what that client told tmux about itself.
//
// Nothing here may come from tmux's global environment. That snapshot describes
// whatever started the server, so a server started under iTerm2 and attached
// from a plain terminal would otherwise be sent an OSC 1337 image the client
// cannot show — and which vanishes without an error. The same reasoning rules
// out the process environment's KITTY_WINDOW_ID, which names a window of the
// terminal that started the server.
//
// The version query answer is tried first because it identifies terminals whose
// TERM does not, with TERM second for the terminals that announce themselves
// that way instead.
func tmuxClientProtocol(client tmuxClient) terminalImageProtocol {
	termType := strings.ToLower(client.termType)
	for _, candidate := range tmuxClientTermTypes {
		if strings.Contains(termType, candidate.marker) {
			return candidate.protocol
		}
	}
	return detectImageProtocol(func(key string) string {
		if key == "TERM" {
			return client.termName
		}
		return ""
	})
}

// wrapTmuxPassthrough tunnels an escape sequence through tmux to the terminal
// hosting it.
//
// The body of a DCS "tmux;" sequence is forwarded verbatim. Every ESC in the
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
// sequence has to be tunneled through tmux to get there.
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
// Sixel is deliberately absent, because tmux draws it itself when built with
// support for it and a passthrough-wrapped Sixel is not reliably forwarded:
// claiming it would emit a sequence that comes out as garbage rather than an
// image.
var tmuxTunnelledProtocols = []terminalImageProtocol{imageProtocolKitty, imageProtocolITerm2}

// detectImageTransport decides how an inline image would reach the terminal.
//
// Outside tmux this is just the protocol detection. Inside it, two more things
// have to hold: tmux must be forwarding passthrough at all, and the outer
// terminal — read from tmux's own environment, since the process environment
// only describes tmux — must speak a protocol worth tunneling. Anything else
// reports no transport, so the caller falls back to text art rather than writing
// a sequence that would vanish or corrupt the output.
func detectImageTransport(ctx context.Context, getenv func(string) string, query tmuxQuery) terminalImageTransport {
	getenv = osGetenv(getenv)
	if !insideTmux(getenv) {
		return terminalImageTransport{protocol: detectImageProtocol(getenv)}
	}

	query = resolveTmuxQuery(query)
	if reason, forwards := tmuxForwardsPassthrough(ctx, query); !forwards {
		return terminalImageTransport{tmuxBlockedReason: reason}
	}

	pane, ok := tmuxPaneState(ctx, query)
	if !ok {
		return terminalImageTransport{}
	}
	clients, ok := tmuxSessionClients(ctx, query, pane.session)
	if !ok {
		return terminalImageTransport{}
	}
	protocol := tmuxClientsProtocol(clients)
	if !slices.Contains(tmuxTunnelledProtocols, protocol) {
		return terminalImageTransport{}
	}
	return terminalImageTransport{protocol: protocol, viaTmux: true}
}
