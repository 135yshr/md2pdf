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
	default:
		return s.passthrough, nil
	}
}

// visibleClient is a display-message answer for an attached, visible pane whose
// terminal did not identify itself beyond TERM.
func visibleClient(termName string) string {
	return termName + "||1|1|0|1"
}

// visibleClientType is a display-message answer for an attached, visible pane
// whose terminal answered tmux's version query, the way iTerm2 and kitty do.
func visibleClientType(termName, termType string) string {
	return termName + "|" + termType + "|1|1|0|1"
}

// stubTmuxCommands answers show-environment and the allow-passthrough lookup,
// with a visible client reporting the TERM tmux's global environment holds.
func stubTmuxCommands(env, passthrough string) tmuxQuery {
	term := ""
	for _, line := range strings.Split(env, "\n") {
		if value, ok := strings.CutPrefix(line, "TERM="); ok {
			term = value
		}
	}
	return tmuxStub{env: env, passthrough: passthrough, client: visibleClient(term)}.query
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
			client:       visibleClientType("xterm-256color", "iTerm2 3.6.11"),
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
				query = tmuxStub{env: tc.tmuxEnv, client: tc.client, passthrough: tc.passthrough}.query
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

	direct, err := encodeTerminalImage(terminalImageTransport{protocol: imageProtocolKitty}, data, 40, 1)
	if err != nil {
		t.Fatalf("direct: %v", err)
	}
	if strings.HasPrefix(direct, "\x1bPtmux;") {
		t.Errorf("a sequence going straight to the terminal was wrapped for tmux: %q", direct[:32])
	}

	tunneled, err := encodeTerminalImage(
		terminalImageTransport{protocol: imageProtocolKitty, viaTmux: true}, data, 40, 1)
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
			tmuxStub{client: visibleClientType("xterm-256color", "iTerm2 3.6.11"), passthrough: "on\n"}.query)
		if err != nil {
			t.Fatalf("resolveConsoleMermaidPlan: %v", err)
		}
		if plan.transport.protocol != imageProtocolITerm2 || !plan.transport.viaTmux {
			t.Errorf("transport = %+v, want iTerm2 through tmux", plan.transport)
		}
	})
}

