package converter

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// errStubTmuxFailure stands in for a tmux command that cannot run.
var errStubTmuxFailure = errors.New("stub tmux failure")

// TestInsideTmux pins how tmux is detected. $TMUX is the marker rather than
// TERM: tmux sets TERM to screen-256color, but so do other multiplexers and a
// plain screen session, and only tmux offers the passthrough this relies on.
func TestInsideTmux(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want bool
	}{
		{"inside tmux", map[string]string{"TMUX": "/tmp/tmux-501/default,123,0"}, true},
		{"outside tmux", map[string]string{"TERM": "xterm-256color"}, false},
		{"screen TERM without tmux", map[string]string{"TERM": "screen-256color"}, false},
		{"empty TMUX is not inside", map[string]string{"TMUX": ""}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := insideTmux(mapGetenv(tc.env)); got != tc.want {
				t.Errorf("insideTmux(%v) = %t, want %t", tc.env, got, tc.want)
			}
		})
	}
}

// mapGetenv adapts a map to the getenv function the detectors take.
func mapGetenv(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

// TestWrapTmuxPassthrough covers the tunnel itself: tmux forwards a DCS
// wrapped payload to the outer terminal, and every ESC inside it has to be
// doubled or tmux ends the sequence at the first one.
func TestWrapTmuxPassthrough(t *testing.T) {
	got := wrapTmuxPassthrough("\x1b_Gf=100;AAAA\x1b\\")
	const want = "\x1bPtmux;\x1b\x1b_Gf=100;AAAA\x1b\x1b\\\x1b\\"
	if got != want {
		t.Errorf("wrapTmuxPassthrough()\n got: %q\nwant: %q", got, want)
	}
	if !strings.HasPrefix(got, "\x1bPtmux;") {
		t.Errorf("payload is not wrapped in a tmux DCS: %q", got)
	}
}

// tmuxStub answers each tmux command transport detection issues. A zero client
// stands for no attached client at all.
type tmuxStub struct {
	env         string
	passthrough string
	client      string
	clients     string
	err         error
}

// query implements tmuxQuery.
func (s tmuxStub) query(_ context.Context, args ...string) (string, error) {
	if s.err != nil {
		return "", s.err
	}
	switch {
	case len(args) == 0:
		return "", nil
	case args[0] == "show-environment":
		return s.env, nil
	case args[0] == "display-message":
		return s.client, nil
	case args[0] == "list-clients":
		return s.clients, nil
	default:
		return s.passthrough, nil
	}
}

// visiblePane is a display-message answer for an attached, visible pane.
const visiblePane = "0|1|1|0|1"

// clientList is a list-clients answer for terminals attached to the session.
func clientList(clients ...string) string {
	return strings.Join(clients, "\n") + "\n"
}

// stubTmuxCommands answers the passthrough lookup with a visible pane and a
// single attached client reporting the TERM tmux's global environment holds.
func stubTmuxCommands(env, passthrough string) tmuxQuery {
	term := ""
	for _, line := range strings.Split(env, "\n") {
		if value, ok := strings.CutPrefix(line, "TERM="); ok {
			term = value
		}
	}
	return tmuxStub{
		env:         env,
		passthrough: passthrough,
		client:      visiblePane,
		clients:     clientList(term + "|"),
	}.query
}

// TestDetectImageTransport covers where an image escape sequence has to go.
// Inside tmux the process environment describes tmux, not the terminal that
// draws, so detection has to look through the session — and only when tmux will
// actually forward the sequence.
func TestDetectImageTransport(t *testing.T) {
	const insideEnv = "TERM=screen-256color"
	tests := []struct {
		name         string
		env          map[string]string
		tmuxEnv      string
		client       string
		clients      string
		passthrough  string
		wantProtocol terminalImageProtocol
		wantViaTmux  bool
	}{
		{
			name:         "outside tmux is unchanged",
			env:          map[string]string{"TERM_PROGRAM": "iTerm.app"},
			wantProtocol: imageProtocolITerm2,
		},
		{
			name:         "outside tmux with no image protocol",
			env:          map[string]string{"TERM": "xterm-256color"},
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "tmux over iTerm2 with passthrough on",
			env:          map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TERM_PROGRAM": "tmux", "TERM": "screen-256color"},
			tmuxEnv:      insideEnv + "\nTERM_PROGRAM=iTerm.app\n",
			client:       visiblePane,
			clients:      clientList("xterm-256color|iTerm2 3.6.11"),
			passthrough:  "on\n",
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
		{
			name:         "tmux over kitty with passthrough on",
			env:          map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TERM_PROGRAM": "tmux", "TERM": "screen-256color"},
			tmuxEnv:      "TERM=xterm-kitty\n",
			passthrough:  "all\n",
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			// Without passthrough tmux swallows the sequence and the image
			// vanishes with no error, so nothing may be claimed.
			name:         "tmux with passthrough off",
			env:          map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TERM_PROGRAM": "tmux"},
			tmuxEnv:      insideEnv + "\nTERM_PROGRAM=iTerm.app\n",
			passthrough:  "off\n",
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "tmux over a terminal with no image protocol",
			env:          map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TERM_PROGRAM": "tmux"},
			tmuxEnv:      "TERM=xterm-256color\n",
			passthrough:  "on\n",
			wantProtocol: imageProtocolNone,
		},
		{
			// Sixel is not tunneled: tmux handles it natively when built for
			// it, and a passthrough-wrapped Sixel is not reliably forwarded.
			name:         "tmux over a Sixel terminal is not claimed",
			env:          map[string]string{"TMUX": "/tmp/tmux-501/default,1,0", "TERM_PROGRAM": "tmux"},
			tmuxEnv:      "TERM=foot\n",
			passthrough:  "on\n",
			wantProtocol: imageProtocolNone,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := stubTmuxCommands(tc.tmuxEnv, tc.passthrough)
			if tc.client != "" {
				query = tmuxStub{env: tc.tmuxEnv, client: tc.client, clients: tc.clients,
					passthrough: tc.passthrough}.query
			}
			got := detectImageTransport(t.Context(), mapGetenv(tc.env), query)
			if got.protocol != tc.wantProtocol {
				t.Errorf("protocol = %v, want %v", got.protocol, tc.wantProtocol)
			}
			if got.viaTmux != tc.wantViaTmux {
				t.Errorf("viaTmux = %t, want %t", got.viaTmux, tc.wantViaTmux)
			}
		})
	}
}

// TestEncodeTerminalImage_TunnelsThroughTmux covers the last step: the same
// image, wrapped for tmux or not depending on how it has to travel.
func TestEncodeTerminalImage_TunnelsThroughTmux(t *testing.T) {
	data := stubPNG(t, 16, 16)

	direct, err := encodeTerminalImage(terminalImageTransport{protocol: imageProtocolKitty}, data, 40)
	if err != nil {
		t.Fatalf("direct: %v", err)
	}
	if strings.HasPrefix(direct, "\x1bPtmux;") {
		t.Errorf("a sequence going straight to the terminal was wrapped for tmux: %q", direct[:32])
	}

	tunneled, err := encodeTerminalImage(
		terminalImageTransport{protocol: imageProtocolKitty, viaTmux: true}, data, 40)
	if err != nil {
		t.Fatalf("tunneled: %v", err)
	}
	if tunneled != wrapTmuxPassthrough(direct) {
		t.Error("the tmux sequence is not the direct one wrapped for passthrough")
	}
	if strings.Contains(tunneled[len("\x1bPtmux;"):len(tunneled)-2], "\x1b\x1b\x1b") {
		t.Error("ESC doubling went wrong inside the payload")
	}
}

// TestResolveConsoleMermaidPlan_TmuxPassthroughOff covers the error a user can
// act on. Inside tmux with passthrough off, images are one setting away from
// working, so -mermaid-render image must say which setting rather than blame the
// terminal for lacking a protocol it does have.
func TestResolveConsoleMermaidPlan_TmuxPassthroughOff(t *testing.T) {
	getenv := mapGetenv(map[string]string{
		"TMUX":         "/tmp/tmux-501/default,1,0",
		"TERM_PROGRAM": "tmux",
		"TERM":         "screen-256color",
	})
	query := stubTmuxCommands("TERM_PROGRAM=iTerm.app\n", "off\n")

	t.Run("auto degrades to text art", func(t *testing.T) {
		plan, err := resolveConsoleMermaidPlan(t.Context(), MermaidRenderAuto, true, false, getenv, query)
		if err != nil {
			t.Fatalf("resolveConsoleMermaidPlan: %v", err)
		}
		if plan.mode != MermaidRenderASCII {
			t.Errorf("mode = %q, want %q", plan.mode, MermaidRenderASCII)
		}
	})

	t.Run("explicit image names the setting", func(t *testing.T) {
		_, err := resolveConsoleMermaidPlan(t.Context(), MermaidRenderImage, true, false, getenv, query)
		if err == nil {
			t.Fatal("expected an error with tmux passthrough off")
		}
		for _, want := range []string{"allow-passthrough", "tmux"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q does not mention %q", err, want)
			}
		}
	})

	t.Run("passthrough on reaches the outer terminal", func(t *testing.T) {
		plan, err := resolveConsoleMermaidPlan(t.Context(), MermaidRenderImage, true, false, getenv,
			tmuxStub{client: visiblePane, clients: clientList("xterm-256color|iTerm2 3.6.11"),
				passthrough: "on\n"}.query)
		if err != nil {
			t.Fatalf("resolveConsoleMermaidPlan: %v", err)
		}
		if plan.transport.protocol != imageProtocolITerm2 || !plan.transport.viaTmux {
			t.Errorf("transport = %+v, want iTerm2 through tmux", plan.transport)
		}
	})
}

