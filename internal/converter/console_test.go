package converter

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/charmbracelet/colorprofile"
)

func TestResolveConsoleWidth(t *testing.T) {
	tests := []struct {
		name      string
		explicit  int
		isTTY     bool
		termWidth int
		want      int
	}{
		{"explicit width wins on a terminal", 100, true, 200, 100},
		{"explicit width wins when piped", 60, false, 0, 60},
		{"terminal width minus margin", 0, true, 80, 78},
		{"wide terminal is capped", 0, true, 200, consoleMaxWidth},
		{"narrow terminal keeps a floor", 0, true, 10, consoleMinWidth},
		{"piped output uses the fallback", 0, false, 200, consoleFallbackWidth},
		{"unknown terminal width uses the fallback", 0, true, 0, consoleFallbackWidth},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := resolveConsoleWidth(tc.explicit, tc.isTTY, tc.termWidth); got != tc.want {
				t.Errorf("resolveConsoleWidth(%d, %t, %d) = %d, want %d",
					tc.explicit, tc.isTTY, tc.termWidth, got, tc.want)
			}
		})
	}
}

func TestResolveConsoleStyle(t *testing.T) {
	dark := func() bool { return true }
	light := func() bool { return false }

	tests := []struct {
		name     string
		explicit string
		envStyle string
		isTTY    bool
		noColor  bool
		darkBG   func() bool
		want     string
	}{
		{"auto picks dark on a dark background", "", "", true, false, dark, "dark"},
		{"auto picks light on a light background", "", "", true, false, light, "light"},
		{"explicit auto is resolved too", "auto", "", true, false, light, "light"},
		{"explicit style wins", "dracula", "", true, false, dark, "dracula"},
		{"explicit style wins over the environment", "pink", "light", true, false, dark, "pink"},
		{"environment style is used when no flag", "", "light", true, false, dark, "light"},
		{"NO_COLOR forces notty", "dracula", "", true, true, dark, "notty"},
		{"non-terminal output forces notty", "dracula", "", false, false, dark, "notty"},
		{"missing background detection falls back to dark", "", "", true, false, nil, "dark"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := resolveConsoleStyle(tc.explicit, tc.envStyle, tc.isTTY, tc.noColor, tc.darkBG)
			if got != tc.want {
				t.Errorf("resolveConsoleStyle(%q, %q, %t, %t) = %q, want %q",
					tc.explicit, tc.envStyle, tc.isTTY, tc.noColor, got, tc.want)
			}
		})
	}
}

func TestRenderConsoleMarkdown_RendersStructure(t *testing.T) {
	md := []byte("# Title\n\nSome **bold** text.\n\n- first\n- second\n")

	out, err := renderConsoleMarkdown(md, "notty", 80)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}

	// The notty style keeps Markdown emphasis markers instead of using ANSI
	// bold, so the rendered text still carries them.
	got := string(out)
	for _, want := range []string{"Title", "Some **bold** text.", "• first", "• second"} {
		if !strings.Contains(got, want) {
			t.Errorf("rendered output missing %q:\n%s", want, got)
		}
	}
}

func TestRenderConsoleMarkdown_NoTTYStyleHasNoEscapes(t *testing.T) {
	out, err := renderConsoleMarkdown([]byte("# Title\n\n`code`\n"), "notty", 80)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}
	if bytes.ContainsRune(out, 0x1b) {
		t.Errorf("notty style emitted ANSI escapes: %q", out)
	}
}

func TestRenderConsoleMarkdown_DarkStyleAddsEscapes(t *testing.T) {
	out, err := renderConsoleMarkdown([]byte("# Title\n"), "dark", 80)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}
	if !bytes.ContainsRune(out, 0x1b) {
		t.Errorf("dark style emitted no ANSI escapes: %q", out)
	}
}

func TestRenderConsoleMarkdown_WrapsAtRequestedWidth(t *testing.T) {
	md := []byte(strings.Repeat("word ", 60))

	out, err := renderConsoleMarkdown(md, "notty", 40)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}

	for _, line := range strings.Split(strings.TrimRight(string(out), "\n"), "\n") {
		if len([]rune(line)) > 40 {
			t.Errorf("line exceeds requested width 40 (%d runes): %q", len([]rune(line)), line)
		}
	}
}

