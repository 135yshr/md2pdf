package converter

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadInput_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Title\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	c := newTestConverter(t, &Config{Format: FormatConsole})
	got, err := c.readInput(path)
	if err != nil {
		t.Fatalf("readInput: %v", err)
	}
	if string(got) != "# Title\n" {
		t.Errorf("readInput = %q", got)
	}
}

func TestReadInput_Stdin(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatConsole})
	c.stdin = strings.NewReader("# From stdin\n")

	got, err := c.readInput(StdinPath)
	if err != nil {
		t.Fatalf("readInput: %v", err)
	}
	if string(got) != "# From stdin\n" {
		t.Errorf("readInput = %q", got)
	}
}

// TestReadInput_EmptyStdinIsAnError covers the criterion that empty input must
// not silently produce an empty document or a zero-byte file.
func TestReadInput_EmptyStdinIsAnError(t *testing.T) {
	for _, in := range []string{"", "\n", "   \n\t\n"} {
		c := newTestConverter(t, &Config{Format: FormatConsole})
		c.stdin = strings.NewReader(in)

		_, err := c.readInput(StdinPath)
		if err == nil {
			t.Errorf("readInput(%q) = nil error, want an empty-input error", in)
			continue
		}
		if !strings.Contains(err.Error(), "empty") {
			t.Errorf("error %q does not mention emptiness", err)
		}
	}
}

// TestReadInput_EmptyFileIsNotAnError is a regression guard. Before multi-input
// support an empty file rendered an empty document and exited 0, and the
// acceptance criteria require a single input path to behave as it did then. Only
// an empty *pipe* is treated as a failure — see readInput's comment for why the
// two differ.
func TestReadInput_EmptyFileIsNotAnError(t *testing.T) {
	dir := t.TempDir()
	for _, content := range []string{"", "\n", "   \n\t\n"} {
		path := filepath.Join(dir, "empty.md")
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		c := newTestConverter(t, &Config{Format: FormatConsole})
		got, err := c.readInput(path)
		if err != nil {
			t.Errorf("readInput on an empty file returned %v, want nil", err)
		}
		if string(got) != content {
			t.Errorf("readInput = %q, want %q", got, content)
		}
	}
}

func TestReadInput_MissingFile(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatConsole})
	if _, err := c.readInput(filepath.Join(t.TempDir(), "nope.md")); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestInputDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")

	got, err := inputDir(path)
	if err != nil {
		t.Fatalf("inputDir: %v", err)
	}
	want, err := filepath.Abs(dir)
	if err != nil {
		t.Fatalf("abs: %v", err)
	}
	if got != want {
		t.Errorf("inputDir(%q) = %q, want %q", path, got, want)
	}
}

// TestInputDir_StdinResolvesAgainstWorkingDirectory covers the criterion that a
// document on stdin resolves ./img.png against the current directory.
func TestInputDir_StdinResolvesAgainstWorkingDirectory(t *testing.T) {
	got, err := inputDir(StdinPath)
	if err != nil {
		t.Fatalf("inputDir: %v", err)
	}
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if got != cwd {
		t.Errorf("inputDir(%q) = %q, want the working directory %q", StdinPath, got, cwd)
	}
}