// TestTmuxPaneState covers reading the pane md2pdf is drawing into: which
// session it belongs to, and whether tmux is currently showing it.
func TestTmuxPaneState(t *testing.T) {
	tests := []struct {
		name        string
		answer      string
		wantSession string
		wantVisible bool
		wantOK      bool
	}{
		{"attached and visible", "0|1|1|0|1", "0", true, true},
		{"visible split pane", "work|1|1|0|0", "work", true, true},
		{"background window", "0|0|1|0|1", "0", false, true},
		{"detached session", "0|1|0|0|1", "0", false, true},
		{"hidden behind a zoomed pane", "0|1|1|1|0", "0", false, true},
		{"the zoomed pane itself is visible", "0|1|1|1|1", "0", true, true},
		{"unparseable answer", "nonsense", "", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tmuxPaneState(t.Context(), tmuxStub{client: tc.answer}.query)
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tc.wantOK)
			}
			if got.session != tc.wantSession {
				t.Errorf("session = %q, want %q", got.session, tc.wantSession)
			}
			if got.visible != tc.wantVisible {
				t.Errorf("visible = %t, want %t", got.visible, tc.wantVisible)
			}
		})
	}
}

// TestTmuxForwardsPassthrough covers what tmux(1) actually promises: "on"
// forwards passthrough only while the pane is visible, and "all" forwards it
// either way. Treating "on" as unconditional would emit an image into a hidden
// pane, where tmux drops it and the diagram silently disappears.
func TestTmuxForwardsPassthrough(t *testing.T) {
	tests := []struct {
		name        string
		passthrough string
		pane        string
		err         error
		want        bool
	}{
		{"all with a visible pane", "all\n", visiblePane, nil, true},
		{"all with a hidden pane", "all\n", "0|0|1|0|1", nil, true},
		{"on with a visible pane", "on\n", visiblePane, nil, true},
		{"on with a background window", "on\n", "0|0|1|0|1", nil, false},
		{"on with a detached session", "on\n", "0|1|0|0|1", nil, false},
		{"off", "off\n", visiblePane, nil, false},
		{"empty", "\n", visiblePane, nil, false},
		{"tmux too old to know the option", "", "", errStubTmuxFailure, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{passthrough: tc.passthrough, client: tc.pane, err: tc.err}.query
			reason, got := tmuxForwardsPassthrough(t.Context(), query)
			if got != tc.want {
				t.Errorf("tmuxForwardsPassthrough() = %t, want %t (reason %q)", got, tc.want, reason)
			}
			if got == (reason != "") {
				t.Errorf("reason %q does not match forwards=%t", reason, got)
			}
		})
	}
}

