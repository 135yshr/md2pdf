package converter

import (
	"strings"
	"testing"
)

func TestExtractMermaidFromMarkdown_ReplacesBlockWithPlaceholder(t *testing.T) {
	src := "# Title\n\n```mermaid\nflowchart TD\n  A --> B\n```\n\nAfter.\n"

	out, blocks := extractMermaidFromMarkdown(src)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block, got %d", len(blocks))
	}
	if strings.Contains(out, "```mermaid") {
		t.Errorf("mermaid fence should be removed, got:\n%s", out)
	}
	if !strings.Contains(out, blocks[0].Placeholder) {
		t.Errorf("output should contain placeholder %q, got:\n%s", blocks[0].Placeholder, out)
	}
	if want := "flowchart TD\n  A --> B\n"; blocks[0].Source != want {
		t.Errorf("block source = %q, want %q", blocks[0].Source, want)
	}
	// Surrounding content must be preserved.
	if !strings.Contains(out, "# Title") || !strings.Contains(out, "After.") {
		t.Errorf("surrounding markdown lost, got:\n%s", out)
	}
}

func TestExtractMermaidFromMarkdown_OrdersMultipleBlocks(t *testing.T) {
	src := "```mermaid\nA\n```\n\ntext\n\n```mermaid\nB\n```\n"

	out, blocks := extractMermaidFromMarkdown(src)

	if len(blocks) != 2 {
		t.Fatalf("expected 2 blocks, got %d", len(blocks))
	}
	if blocks[0].Source != "A\n" || blocks[1].Source != "B\n" {
		t.Errorf("unexpected block sources: %q, %q", blocks[0].Source, blocks[1].Source)
	}
	first := strings.Index(out, blocks[0].Placeholder)
	second := strings.Index(out, blocks[1].Placeholder)
	if first < 0 || second < 0 || first >= second {
		t.Errorf("placeholders out of order: first=%d second=%d", first, second)
	}
}

func TestExtractMermaidFromMarkdown_PreservesNonMermaidFences(t *testing.T) {
	// A fenced code block that merely shows a mermaid example must be left
	// untouched, never extracted.
	src := "```text\n```mermaid still inside\nstays as code\n```\n"

	out, blocks := extractMermaidFromMarkdown(src)

	if len(blocks) != 0 {
		t.Fatalf("expected no extracted blocks, got %d", len(blocks))
	}
	if out != src {
		t.Errorf("non-mermaid fence altered:\ngot:  %q\nwant: %q", out, src)
	}
}

func TestExtractMermaidFromMarkdown_NoFence(t *testing.T) {
	src := "Just a paragraph.\n\nAnother one.\n"

	out, blocks := extractMermaidFromMarkdown(src)

	if len(blocks) != 0 {
		t.Errorf("expected no blocks, got %d", len(blocks))
	}
	if out != src {
		t.Errorf("plain markdown should round-trip, got %q", out)
	}
}

func TestExtractMermaidFromMarkdown_TildeFence(t *testing.T) {
	src := "~~~mermaid\ngraph LR\n~~~\n"

	out, blocks := extractMermaidFromMarkdown(src)

	if len(blocks) != 1 {
		t.Fatalf("expected 1 block from tilde fence, got %d", len(blocks))
	}
	if strings.Contains(out, "~~~") {
		t.Errorf("tilde fence should be removed, got:\n%s", out)
	}
}

func TestParseFenceOpen(t *testing.T) {
	tests := []struct {
		name     string
		line     string
		wantOK   bool
		wantLang string
	}{
		{"backtick mermaid", "```mermaid", true, "mermaid"},
		{"backtick with space", "``` mermaid", true, "mermaid"},
		{"tilde mermaid", "~~~mermaid", true, "mermaid"},
		{"plain fence", "```", true, ""},
		{"indented up to three", "   ```go", true, "go"},
		{"indented four is code", "    ```go", false, ""},
		{"not a fence", "text", false, ""},
		{"too short", "``", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, ok := parseFenceOpen(tt.line)
			if ok != tt.wantOK {
				t.Fatalf("parseFenceOpen(%q) ok = %v, want %v", tt.line, ok, tt.wantOK)
			}
			if ok && f.lang != tt.wantLang {
				t.Errorf("lang = %q, want %q", f.lang, tt.wantLang)
			}
		})
	}
}

func TestIsFenceClose(t *testing.T) {
	open := fence{char: '`', n: 3}
	if !isFenceClose("```", open) {
		t.Error("``` should close a ``` fence")
	}
	if !isFenceClose("`````", open) {
		t.Error("a longer run should close a shorter fence")
	}
	if isFenceClose("``", open) {
		t.Error("a shorter run must not close the fence")
	}
	if isFenceClose("``` info", open) {
		t.Error("a closing fence may not carry an info string")
	}
	if isFenceClose("~~~", open) {
		t.Error("a different fence character must not close")
	}
}