func TestJoinConsoleDocuments(t *testing.T) {
	tests := []struct {
		name      string
		docs      [][]byte
		separator []byte
		want      string
	}{
		{"no documents", nil, []byte("SEP"), ""},
		{"one document has no separator", [][]byte{[]byte("A")}, []byte("SEP"), "A"},
		{"two documents", [][]byte{[]byte("A"), []byte("B")}, []byte("SEP"), "ASEPB"},
		{"three documents", [][]byte{[]byte("A"), []byte("B"), []byte("C")}, []byte("SEP"), "ASEPBSEPC"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := joinConsoleDocuments(tc.docs, tc.separator)
			if string(got) != tc.want {
				t.Errorf("joinConsoleDocuments = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestRenderConsole_MultipleDocumentsInOrder covers the criterion that several
// files render in argument order with a separator between them.
func TestRenderConsole_MultipleDocumentsInOrder(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.md")
	second := filepath.Join(dir, "second.md")
	if err := os.WriteFile(first, []byte("# Alpha document\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := os.WriteFile(second, []byte("# Beta document\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	outPath := filepath.Join(dir, "out.txt")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{Format: FormatConsole, ConsolePager: false})
	if err := c.renderConsole([]string{first, second}, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	got := string(data)

	iAlpha := strings.Index(got, "Alpha document")
	iBeta := strings.Index(got, "Beta document")
	if iAlpha < 0 || iBeta < 0 {
		t.Fatalf("both documents must appear:\n%s", got)
	}
	if iAlpha > iBeta {
		t.Errorf("documents are out of argument order:\n%s", got)
	}
	between := got[iAlpha:iBeta]
	if !strings.Contains(between, "--") {
		t.Errorf("no separator rule between the documents:\n%q", between)
	}
}

// TestRenderConsole_SameFileTwiceRendersTwice covers the criterion that inputs
// are not implicitly deduplicated.
func TestRenderConsole_SameFileTwiceRendersTwice(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	if err := os.WriteFile(path, []byte("# Repeated heading\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	outPath := filepath.Join(dir, "out.txt")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{Format: FormatConsole, ConsolePager: false})
	if err := c.renderConsole([]string{path, path}, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if n := strings.Count(string(data), "Repeated heading"); n != 2 {
		t.Errorf("heading appears %d times, want 2:\n%s", n, data)
	}
}

// TestRenderConsole_SingleDocumentHasNoSeparator is the regression guard: one
// input must render exactly as it did before multi-input support.
func TestRenderConsole_SingleDocumentHasNoSeparator(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "doc.md")
	md := []byte("# Title\n\nBody text.\n")
	if err := os.WriteFile(path, md, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	want, err := renderConsoleMarkdown(md, "notty", consoleFallbackWidth)
	if err != nil {
		t.Fatalf("baseline: %v", err)
	}

	outPath := filepath.Join(dir, "out.txt")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{Format: FormatConsole, ConsolePager: false})
	if err := c.renderConsole([]string{path}, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}
	got, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("single-document output drifted.\n got: %q\nwant: %q", got, want)
	}
}

// TestRenderConsole_StdinDocument covers rendering a document piped in.
func TestRenderConsole_StdinDocument(t *testing.T) {
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out.txt")
	f, err := os.Create(outPath)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	defer f.Close()

	c := newTestConverter(t, &Config{Format: FormatConsole, ConsolePager: false})
	c.stdin = strings.NewReader("# Piped heading\n\nSome body.\n")

	if err := c.renderConsole([]string{StdinPath}, f); err != nil {
		t.Fatalf("renderConsole: %v", err)
	}
	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(string(data), "Piped heading") {
		t.Errorf("stdin document not rendered:\n%s", data)
	}
}

// TestConvert_EmptyStdinFailsWithoutWritingOutput checks an empty pipe does not
// leave a zero-byte output file behind.
func TestConvert_EmptyStdinFailsWithoutWritingOutput(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pdf")

	c := newTestConverter(t, &Config{Format: FormatPDF})
	c.stdin = strings.NewReader("")

	if err := c.Convert([]string{StdinPath}, out); err == nil {
		t.Fatal("expected an error for empty stdin")
	}
	if _, err := os.Stat(out); err == nil {
		t.Error("an output file was created despite the failure")
	}
}

// TestConvert_StdinToFileFormat drives the whole Convert path for a document
// read from standard input, using a stub pandoc. It covers two criteria that
// would otherwise need a real toolchain: that a file format produces output from
// stdin at all, and that relative paths inside the document resolve against the
// working directory, since stdin has no location of its own.
func TestConvert_StdinToFileFormat(t *testing.T) {
	skipOnWindows(t)

	workDir := t.TempDir()
	argsFile := filepath.Join(workDir, "args.txt")
	bin := makeArgsRecordingPandoc(t, argsFile)
	outPath := filepath.Join(workDir, "out.docx")

	c := &Converter{
		cfg:     &Config{Format: FormatDOCX, PandocPath: bin},
		workDir: workDir,
		stdin:   strings.NewReader("# Piped\n\n![local](./img.png)\n"),
	}

	if err := c.Convert([]string{StdinPath}, outPath); err != nil {
		t.Fatalf("Convert: %v", err)
	}

	recorded, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read recorded args: %v", err)
	}
	args := string(recorded)

	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if !strings.Contains(args, cwd) {
		t.Errorf("--resource-path does not include the working directory %q, so a relative\n"+
			"image in a piped document could not resolve; pandoc args: %s", cwd, args)
	}
	if !strings.Contains(args, outPath) {
		t.Errorf("pandoc was not told to write %q; args: %s", outPath, args)
	}

	// The document pandoc reads must carry what arrived on standard input.
	written, err := os.ReadFile(filepath.Join(workDir, "document.md"))
	if err != nil {
		t.Fatalf("read the document handed to pandoc: %v", err)
	}
	if !strings.Contains(string(written), "# Piped") {
		t.Errorf("stdin content did not reach pandoc: %q", written)
	}
}

// TestConvert_RejectsMultipleInputsForFileFormats guards the converter itself,
// not just the CLI, since Convert is the package's entry point.
func TestConvert_RejectsMultipleInputsForFileFormats(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatPDF})
	err := c.Convert([]string{"a.md", "b.md"}, filepath.Join(t.TempDir(), "out.pdf"))
	if err == nil {
		t.Fatal("expected an error for multiple inputs with a file format")
	}
	if !strings.Contains(err.Error(), "single input") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestConvert_RejectsNoInputs(t *testing.T) {
	c := newTestConverter(t, &Config{Format: FormatConsole})
	if err := c.Convert(nil, ""); err == nil {
		t.Error("expected an error for no inputs")
	}
}