// TestDetectImageTransport_FollowsTheAttachedClient is the regression test for
// trusting tmux's server-global snapshot. A server started under one terminal
// and attached from another must not have the first terminal's protocol sent to
// the second.
func TestDetectImageTransport_FollowsTheAttachedClient(t *testing.T) {
	inTmux := mapGetenv(map[string]string{
		"TMUX":         "/tmp/tmux-501/default,1,0",
		"TERM_PROGRAM": "tmux",
		"TERM":         "screen-256color",
	})

	tests := []struct {
		name         string
		env          string
		pane         string
		clients      string
		passthrough  string
		wantProtocol terminalImageProtocol
		wantViaTmux  bool
	}{
		{
			// kitty does not speak the iTerm2 protocol, so sending it there
			// would show escape garbage instead of a diagram.
			name:         "server started under iTerm2, attached from kitty",
			env:          "TERM=xterm-256color\nTERM_PROGRAM=iTerm.app\n",
			pane:         visiblePane,
			clients:      clientList("xterm-kitty|"),
			passthrough:  "on\n",
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			// The stale KITTY_WINDOW_ID must not make this look like kitty.
			name:         "server started under kitty, attached from a plain terminal",
			env:          "TERM=xterm-kitty\nKITTY_WINDOW_ID=7\n",
			pane:         visiblePane,
			clients:      clientList("xterm-256color|"),
			passthrough:  "on\n",
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "hidden pane with passthrough on",
			env:          "TERM_PROGRAM=iTerm.app\n",
			pane:         "0|0|1|0|1",
			clients:      clientList("xterm-256color|iTerm2 3.6.11"),
			passthrough:  "on\n",
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "hidden pane with passthrough all",
			env:          "TERM_PROGRAM=iTerm.app\n",
			pane:         "0|0|1|0|1",
			clients:      clientList("xterm-256color|iTerm2 3.6.11"),
			passthrough:  "all\n",
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{env: tc.env, client: tc.pane, clients: tc.clients,
				passthrough: tc.passthrough}.query
			got := detectImageTransport(t.Context(), inTmux, query)
			if got.protocol != tc.wantProtocol {
				t.Errorf("protocol = %v, want %v", got.protocol, tc.wantProtocol)
			}
			if got.viaTmux != tc.wantViaTmux {
				t.Errorf("viaTmux = %t, want %t", got.viaTmux, tc.wantViaTmux)
			}
		})
	}
}

