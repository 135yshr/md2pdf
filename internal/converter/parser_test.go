package converter

import (
	"strings"
	"testing"
)

func TestParseMarkdown_BasicHTML(t *testing.T) {
	src := []byte("# Hello\n\nThis is a paragraph.\n")
	doc, err := parseMarkdown(src)
	if err != nil {
		t.Fatalf("parseMarkdown() error: %v", err)
	}
	if !strings.Contains(doc.HTML, "<h1") {
		t.Errorf("expected <h1> in output, got: %s", doc.HTML)
	}
	if !strings.Contains(doc.HTML, "paragraph") {
		t.Errorf("expected paragraph text in output, got: %s", doc.HTML)
	}
	if len(doc.mermaidBlocks) != 0 {
		t.Errorf("expected 0 mermaid blocks, got %d", len(doc.mermaidBlocks))
	}
}

func TestParseMarkdown_MermaidExtraction(t *testing.T) {
	src := []byte("# Diagram\n\n```mermaid\nflowchart TD\n  A --> B\n```\n")
	doc, err := parseMarkdown(src)
	if err != nil {
		t.Fatalf("parseMarkdown() error: %v", err)
	}
	if len(doc.mermaidBlocks) != 1 {
		t.Fatalf("expected 1 mermaid block, got %d", len(doc.mermaidBlocks))
	}
	block := doc.mermaidBlocks[0]
	if !strings.Contains(block.Source, "flowchart") {
		t.Errorf("unexpected mermaid source: %q", block.Source)
	}
	// Placeholder should appear in HTML, raw <pre><code> should not.
	if strings.Contains(doc.HTML, "<pre><code") && strings.Contains(doc.HTML, "flowchart") {
		t.Errorf("mermaid block was not replaced by placeholder in HTML")
	}
	placeholder := "<!--" + block.Placeholder + "-->"
	if !strings.Contains(doc.HTML, placeholder) {
		t.Errorf("placeholder %q not found in HTML: %s", placeholder, doc.HTML)
	}
}

func TestParseMarkdown_MultipleMermaid(t *testing.T) {
	src := []byte(`# Multi

` + "```mermaid\nsequenceDiagram\n  A->>B: Hi\n```" + `

Some text.

` + "```mermaid\nflowchart TD\n  X --> Y\n```" + `
`)
	doc, err := parseMarkdown(src)
	if err != nil {
		t.Fatalf("parseMarkdown() error: %v", err)
	}
	if len(doc.mermaidBlocks) != 2 {
		t.Fatalf("expected 2 mermaid blocks, got %d", len(doc.mermaidBlocks))
	}
}

func TestParseMarkdown_GFMTable(t *testing.T) {
	src := []byte("| A | B |\n|---|---|\n| 1 | 2 |\n")
	doc, err := parseMarkdown(src)
	if err != nil {
		t.Fatalf("parseMarkdown() error: %v", err)
	}
	if !strings.Contains(doc.HTML, "<table") {
		t.Errorf("expected <table> in output for GFM table, got: %s", doc.HTML)
	}
}

func TestFirstHeading(t *testing.T) {
	tests := []struct {
		name     string
		html     string
		expected string
	}{
		{
			name:     "simple h1",
			html:     "<h1>Hello World</h1>",
			expected: "Hello World",
		},
		{
			name:     "h1 with id attribute",
			html:     `<h1 id="foo">My Title</h1>`,
			expected: "My Title",
		},
		{
			name:     "h1 with inner anchor",
			html:     `<h1 id="foo"><a href="#foo">Section</a></h1>`,
			expected: "Section",
		},
		{
			name:     "no h1",
			html:     "<p>no heading</p>",
			expected: "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			d := &parsedDoc{HTML: tc.html}
			got := d.firstHeading()
			if got != tc.expected {
				t.Errorf("firstHeading() = %q, want %q", got, tc.expected)
			}
		})
	}
}

func TestHTMLEscape(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"<b>", "&lt;b&gt;"},
		{"a & b", "a &amp; b"},
		{"normal text", "normal text"},
	}
	for _, tc := range tests {
		got := htmlEscape(tc.input)
		if got != tc.expected {
			t.Errorf("htmlEscape(%q) = %q, want %q", tc.input, got, tc.expected)
		}
	}
}
