package converter

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
)

// mermaidBlock holds the source text of a single Mermaid diagram and the SVG
// content that will replace it after rendering.
type mermaidBlock struct {
	// Source is the raw Mermaid diagram definition.
	Source string
	// SVGContent is populated after mmdc renders the diagram.
	SVGContent string
	// Placeholder is the unique HTML comment used to locate the block in the
	// rendered HTML so it can be swapped with the inline SVG.
	Placeholder string
}

// parsedDoc contains the Markdown rendered to HTML together with the list of
// Mermaid blocks that were extracted during parsing.
type parsedDoc struct {
	// HTML is the full Markdown rendered to HTML, with Mermaid code blocks
	// replaced by their unique placeholder comments.
	HTML string
	// mermaidBlocks is the ordered list of extracted Mermaid diagrams.
	mermaidBlocks []*mermaidBlock
}

// parseMarkdown converts raw Markdown bytes into a parsedDoc.
// Fenced code blocks with the language tag "mermaid" are extracted and
// replaced with placeholder comments so they can be swapped with SVG later.
func parseMarkdown(src []byte) (*parsedDoc, error) {
	md := goldmark.New(
		goldmark.WithExtensions(
			extension.GFM,
			extension.Table,
		),
		goldmark.WithParserOptions(
			parser.WithAutoHeadingID(),
		),
		goldmark.WithRendererOptions(
			html.WithHardWraps(),
			html.WithXHTML(),
			html.WithUnsafe(), // allow raw HTML pass-through
		),
	)

	reader := text.NewReader(src)
	doc := md.Parser().Parse(reader)

	var blocks []*mermaidBlock

	// Walk the AST to find fenced code blocks tagged as "mermaid".
	// We replace their source text with a placeholder before rendering.
	ast.Walk(doc, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		cb, ok := n.(*ast.FencedCodeBlock)
		if !ok {
			return ast.WalkContinue, nil
		}
		lang := string(cb.Language(src))
		if !strings.EqualFold(lang, "mermaid") {
			return ast.WalkContinue, nil
		}

		// Extract diagram source.
		var buf bytes.Buffer
		for i := 0; i < cb.Lines().Len(); i++ {
			line := cb.Lines().At(i)
			buf.Write(line.Value(src))
		}

		idx := len(blocks)
		placeholder := fmt.Sprintf("MERMAID_PLACEHOLDER_%d", idx)
		block := &mermaidBlock{
			Source:      buf.String(),
			Placeholder: placeholder,
		}
		blocks = append(blocks, block)

		// Replace node content with a raw HTML node containing the placeholder.
		rawHTML := ast.NewRawHTML()
		seg := text.NewSegment(0, 0)
		rawHTML.Segments.Append(seg)

		// We cannot easily mutate the AST node here, so we rely on the
		// post-processing step in renderHTML() to swap fenced blocks.
		_ = rawHTML
		return ast.WalkContinue, nil
	})

	// Render the full document to HTML.
	var htmlBuf bytes.Buffer
	if err := md.Renderer().Render(&htmlBuf, src, doc); err != nil {
		return nil, fmt.Errorf("goldmark render: %w", err)
	}

	rendered := htmlBuf.String()

	// Replace each <pre><code class="language-mermaid">…</code></pre> block with
	// its placeholder comment so the HTML builder can inject the SVG later.
	for _, b := range blocks {
		rendered = replaceMermaidBlock(rendered, b.Source, b.Placeholder)
	}

	return &parsedDoc{
		HTML:          rendered,
		mermaidBlocks: blocks,
	}, nil
}

// replaceMermaidBlock scans rendered HTML for a <pre><code …mermaid…> block
// whose content matches src and replaces it with the placeholder comment.
// This is a targeted string replacement rather than a full HTML parse.
func replaceMermaidBlock(html, src, placeholder string) string {
	// goldmark renders fenced mermaid blocks as:
	//   <pre><code class="language-mermaid">…source…</code></pre>
	// We search for the pattern and replace the entire <pre>…</pre> element.
	escaped := htmlEscape(src)
	targets := []string{
		"<pre><code class=\"language-mermaid\">" + escaped + "</code></pre>",
		"<pre><code class=\"language-mermaid\">" + src + "</code></pre>",
	}
	comment := "<!--" + placeholder + "-->"
	for _, t := range targets {
		if idx := strings.Index(html, t); idx >= 0 {
			return html[:idx] + comment + html[idx+len(t):]
		}
	}
	return html
}

// htmlEscape escapes special HTML characters in s.
func htmlEscape(s string) string {
	s = strings.ReplaceAll(s, "&", "&amp;")
	s = strings.ReplaceAll(s, "<", "&lt;")
	s = strings.ReplaceAll(s, ">", "&gt;")
	return s
}