// TestResolveConsoleMermaidPlan_TmuxBlockedReasonsAreActionable pins that the
// error names the setting that actually applies. "off" and "on but this pane is
// hidden" need different commands, so a single fixed message would send half the
// users to the wrong one.
func TestResolveConsoleMermaidPlan_TmuxBlockedReasonsAreActionable(t *testing.T) {
	getenv := mapGetenv(map[string]string{
		"TMUX":         "/tmp/tmux-501/default,1,0",
		"TERM_PROGRAM": "tmux",
	})
	const globals = "TERM_PROGRAM=iTerm.app\n"

	tests := []struct {
		name        string
		passthrough string
		client      string
		want        string
	}{
		{
			name:        "off points at turning it on",
			passthrough: "off\n",
			client:      visiblePane,
			want:        "allow-passthrough on`",
		},
		{
			name:        "on with a hidden pane points at all",
			passthrough: "on\n",
			client:      "xterm-256color||0|1|0|1",
			want:        "allow-passthrough all`",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{env: globals, client: tc.client, passthrough: tc.passthrough}.query
			_, err := resolveConsoleMermaidPlan(t.Context(), MermaidRenderImage, true, false, getenv, query)
			if err == nil {
				t.Fatal("expected an error")
			}
			if !strings.Contains(err.Error(), tc.want) {
				t.Errorf("error %q does not point at %q", err, tc.want)
			}
		})
	}
}

