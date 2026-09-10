package converter

import (
	"errors"
	"strings"
	"testing"
)

// errStubTmuxFailure stands in for a tmux command that cannot run.
var errStubTmuxFailure = errors.New("stub tmux failure")

// stubTmux returns a tmuxQuery answering the given output for any command.
func stubTmux(out string, err error) tmuxQuery {
	return func(...string) (string, error) { return out, err }
}

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

// TestTmuxGlobalEnv covers reading the outer terminal's identity back out of
// tmux. Inside a session the process environment says TERM_PROGRAM=tmux, which
// hides the terminal that actually has to draw the image; tmux keeps the
// original in its global environment.
func TestTmuxGlobalEnv(t *testing.T) {
	const output = "COLORTERM=truecolor\nTERM=xterm-256color\nTERM_PROGRAM=iTerm.app\n-REMOVED\n"

	t.Run("reads the outer terminal", func(t *testing.T) {
		outer := tmuxGlobalEnv(stubTmux(output, nil), mapGetenv(map[string]string{
			"TERM_PROGRAM": "tmux",
			"TERM":         "screen-256color",
		}))
		if got := outer("TERM_PROGRAM"); got != "iTerm.app" {
			t.Errorf("TERM_PROGRAM = %q, want iTerm.app", got)
		}
		if got := outer("TERM"); got != "xterm-256color" {
			t.Errorf("TERM = %q, want xterm-256color", got)
		}
	})

	t.Run("falls back to the process environment", func(t *testing.T) {
		outer := tmuxGlobalEnv(stubTmux(output, nil), mapGetenv(map[string]string{
			"KITTY_WINDOW_ID": "3",
		}))
		if got := outer("KITTY_WINDOW_ID"); got != "3" {
			t.Errorf("KITTY_WINDOW_ID = %q, want 3, tmux does not carry it", got)
		}
	})

	t.Run("an entry marked for removal is not a value", func(t *testing.T) {
		outer := tmuxGlobalEnv(stubTmux(output, nil), mapGetenv(nil))
		if got := outer("REMOVED"); got != "" {
			t.Errorf("REMOVED = %q, want empty", got)
		}
	})

	t.Run("a failing tmux leaves the process environment alone", func(t *testing.T) {
		outer := tmuxGlobalEnv(stubTmux("", errStubTmuxFailure), mapGetenv(map[string]string{
			"TERM_PROGRAM": "tmux",
		}))
		if got := outer("TERM_PROGRAM"); got != "tmux" {
			t.Errorf("TERM_PROGRAM = %q, want the process value tmux", got)
		}
	})
}

// TestTmuxAllowsPassthrough covers the option that decides whether an escape
// sequence reaches the outer terminal at all. tmux discards passthrough unless
// it is turned on, so an image emitted without it would simply vanish.
func TestTmuxAllowsPassthrough(t *testing.T) {
	tests := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		{"on", "on\n", nil, true},
		{"all", "all\n", nil, true},
		{"off", "off\n", nil, false},
		{"empty", "\n", nil, false},
		{"tmux too old to know the option", "", errStubTmuxFailure, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := tmuxAllowsPassthrough(stubTmux(tc.out, tc.err)); got != tc.want {
				t.Errorf("tmuxAllowsPassthrough(%q, %v) = %t, want %t", tc.out, tc.err, got, tc.want)
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

// stubTmuxCommands answers show-environment and the allow-passthrough lookup
// separately, which is what transport detection asks for.
func stubTmuxCommands(env, passthrough string) tmuxQuery {
	return func(args ...string) (string, error) {
		if len(args) > 0 && args[0] == "show-environment" {
			return env, nil
		}
		return passthrough, nil
	}
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
			// Sixel is not tunnelled: tmux handles it natively when built for
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
			got := detectImageTransport(mapGetenv(tc.env), stubTmuxCommands(tc.tmuxEnv, tc.passthrough))
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

	tunnelled, err := encodeTerminalImage(
		terminalImageTransport{protocol: imageProtocolKitty, viaTmux: true}, data, 40)
	if err != nil {
		t.Fatalf("tunnelled: %v", err)
	}
	if tunnelled != wrapTmuxPassthrough(direct) {
		t.Error("the tmux sequence is not the direct one wrapped for passthrough")
	}
	if strings.Contains(tunnelled[len("\x1bPtmux;"):len(tunnelled)-2], "\x1b\x1b\x1b") {
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
		plan, err := resolveConsoleMermaidPlan(MermaidRenderAuto, true, false, getenv, query)
		if err != nil {
			t.Fatalf("resolveConsoleMermaidPlan: %v", err)
		}
		if plan.mode != MermaidRenderASCII {
			t.Errorf("mode = %q, want %q", plan.mode, MermaidRenderASCII)
		}
	})

	t.Run("explicit image names the setting", func(t *testing.T) {
		_, err := resolveConsoleMermaidPlan(MermaidRenderImage, true, false, getenv, query)
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
		plan, err := resolveConsoleMermaidPlan(MermaidRenderImage, true, false, getenv,
			stubTmuxCommands("TERM_PROGRAM=iTerm.app\n", "on\n"))
		if err != nil {
			t.Fatalf("resolveConsoleMermaidPlan: %v", err)
		}
		if plan.transport.protocol != imageProtocolITerm2 || !plan.transport.viaTmux {
			t.Errorf("transport = %+v, want iTerm2 through tmux", plan.transport)
		}
	})
}
