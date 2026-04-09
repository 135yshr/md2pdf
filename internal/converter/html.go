package converter

import (
	"fmt"
	"os"
	"strings"
	"text/template"
)

// htmlTemplate is the full GitHub-styled page template.
// Mermaid SVGs are injected inline; fonts are referenced via local file:// URLs.
const htmlTemplate = `<!DOCTYPE html>
<html lang="ja">
<head>
<meta charset="UTF-8">
<title>{{.Title}}</title>
<style>
{{.CSS}}
</style>
</head>
<body>
{{.Body}}
</body>
</html>`

// htmlData holds the data passed to htmlTemplate.
type htmlData struct {
	Title string
	CSS   string
	Body  string
}

// buildHTML assembles the final HTML file at destPath from the parsed document.
// Each Mermaid placeholder comment is replaced with its rendered SVG.
func (c *Converter) buildHTML(doc *parsedDoc, destPath string) error {
	css := c.buildCSS()

	body := doc.HTML
	for _, block := range doc.mermaidBlocks {
		wrapped := `<div class="diagram-wrapper">` + "\n" + block.SVGContent + "\n</div>"
		comment := "<!--" + block.Placeholder + "-->"
		body = strings.ReplaceAll(body, comment, wrapped)
	}

	tmpl, err := template.New("page").Parse(htmlTemplate)
	if err != nil {
		return fmt.Errorf("parse html template: %w", err)
	}

	title := strings.TrimSuffix(doc.firstHeading(), "")
	if title == "" {
		title = "Document"
	}

	var sb strings.Builder
	if err := tmpl.Execute(&sb, htmlData{
		Title: title,
		CSS:   css,
		Body:  body,
	}); err != nil {
		return fmt.Errorf("execute html template: %w", err)
	}

	if err := os.WriteFile(destPath, []byte(sb.String()), 0o644); err != nil {
		return fmt.Errorf("write html: %w", err)
	}
	return nil
}

// buildCSS returns the complete stylesheet string, with @font-face declarations
// pointing at the configured local font files.
func (c *Converter) buildCSS() string {
	var fontFaces string
	if c.cfg.FontRegular != "" {
		fontFaces += fontFace("Noto Sans JP", 400, c.cfg.FontRegular)
	}
	if c.cfg.FontMedium != "" {
		fontFaces += fontFace("Noto Sans JP", 500, c.cfg.FontMedium)
	}
	if c.cfg.FontBold != "" {
		fontFaces += fontFace("Noto Sans JP", 700, c.cfg.FontBold)
	}

	return fontFaces + baseCSS
}

// fontFace returns a single @font-face rule for the given family, weight and path.
func fontFace(family string, weight int, path string) string {
	return fmt.Sprintf(`
  @font-face {
    font-family: '%s';
    font-weight: %d;
    font-style: normal;
    src: url('file://%s') format('truetype');
  }
`, family, weight, path)
}

// baseCSS is the GitHub-flavored Markdown stylesheet.
const baseCSS = `
  * { box-sizing: border-box; margin: 0; padding: 0; }

  body {
    font-family: 'Noto Sans JP', -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
    font-size: 15px;
    line-height: 1.75;
    color: #1f2328;
    background: #ffffff;
    padding: 36px 44px;
    max-width: 900px;
    margin: 0 auto;
  }

  h1 {
    font-family: 'Noto Sans JP', sans-serif;
    font-size: 1.85em;
    font-weight: 700;
    color: #1f2328;
    border-bottom: 2px solid #d0d7de;
    padding-bottom: 10px;
    margin-top: 32px;
    margin-bottom: 20px;
  }

  h2 {
    font-family: 'Noto Sans JP', sans-serif;
    font-size: 1.4em;
    font-weight: 700;
    color: #1f2328;
    border-bottom: 1px solid #d0d7de;
    padding-bottom: 7px;
    margin-top: 36px;
    margin-bottom: 16px;
  }

  h3 {
    font-family: 'Noto Sans JP', sans-serif;
    font-size: 1.15em;
    font-weight: 600;
    color: #1f2328;
    margin-top: 24px;
    margin-bottom: 10px;
  }

  h4, h5, h6 {
    font-family: 'Noto Sans JP', sans-serif;
    font-size: 1em;
    font-weight: 600;
    color: #1f2328;
    margin-top: 20px;
    margin-bottom: 8px;
  }

  p {
    font-family: 'Noto Sans JP', sans-serif;
    margin-bottom: 14px;
    color: #1f2328;
  }

  a { color: #0969da; text-decoration: none; }
  a:hover { text-decoration: underline; }

  blockquote {
    border-left: 4px solid #d0d7de;
    padding: 6px 16px;
    color: #656d76;
    background: #f6f8fa;
    margin: 14px 0;
    border-radius: 0 6px 6px 0;
  }
  blockquote p {
    margin-bottom: 0;
    color: #656d76;
    font-size: 13.5px;
    font-family: 'Noto Sans JP', sans-serif;
  }

  table {
    width: 100%;
    border-collapse: collapse;
    margin: 14px 0;
    font-size: 13.5px;
    font-family: 'Noto Sans JP', sans-serif;
  }
  th {
    background: #f6f8fa;
    font-weight: 600;
    text-align: left;
    padding: 8px 13px;
    border: 1px solid #d0d7de;
    color: #1f2328;
  }
  td {
    padding: 8px 13px;
    border: 1px solid #d0d7de;
    color: #1f2328;
  }
  tr:nth-child(even) td { background: #f6f8fa; }

  code {
    font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 85%;
    background: rgba(175, 184, 193, 0.2);
    padding: 2px 5px;
    border-radius: 5px;
    color: #1f2328;
  }

  pre {
    background: #f6f8fa;
    border: 1px solid #d0d7de;
    border-radius: 6px;
    padding: 16px;
    overflow-x: auto;
    margin: 14px 0;
  }
  pre code {
    background: none;
    padding: 0;
    font-size: 13px;
    line-height: 1.5;
  }

  ul, ol {
    padding-left: 2em;
    margin-bottom: 14px;
    font-family: 'Noto Sans JP', sans-serif;
  }
  li { margin-bottom: 5px; }

  hr {
    border: none;
    border-top: 1px solid #d0d7de;
    margin: 28px 0;
  }

  strong { font-weight: 700; }
  em     { font-style: italic; }

  img {
    max-width: 100%;
    height: auto;
  }

  /* Mermaid diagram container */
  .diagram-wrapper {
    background: #f6f8fa;
    border: 1px solid #d0d7de;
    border-radius: 8px;
    padding: 20px;
    margin: 16px 0;
    text-align: center;
    overflow: hidden;
  }
  .diagram-wrapper svg {
    max-width: 100%;
    height: auto;
    display: block;
    margin: 0 auto;
  }
`

// firstHeading extracts the text content of the first <h1> element from the
// rendered HTML, used as the document title.
func (d *parsedDoc) firstHeading() string {
	html := d.HTML
	start := strings.Index(html, "<h1")
	if start < 0 {
		return ""
	}
	tagEnd := strings.Index(html[start:], ">")
	if tagEnd < 0 {
		return ""
	}
	inner := html[start+tagEnd+1:]
	end := strings.Index(inner, "</h1>")
	if end < 0 {
		return ""
	}
	// Strip any inner tags (e.g. anchor links added by goldmark).
	raw := inner[:end]
	result := strings.Builder{}
	inTag := false
	for _, r := range raw {
		switch {
		case r == '<':
			inTag = true
		case r == '>':
			inTag = false
		case !inTag:
			result.WriteRune(r)
		}
	}
	return strings.TrimSpace(result.String())
}