// TestDetectImageTransport_NeverMixesClientAndServerMetadata is the regression
// test for identifying the protocol from a mix of the two terminals.
//
// A server started under iTerm2 and attached from a plain terminal used to keep
// the server's TERM_PROGRAM, so an OSC 1337 image was tunneled to a client that
// cannot show one — it disappears. Nothing protocol-identifying may come from
// the server snapshot; tmux reports each client's own answer to its version
// query as #{client_termtype}, which is what iTerm2 and kitty are recognized by.
func TestDetectImageTransport_NeverMixesClientAndServerMetadata(t *testing.T) {
	inTmux := mapGetenv(map[string]string{
		"TMUX":         "/tmp/tmux-501/default,1,0",
		"TERM_PROGRAM": "tmux",
		"TERM":         "screen-256color",
	})

	tests := []struct {
		name         string
		env          string
		clients      string
		wantProtocol terminalImageProtocol
		wantViaTmux  bool
	}{
		{
			name:         "the client identifies itself as iTerm2",
			env:          "TERM_PROGRAM=iTerm.app\n",
			clients:      clientList("xterm-256color|iTerm2 3.6.11"),
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
		{
			name:         "the client identifies itself as WezTerm",
			env:          "",
			clients:      clientList("xterm-256color|WezTerm 20240203"),
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
		{
			name:         "the client identifies itself as kitty",
			env:          "TERM_PROGRAM=iTerm.app\n",
			clients:      clientList("xterm-256color|kitty(0.35.2)"),
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			// The heart of the finding: iTerm2 started the server, a plain
			// terminal is attached, and the server's TERM_PROGRAM must count
			// for nothing.
			name:         "a plain client does not inherit the server's iTerm2",
			env:          "TERM_PROGRAM=iTerm.app\n",
			clients:      clientList("xterm-256color|"),
			wantProtocol: imageProtocolNone,
		},
		{
			// TERM is still client-authoritative for terminals that announce
			// themselves that way rather than through the version query.
			name:         "a client whose TERM names kitty is still kitty",
			env:          "TERM_PROGRAM=iTerm.app\n",
			clients:      clientList("xterm-kitty|"),
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			name:         "a stale server KITTY_WINDOW_ID counts for nothing",
			env:          "TERM=xterm-kitty\nKITTY_WINDOW_ID=7\n",
			clients:      clientList("xterm-256color|"),
			wantProtocol: imageProtocolNone,
		},
		{
			// Two terminals watching the same session: passthrough reaches
			// both, so neither protocol may be claimed.
			name:         "two clients that disagree get no protocol",
			env:          "",
			clients:      clientList("xterm-kitty|kitty(0.35.2)", "xterm-256color|iTerm2 3.6.11"),
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "two clients that agree do",
			env:          "",
			clients:      clientList("xterm-kitty|kitty(0.35.2)", "xterm-kitty|"),
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{env: tc.env, client: visiblePane, clients: tc.clients,
				passthrough: "on\n"}.query
			got := detectImageTransport(t.Context(), inTmux, query)
			if got.protocol != tc.wantProtocol {
				t.Errorf("protocol = %v, want %v", got.protocol, tc.wantProtocol)
			}
			if got.viaTmux != tc.wantViaTmux {
				t.Errorf("viaTmux = %t, want %t", got.viaTmux, tc.wantViaTmux)
			}
		})
	}
}

// TestTmuxSessionClients covers reading every client attached to the session.
// A pane's passthrough output goes to all of them, so one client's answer is
// not the whole picture.
func TestTmuxSessionClients(t *testing.T) {
	tests := []struct {
		name   string
		out    string
		err    error
		want   []tmuxClient
		wantOK bool
	}{
		{
			name:   "one client",
			out:    "xterm-256color|iTerm2 3.6.11\n",
			want:   []tmuxClient{{termName: "xterm-256color", termType: "iTerm2 3.6.11"}},
			wantOK: true,
		},
		{
			name: "two clients",
			out:  "xterm-kitty|kitty(0.35.2)\nxterm-256color|iTerm2 3.6.11\n",
			want: []tmuxClient{
				{termName: "xterm-kitty", termType: "kitty(0.35.2)"},
				{termName: "xterm-256color", termType: "iTerm2 3.6.11"},
			},
			wantOK: true,
		},
		{
			name:   "a client that answered no version query",
			out:    "xterm-256color|\n",
			want:   []tmuxClient{{termName: "xterm-256color"}},
			wantOK: true,
		},
		{name: "no clients", out: "\n", want: nil, wantOK: true},
		{name: "tmux cannot be asked", out: "", err: errStubTmuxFailure, want: nil, wantOK: false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tmuxSessionClients(t.Context(), tmuxStub{clients: tc.out, err: tc.err}.query, "0")
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tc.wantOK)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d clients, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("client %d = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestTmuxClientsProtocol is the fix for a session watched from two terminals at
// once. Passthrough reaches every client showing the pane, so a protocol may
// only be claimed when they all speak it: emitting kitty graphics because one
// client is kitty would leave an iTerm2 viewer with no diagram at all, and the
// Mermaid source already replaced.
func TestTmuxClientsProtocol(t *testing.T) {
	kitty := tmuxClient{termName: "xterm-kitty", termType: "kitty(0.35.2)"}
	kittyByTerm := tmuxClient{termName: "xterm-kitty"}
	iterm := tmuxClient{termName: "xterm-256color", termType: "iTerm2 3.6.11"}
	plain := tmuxClient{termName: "xterm-256color"}

	tests := []struct {
		name    string
		clients []tmuxClient
		want    terminalImageProtocol
	}{
		{"no clients", nil, imageProtocolNone},
		{"one kitty", []tmuxClient{kitty}, imageProtocolKitty},
		{"one iTerm2", []tmuxClient{iterm}, imageProtocolITerm2},
		{"two clients that agree", []tmuxClient{kitty, kittyByTerm}, imageProtocolKitty},
		{"two clients that disagree", []tmuxClient{kitty, iterm}, imageProtocolNone},
		{"one capable, one not", []tmuxClient{kitty, plain}, imageProtocolNone},
		{"none capable", []tmuxClient{plain, plain}, imageProtocolNone},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tmuxClientsProtocol(tc.clients); got != tc.want {
				t.Errorf("tmuxClientsProtocol(%+v) = %v, want %v", tc.clients, got, tc.want)
			}
		})
	}
}
