package converter

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFindPython_ExplicitPathSucceeds(t *testing.T) {
	skipOnWindows(t)
	py := makeFakePython(t, "python-ok", 0, "")
	c := &Converter{cfg: &Config{PythonPath: py}}

	got, err := c.findPython()
	if err != nil {
		t.Fatalf("findPython: %v", err)
	}
	if got != py {
		t.Errorf("findPython() = %q, want %q", got, py)
	}
}

func TestFindPython_ExplicitPathFailsPrecheck(t *testing.T) {
	skipOnWindows(t)
	py := makeFakePython(t, "python-no-playwright", 1,
		"ModuleNotFoundError: No module named 'playwright'")
	c := &Converter{cfg: &Config{PythonPath: py}}

	_, err := c.findPython()
	if err == nil {
		t.Fatal("expected error from explicit path that cannot import playwright")
	}
	msg := err.Error()
	if !strings.Contains(msg, py) {
		t.Errorf("error should reference interpreter path %q, got: %v", py, err)
	}
	if !strings.Contains(msg, "playwright") {
		t.Errorf("error should mention playwright, got: %v", err)
	}
}

func TestFindPython_ExplicitPathNonexistent(t *testing.T) {
	c := &Converter{cfg: &Config{PythonPath: "/no/such/python-binary"}}

	if _, err := c.findPython(); err == nil {
		t.Fatal("expected error for nonexistent explicit python path")
	}
}

func TestFindPython_AutoDetectSkipsInterpreterMissingPlaywright(t *testing.T) {
	skipOnWindows(t)
	stubExtraPythonCandidates(t, nil)
	dir := t.TempDir()
	writeShellScript(t, filepath.Join(dir, "python3"), 1,
		"ModuleNotFoundError: No module named 'playwright'")
	t.Setenv("PATH", dir)

	c := &Converter{cfg: &Config{}}
	_, err := c.findPython()
	if err == nil {
		t.Fatal("expected error when only interpreter on PATH lacks playwright")
	}
	msg := err.Error()
	if !strings.Contains(msg, "playwright") {
		t.Errorf("error should mention playwright, got: %v", err)
	}
	if !strings.Contains(msg, "MD2PDF_PYTHON") {
		t.Errorf("error should suggest -python / MD2PDF_PYTHON remediation, got: %v", err)
	}
}

func TestFindPython_AutoDetectFindsWellKnownPath(t *testing.T) {
	skipOnWindows(t)
	py := makeFakePython(t, "python-ok", 0, "")
	stubExtraPythonCandidates(t, []string{py})
	t.Setenv("PATH", t.TempDir()) // empty: no python3/python on PATH

	c := &Converter{cfg: &Config{}}
	got, err := c.findPython()
	if err != nil {
		t.Fatalf("findPython: %v", err)
	}
	if got != py {
		t.Errorf("findPython() = %q, want %q (well-known path)", got, py)
	}
}

func TestFindPython_AutoDetectSkipsNonexistentExtras(t *testing.T) {
	skipOnWindows(t)
	bad := makeFakePython(t, "python-no-playwright", 1,
		"ModuleNotFoundError: No module named 'playwright'")
	stubExtraPythonCandidates(t, []string{
		"/no/such/python-1",
		"/no/such/python-2",
		bad,
	})
	t.Setenv("PATH", t.TempDir())

	c := &Converter{cfg: &Config{}}
	_, err := c.findPython()
	if err == nil {
		t.Fatal("expected error when no candidate has playwright")
	}
	msg := err.Error()
	if strings.Contains(msg, "/no/such/python") {
		t.Errorf("nonexistent paths should not appear in error, got: %v", err)
	}
	if !strings.Contains(msg, bad) {
		t.Errorf("error should reference the existing-but-failing python %q, got: %v", bad, err)
	}
}

func TestFindPython_AutoDetectDeduplicatesPathAndExtras(t *testing.T) {
	skipOnWindows(t)
	py := makeFakePython(t, "python3", 0, "")
	stubExtraPythonCandidates(t, []string{py}) // same path that PATH will resolve
	t.Setenv("PATH", filepath.Dir(py))

	c := &Converter{cfg: &Config{}}
	got, err := c.findPython()
	if err != nil {
		t.Fatalf("findPython: %v", err)
	}
	if got != py {
		t.Errorf("findPython() = %q, want %q", got, py)
	}
}

func TestLastNonEmptyLine(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"single line", "hello", "hello"},
		{"trailing newline", "hello\n", "hello"},
		{"multi-line uses last", "first\nsecond\nthird", "third"},
		{"trailing blanks ignored", "real\n\n  \n", "real"},
		{"only blanks", "\n  \n", ""},
		{"empty", "", ""},
		{
			"python traceback",
			"Traceback (most recent call last):\n  File \"<string>\", line 1, in <module>\nModuleNotFoundError: No module named 'playwright'\n",
			"ModuleNotFoundError: No module named 'playwright'",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastNonEmptyLine(tc.in); got != tc.want {
				t.Errorf("lastNonEmptyLine(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func skipOnWindows(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("test uses /bin/sh; skipping on Windows")
	}
}

// stubExtraPythonCandidates replaces the well-known-path generator for the
// duration of t, so tests don't pick up the host machine's real Python
// installations.
func stubExtraPythonCandidates(t *testing.T, paths []string) {
	t.Helper()
	saved := extraPythonCandidatesFn
	extraPythonCandidatesFn = func() []string { return paths }
	t.Cleanup(func() { extraPythonCandidatesFn = saved })
}

// makeFakePython writes an executable shell script to a fresh temp dir and
// returns its path. The script mimics enough of python's behavior for
// canImportPlaywright: it ignores its args, optionally writes stderrText to
// stderr, and exits with exitCode.
func makeFakePython(t *testing.T, name string, exitCode int, stderrText string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeShellScript(t, path, exitCode, stderrText)
	return path
}

func writeShellScript(t *testing.T, path string, exitCode int, stderrText string) {
	t.Helper()
	var b strings.Builder
	b.WriteString("#!/bin/sh\n")
	if stderrText != "" {
		b.WriteString("cat 1>&2 <<'EOF'\n")
		b.WriteString(stderrText)
		b.WriteString("\nEOF\n")
	}
	b.WriteString(fmt.Sprintf("exit %d\n", exitCode))
	if err := os.WriteFile(path, []byte(b.String()), 0o755); err != nil {
		t.Fatalf("write shell script %q: %v", path, err)
	}
}