// TestTmuxClientState covers reading the pane md2pdf is actually drawing into.
// The global environment describes whatever started the server, which can be a
// different terminal from the one attached now.
func TestTmuxClientState(t *testing.T) {
	tests := []struct {
		name        string
		answer      string
		wantTerm    string
		wantVisible bool
		wantOK      bool
	}{
		{"attached and visible", "xterm-kitty||1|1|0|1", "xterm-kitty", true, true},
		{"visible split pane", "xterm-256color||1|1|0|0", "xterm-256color", true, true},
		{"background window", "xterm-kitty||0|1|0|1", "xterm-kitty", false, true},
		{"detached session", "xterm-kitty||1|0|0|1", "xterm-kitty", false, true},
		{"hidden behind a zoomed pane", "xterm-kitty||1|1|1|0", "xterm-kitty", false, true},
		{"the zoomed pane itself is visible", "xterm-kitty||1|1|1|1", "xterm-kitty", true, true},
		{"no attached client", "||1|0|0|1", "", false, true},
		{"unparseable answer", "nonsense", "", false, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := tmuxClientState(t.Context(), tmuxStub{client: tc.answer}.query)
			if ok != tc.wantOK {
				t.Fatalf("ok = %t, want %t", ok, tc.wantOK)
			}
			if got.termName != tc.wantTerm {
				t.Errorf("termName = %q, want %q", got.termName, tc.wantTerm)
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
		client      string
		err         error
		want        bool
	}{
		{"all with a visible pane", "all\n", visibleClient("xterm-kitty"), nil, true},
		{"all with a hidden pane", "all\n", "xterm-kitty||0|1|0|1", nil, true},
		{"on with a visible pane", "on\n", visibleClient("xterm-kitty"), nil, true},
		{"on with a background window", "on\n", "xterm-kitty||0|1|0|1", nil, false},
		{"on with a detached session", "on\n", "xterm-kitty||1|0|0|1", nil, false},
		{"off", "off\n", visibleClient("xterm-kitty"), nil, false},
		{"empty", "\n", visibleClient("xterm-kitty"), nil, false},
		{"tmux too old to know the option", "", "", errStubTmuxFailure, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{passthrough: tc.passthrough, client: tc.client, err: tc.err}.query
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
		client       string
		passthrough  string
		wantProtocol terminalImageProtocol
		wantViaTmux  bool
	}{
		{
			// kitty does not speak the iTerm2 protocol, so sending it there
			// would show escape garbage instead of a diagram.
			name:         "server started under iTerm2, attached from kitty",
			env:          "TERM=xterm-256color\nTERM_PROGRAM=iTerm.app\n",
			client:       visibleClient("xterm-kitty"),
			passthrough:  "on\n",
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			// The stale KITTY_WINDOW_ID must not make this look like kitty.
			name:         "server started under kitty, attached from a plain terminal",
			env:          "TERM=xterm-kitty\nKITTY_WINDOW_ID=7\n",
			client:       visibleClient("xterm-256color"),
			passthrough:  "on\n",
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "hidden pane with passthrough on",
			env:          "TERM_PROGRAM=iTerm.app\n",
			client:       "xterm-256color|0|1|0|1",
			passthrough:  "on\n",
			wantProtocol: imageProtocolNone,
		},
		{
			name:         "hidden pane with passthrough all",
			env:          "TERM_PROGRAM=iTerm.app\n",
			client:       "xterm-256color|iTerm2 3.6.11|0|1|0|1",
			passthrough:  "all\n",
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{env: tc.env, client: tc.client, passthrough: tc.passthrough}.query
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
			client:      visibleClient("xterm-256color"),
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
// the server snapshot; tmux reports the client's own answer to its version query
// as #{client_termtype}, which is what iTerm2 and kitty are recognized by.
func TestDetectImageTransport_NeverMixesClientAndServerMetadata(t *testing.T) {
	inTmux := mapGetenv(map[string]string{
		"TMUX":         "/tmp/tmux-501/default,1,0",
		"TERM_PROGRAM": "tmux",
		"TERM":         "screen-256color",
	})

	tests := []struct {
		name         string
		env          string
		client       string
		wantProtocol terminalImageProtocol
		wantViaTmux  bool
	}{
		{
			name:         "the client identifies itself as iTerm2",
			env:          "TERM_PROGRAM=iTerm.app\n",
			client:       visibleClientType("xterm-256color", "iTerm2 3.6.11"),
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
		{
			name:         "the client identifies itself as WezTerm",
			env:          "",
			client:       visibleClientType("xterm-256color", "WezTerm 20240203"),
			wantProtocol: imageProtocolITerm2,
			wantViaTmux:  true,
		},
		{
			name:         "the client identifies itself as kitty",
			env:          "TERM_PROGRAM=iTerm.app\n",
			client:       visibleClientType("xterm-256color", "kitty(0.35.2)"),
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			// The heart of the finding: iTerm2 started the server, a plain
			// terminal is attached, and the server's TERM_PROGRAM must count
			// for nothing.
			name:         "a plain client does not inherit the server's iTerm2",
			env:          "TERM_PROGRAM=iTerm.app\n",
			client:       visibleClient("xterm-256color"),
			wantProtocol: imageProtocolNone,
		},
		{
			// TERM is still client-authoritative for terminals that announce
			// themselves that way rather than through the version query.
			name:         "a client whose TERM names kitty is still kitty",
			env:          "TERM_PROGRAM=iTerm.app\n",
			client:       visibleClient("xterm-kitty"),
			wantProtocol: imageProtocolKitty,
			wantViaTmux:  true,
		},
		{
			name:         "a stale server KITTY_WINDOW_ID counts for nothing",
			env:          "TERM=xterm-kitty\nKITTY_WINDOW_ID=7\n",
			client:       visibleClient("xterm-256color"),
			wantProtocol: imageProtocolNone,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			query := tmuxStub{env: tc.env, client: tc.client, passthrough: "on\n"}.query
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
