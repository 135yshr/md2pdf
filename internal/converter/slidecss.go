package converter

// slideBaseCSS is the built-in stylesheet for a deck, used in place of
// baseCSS. The document stylesheet is tuned for a long portrait page — 15px
// text in a 900px column — which on a 1280×720 slide reads as a paragraph in
// the corner. Sizes follow Marp's default theme closely enough that a Marp
// deck keeps roughly the same amount of content per slide.
//
// The page rules matter as much as the looks. Every slide after the first
// breaks before itself rather than each breaking after, which would leave a
// blank page at the end; an empty slide gets a 1px min-height, without which
// Chromium collapses it and its page disappears; and body loses all margin
// and padding, or every slide would be pushed inward and its bottom would land
// on a second page.
const slideBaseCSS = `
  @page { margin: 0; }

  *, *::before, *::after { box-sizing: border-box; }

  html, body {
    margin: 0;
    padding: 0;
    background: #ffffff;
  }

  body {
    font-family: 'Noto Sans JP', -apple-system, BlinkMacSystemFont, "Segoe UI", Helvetica, Arial, sans-serif;
    color: #1f2328;
  }

  section.slide {
    position: relative;
    display: flex;
    flex-direction: column;
    justify-content: flex-start;
    overflow: hidden;
    min-height: 1px;
    padding: 64px 76px; /* the 64px is slidePaddingYPx */
    font-size: 30px;
    line-height: 1.5;
    background: #ffffff;
  }
  section.slide + section.slide { break-before: page; }

  section.slide > :first-child { margin-top: 0; }
  section.slide > :last-child { margin-bottom: 0; }

  h1, h2, h3, h4, h5, h6 {
    font-weight: 700;
    line-height: 1.25;
    margin: 0.6em 0 0.4em;
    color: #1f2328;
  }
  h1 { font-size: 1.8em; }
  h2 { font-size: 1.5em; }
  h3 { font-size: 1.25em; }
  h4, h5, h6 { font-size: 1em; }

  p, ul, ol, blockquote, table, pre { margin: 0 0 0.6em; }
  ul, ol { padding-left: 1.4em; }
  li + li { margin-top: 0.15em; }

  a { color: #0969da; text-decoration: none; }
  strong { font-weight: 700; }

  code {
    font-family: "SFMono-Regular", Consolas, "Liberation Mono", Menlo, monospace;
    font-size: 0.85em;
    background: #f6f8fa;
    border-radius: 6px;
    padding: 0.15em 0.35em;
  }
  pre {
    background: #f6f8fa;
    border-radius: 8px;
    padding: 0.8em 1em;
    font-size: 0.7em;
    line-height: 1.45;
    tab-size: 4;
    overflow: hidden;
  }
  pre code { background: none; padding: 0; font-size: 1em; }

  blockquote {
    color: #59636e;
    border-left: 0.2em solid #d0d7de;
    padding: 0 0.9em;
  }

  /* A flex item stretches to the slide's width; a table should not. */
  table { border-collapse: collapse; font-size: 0.8em; align-self: flex-start; }
  section.slide.lead table { align-self: center; }
  th, td { border: 1px solid #d0d7de; padding: 0.3em 0.7em; }
  th { background: #f6f8fa; font-weight: 700; }

  img { max-width: 100%; max-height: 100%; }

  /*
   * A Mermaid diagram shrinks to the space the slide has left for it. The
   * wrapper is a flex item that may shrink (min-height: 0 lets it go below its
   * content), and a flex item's size is definite once flexed inside a
   * container of fixed height, so the SVG's percentage max-height resolves
   * against it. mmdc writes width="100%" with the natural width as an inline
   * max-width, so a small diagram is never enlarged; the viewBox keeps the
   * aspect ratio when either limit applies.
   */
  .diagram-wrapper {
    flex: 0 1 auto;
    min-height: 0;
    display: flex;
    justify-content: center;
    margin: 0 0 0.6em;
  }
  .diagram-wrapper svg {
    max-width: 100%;
    max-height: 100%;
    height: auto;
  }

  /* lead: a title slide, centered both ways. */
  section.slide.lead {
    justify-content: center;
    text-align: center;
  }
  section.slide.lead ul, section.slide.lead ol { text-align: left; display: inline-block; }
`