func TestRenderConsoleMarkdown_KeepsMermaidSourceAsCode(t *testing.T) {
	md := []byte("# Diagram\n\n```mermaid\ngraph TD\n  A[Start] --> B[End]\n```\n")

	out, err := renderConsoleMarkdown(md, "notty", 80)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}

	got := string(out)
	for _, want := range []string{"graph TD", "A[Start] --> B[End]"} {
		if !strings.Contains(got, want) {
			t.Errorf("Mermaid source missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "```") {
		t.Errorf("fence markers leaked into the rendered output:\n%s", got)
	}
}

func TestRenderConsoleMarkdown_ShowsImageAltText(t *testing.T) {
	out, err := renderConsoleMarkdown([]byte("![Architecture](diagram.png)\n"), "notty", 80)
	if err != nil {
		t.Fatalf("renderConsoleMarkdown: %v", err)
	}
	if !strings.Contains(string(out), "Architecture") {
		t.Errorf("image alt text missing from output: %q", out)
	}
}

func TestRenderConsoleMarkdown_RejectsUnknownStyle(t *testing.T) {
	if _, err := renderConsoleMarkdown([]byte("# Title\n"), "no-such-style", 80); err == nil {
		t.Error("expected an error for an unknown style, got nil")
	}
}

func TestValidateConsoleStyle(t *testing.T) {
	stylePath := filepath.Join(t.TempDir(), "custom.json")
	if err := os.WriteFile(stylePath, []byte("{}"), 0o644); err != nil {
		t.Fatalf("write stylesheet: %v", err)
	}

	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{"empty means auto", "", false},
		{"auto", "auto", false},
		{"builtin dark", "dark", false},
		{"builtin tokyo-night", "tokyo-night", false},
		{"existing stylesheet path", stylePath, false},
		{"unknown name", "darkk", true},
		{"missing stylesheet path", filepath.Join(t.TempDir(), "nope.json"), true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConsoleStyle(tc.value)
			if tc.wantErr && err == nil {
				t.Errorf("ValidateConsoleStyle(%q) = nil, want error", tc.value)
			}
			if !tc.wantErr && err != nil {
				t.Errorf("ValidateConsoleStyle(%q): %v", tc.value, err)
			}
		})
	}
}

func TestEnsureLessRawControl(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"bare less gains -R", []string{"less"}, []string{"less", "-R"}},
		{"absolute less gains -R", []string{"/usr/bin/less"}, []string{"/usr/bin/less", "-R"}},
		{"existing -R is kept", []string{"less", "-R"}, []string{"less", "-R"}},
		{"lowercase -r counts", []string{"less", "-r"}, []string{"less", "-r"}},
		{"combined flags count", []string{"less", "-FRX"}, []string{"less", "-FRX"}},
		{"long flag counts", []string{"less", "--raw-control-chars"}, []string{"less", "--raw-control-chars"}},
		{"other flags still gain -R", []string{"less", "-F"}, []string{"less", "-F", "-R"}},
		{"other pagers are untouched", []string{"more"}, []string{"more"}},
		{"bat is untouched", []string{"bat", "-p"}, []string{"bat", "-p"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureLessRawControl(append([]string(nil), tc.argv...))
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("ensureLessRawControl(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}

func TestResolvePager_UsesPagerEnv(t *testing.T) {
	t.Setenv("PAGER", "cat -v")

	argv, ok := resolvePager()
	if !ok {
		t.Fatal("resolvePager() reported no pager, want cat")
	}
	if filepath.Base(argv[0]) != "cat" {
		t.Errorf("pager binary = %q, want cat", argv[0])
	}
	if !filepath.IsAbs(argv[0]) {
		t.Errorf("pager binary %q is not an absolute path", argv[0])
	}
	if strings.Join(argv[1:], " ") != "-v" {
		t.Errorf("pager args = %v, want [-v]", argv[1:])
	}
}

func TestResolvePager_MissingPagerReportsUnavailable(t *testing.T) {
	t.Setenv("PAGER", "md2pdf-no-such-pager")

	if argv, ok := resolvePager(); ok {
		t.Errorf("resolvePager() = %v, true; want unavailable", argv)
	}
}

func TestResolvePager_DefaultKeepsLessDefaultUntouched(t *testing.T) {
	t.Setenv("PAGER", "less -X")

	if _, ok := resolvePager(); !ok {
		t.Skip("less is not installed")
	}
	if want := []string{"less", "-R", "-F"}; strings.Join(defaultPagerArgv, " ") != strings.Join(want, " ") {
		t.Errorf("defaultPagerArgv = %v, want %v", defaultPagerArgv, want)
	}
}

func TestWriteConsole_StripsColorWhenProfileIsNoTTY(t *testing.T) {
	var buf bytes.Buffer
	if err := writeConsole(&buf, []byte("\x1b[31mred\x1b[0m"), colorprofile.NoTTY); err != nil {
		t.Fatalf("writeConsole: %v", err)
	}
	if got := buf.String(); got != "red" {
		t.Errorf("writeConsole() wrote %q, want %q", got, "red")
	}
}

func TestWriteConsole_KeepsColorForTrueColor(t *testing.T) {
	var buf bytes.Buffer
	in := "\x1b[38;2;255;0;0mred\x1b[0m"
	if err := writeConsole(&buf, []byte(in), colorprofile.TrueColor); err != nil {
		t.Fatalf("writeConsole: %v", err)
	}
	if got := buf.String(); got != in {
		t.Errorf("writeConsole() wrote %q, want %q", got, in)
	}
}

func TestRenderConsole_WritesRenderedDocument(t *testing.T) {
	c, err := New(&Config{Format: FormatConsole, ConsoleWidth: 60})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer c.Close()

	out := filepath.Join(t.TempDir(), "out.txt")
	f, err := os.Create(out)
	if err != nil {
		t.Fatalf("create output: %v", err)
	}
	defer f.Close()

	// A regular file is not a terminal, so this exercises the non-TTY path:
	// notty style, no pager, color escapes stripped.
	if err := c.renderConsole([]byte("# Title\n\nBody text.\n"), f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	if !strings.Contains(string(got), "Title") || !strings.Contains(string(got), "Body text.") {
		t.Errorf("unexpected console output:\n%s", got)
	}
	if bytes.ContainsRune(got, 0x1b) {
		t.Errorf("non-terminal output contains ANSI escapes:\n%q", got)
	}
}
