package converter

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFindPandoc_ExplicitPathSucceeds(t *testing.T) {
	skipOnWindows(t)
	bin := makeFakePandoc(t, "pandoc-ok")
	c := &Converter{cfg: &Config{PandocPath: bin}}

	got, err := c.findPandoc()
	if err != nil {
		t.Fatalf("findPandoc: %v", err)
	}
	if got != bin {
		t.Errorf("findPandoc() = %q, want %q", got, bin)
	}
}

func TestFindPandoc_ExplicitPathNonexistent(t *testing.T) {
	c := &Converter{cfg: &Config{PandocPath: "/no/such/pandoc-binary"}}

	_, err := c.findPandoc()
	if err == nil {
		t.Fatal("expected error for nonexistent explicit pandoc path")
	}
	if !strings.Contains(err.Error(), "pandoc") {
		t.Errorf("error should mention pandoc, got: %v", err)
	}
}

func TestFindPandoc_AutoDetectFromPath(t *testing.T) {
	skipOnWindows(t)
	dir := t.TempDir()
	bin := filepath.Join(dir, "pandoc")
	writeShellScript(t, bin, 0, "")
	t.Setenv("PATH", dir)

	c := &Converter{cfg: &Config{}}
	got, err := c.findPandoc()
	if err != nil {
		t.Fatalf("findPandoc: %v", err)
	}
	if got != bin {
		t.Errorf("findPandoc() = %q, want %q", got, bin)
	}
}

func TestConvertDOCX_InvokesPandocWithExpectedArgs(t *testing.T) {
	skipOnWindows(t)
	workDir := t.TempDir()
	argsFile := filepath.Join(workDir, "args.txt")
	bin := makeArgsRecordingPandoc(t, argsFile)

	htmlPath := filepath.Join(workDir, "document.html")
	if err := os.WriteFile(htmlPath, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}
	docxPath := filepath.Join(workDir, "out.docx")

	c := &Converter{cfg: &Config{PandocPath: bin}, workDir: workDir}
	if err := c.convertDOCX(htmlPath, docxPath); err != nil {
		t.Fatalf("convertDOCX: %v", err)
	}

	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}
	args := string(recorded)
	for _, want := range []string{htmlPath, "-f", "html", "-o", docxPath} {
		if !strings.Contains(args, want) {
			t.Errorf("pandoc args missing %q; got: %s", want, args)
		}
	}
}

func TestConvertDOCX_PropagatesPandocFailure(t *testing.T) {
	skipOnWindows(t)
	workDir := t.TempDir()
	bin := filepath.Join(t.TempDir(), "pandoc")
	writeShellScript(t, bin, 1, "pandoc: boom")

	htmlPath := filepath.Join(workDir, "document.html")
	if err := os.WriteFile(htmlPath, []byte("<h1>hi</h1>"), 0o644); err != nil {
		t.Fatalf("write html: %v", err)
	}

	c := &Converter{cfg: &Config{PandocPath: bin}, workDir: workDir}
	err := c.convertDOCX(htmlPath, filepath.Join(workDir, "out.docx"))
	if err == nil {
		t.Fatal("expected error when pandoc exits non-zero")
	}
	if !strings.Contains(err.Error(), "pandoc DOCX conversion failed") {
		t.Errorf("unexpected error: %v", err)
	}
}

// makeFakePandoc writes an executable no-op pandoc stub and returns its path.
func makeFakePandoc(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	writeShellScript(t, path, 0, "")
	return path
}

// makeArgsRecordingPandoc writes a pandoc stub that records its arguments,
// one per line, to argsFile and exits successfully.
func makeArgsRecordingPandoc(t *testing.T, argsFile string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pandoc")
	script := "#!/bin/sh\nfor a in \"$@\"; do echo \"$a\"; done > " + argsFile + "\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write pandoc stub: %v", err)
	}
	return path
}
