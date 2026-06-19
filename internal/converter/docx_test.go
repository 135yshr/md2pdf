package converter

import (
	"archive/zip"
	"bytes"
	"io"
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

func TestInjectTableBorders_InsertsBeforeCellMargins(t *testing.T) {
	const style = `<w:style w:type="table" w:default="1" w:styleId="Table">` +
		`<w:name w:val="Table" /><w:tblPr><w:tblInd w:w="0" w:type="dxa" />` +
		`<w:tblCellMar><w:left w:w="108" w:type="dxa" /></w:tblCellMar></w:tblPr></w:style>`

	got := injectTableBorders(style)

	if !strings.Contains(got, "<w:tblBorders>") {
		t.Fatalf("expected borders to be injected, got: %s", got)
	}
	bordersAt := strings.Index(got, "<w:tblBorders>")
	cellMarAt := strings.Index(got, "<w:tblCellMar>")
	if bordersAt >= cellMarAt {
		t.Errorf("tblBorders must precede tblCellMar (schema order); borders=%d cellMar=%d", bordersAt, cellMarAt)
	}
}

func TestInjectTableBorders_Idempotent(t *testing.T) {
	const style = `<w:style w:styleId="Table"><w:tblPr><w:tblBorders><w:top w:val="single"/></w:tblBorders>` +
		`<w:tblCellMar/></w:tblPr></w:style>`

	got := injectTableBorders(style)
	if strings.Count(got, "<w:tblBorders>") != 1 {
		t.Errorf("expected existing borders to be left untouched, got %d occurrences", strings.Count(got, "<w:tblBorders>"))
	}
}

func TestInjectTableBorders_LeavesUnrelatedStylesAlone(t *testing.T) {
	const style = `<w:style w:styleId="Normal"><w:name w:val="Normal" /></w:style>`
	if got := injectTableBorders(style); got != style {
		t.Errorf("non-Table style should be unchanged, got: %s", got)
	}
}

func TestAddTableBorders_PatchesStylesXMLOnly(t *testing.T) {
	stylesXML := `<w:styles><w:style w:styleId="Table"><w:tblPr><w:tblCellMar/></w:tblPr></w:style></w:styles>`
	other := "unchanged content"

	var raw bytes.Buffer
	zw := zip.NewWriter(&raw)
	mustZipEntry(t, zw, "word/styles.xml", stylesXML)
	mustZipEntry(t, zw, "word/document.xml", other)
	if err := zw.Close(); err != nil {
		t.Fatalf("close zip: %v", err)
	}

	patched, err := addTableBorders(raw.Bytes())
	if err != nil {
		t.Fatalf("addTableBorders: %v", err)
	}

	entries := readZipEntries(t, patched)
	if !strings.Contains(entries["word/styles.xml"], "<w:tblBorders>") {
		t.Errorf("styles.xml should gain borders, got: %s", entries["word/styles.xml"])
	}
	if entries["word/document.xml"] != other {
		t.Errorf("document.xml should be untouched, got: %s", entries["word/document.xml"])
	}
}

func mustZipEntry(t *testing.T, zw *zip.Writer, name, content string) {
	t.Helper()
	w, err := zw.Create(name)
	if err != nil {
		t.Fatalf("create zip entry %s: %v", name, err)
	}
	if _, err := w.Write([]byte(content)); err != nil {
		t.Fatalf("write zip entry %s: %v", name, err)
	}
}

func readZipEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open patched zip: %v", err)
	}
	out := make(map[string]string)
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open entry %s: %v", f.Name, err)
		}
		b, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read entry %s: %v", f.Name, err)
		}
		out[f.Name] = string(b)
	}
	return out
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
