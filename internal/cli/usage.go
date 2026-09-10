package cli

import (
	"fmt"

	"github.com/135yshr/md2pdf/internal/converter"
)

// The help text is assembled from three pieces so that its bulk — the option
// and input descriptions, where the real maintenance happens — stays
// single-sourced, while the synopsis and the examples can speak to the program
// the user actually invoked. usageOptions is filled in with the default output
// format; the other two pieces with the program name.

// usageSynopsisPDF opens the help text for a program that writes a file by
// default.
const usageSynopsisPDF = `
Usage:
  %[1]s [options] <input.md>
  %[1]s [options] -                       read the document from stdin
  %[1]s -format console [options] <input.md>...   several files, in order
`

// usageOptions describes the flags and the input contract. It reads the same
// whichever name the binary was invoked under.
const usageOptions = `
Options:
  -o <path>               Output path (default: <input>.<format>, e.g.
                          <input>.html with -format html)
  -format <fmt>           Output format: pdf, html, docx or console
                          (default: %[1]s; console has the aliases term and
                          terminal; inferred from -o extension when omitted,
                          including .html and .htm)
  -css <path>             Custom CSS applied after the built-in stylesheet,
                          so its rules win. Repeatable; later files win over
                          earlier ones. Used by pdf and html output.
  -font <path>            Noto Sans CJK JP Regular font (.ttc/.ttf)
  -font-bold <path>       Noto Sans CJK JP Bold font
  -font-medium <path>     Noto Sans CJK JP Medium font
  -mmdc <path>            Path to mmdc (Mermaid CLI) binary
  -pandoc <path>          Path to pandoc binary (used for -format docx)
  -docx-font <family>     Font family for DOCX output (default: Yu Gothic)
  -puppeteer-config <f>   Path to Puppeteer JSON config for mmdc
  -page-size <size>       PDF page size: A4 (default), Letter, A3
  -margin-top <m>         Top margin    (default: 18mm)
  -margin-bottom <m>      Bottom margin (default: 18mm)
  -margin-left <m>        Left margin   (default: 14mm)
  -margin-right <m>       Right margin  (default: 14mm)
  -width <cols>           Console word-wrap width (default: terminal width,
                          capped at 120 columns)
  -style <name|path>      Console color theme: auto (default), dark, light,
                          notty, ascii, dracula, pink, tokyo-night, or a path
                          to a JSON stylesheet (env: GLAMOUR_STYLE)
  -pager                  Page console output through $PAGER on a terminal
                          (default: true; use -pager=false to disable)
  -mermaid-render <mode>  How to draw Mermaid diagrams in console output:
                          auto (default) tries an inline image on terminals
                          that support kitty, iTerm2 or Sixel, then falls back
                          to box-drawing text art, then to the source;
                          image forces inline images and fails if the terminal
                          cannot show them; ascii always draws text art;
                          source always prints the Mermaid source.
                          Inline images bypass the pager, which cannot display
                          them; text art pages normally.
  -doctor                 Report which runtime dependencies are present and
                          which output formats can run, then exit. Exits
                          non-zero when a format is blocked, so it works as a
                          check in a setup script.
  -v                      Verbose output
  -version                Print version and exit

Inputs:
  A single .md path works for every format. "-" reads the document from
  standard input instead; relative paths inside it resolve against the
  current directory, and -o becomes required for pdf and docx because
  there is no input filename to derive the output name from.
  Several paths are accepted for console output only, and render in
  order separated by a rule.
`

// usageExamplesPDF closes the help text for a program that writes a file by
// default.
const usageExamplesPDF = `
Examples:
  %[1]s document.md
  %[1]s -o report.pdf document.md
  %[1]s -format docx document.md
  %[1]s -o report.docx document.md
  %[1]s -format html document.md
  %[1]s -format html -css brand.css document.md
  %[1]s -css brand.css -css client.css -o report.pdf document.md
  %[1]s -format console document.md
  %[1]s -format console -style dark -width 100 document.md
  %[1]s -format console -pager=false document.md | cat
  %[1]s -format console -mermaid-render image document.md
  %[1]s -format console -mermaid-render ascii document.md
  %[1]s -format console -mermaid-render source document.md
  %[1]s -doctor
  %[1]s -font /path/to/NotoSansCJK-Regular.ttc document.md
  cat doc.md | %[1]s -format console -
  gh pr view 41 --json body -q .body | %[1]s -o pr.pdf -
  %[1]s -format console docs/*.md
`

// usageSynopsisConsole opens the help text for a program that renders to the
// terminal by default.
const usageSynopsisConsole = `
Usage:
  %[1]s [options] <input.md>...
  %[1]s [options] -                       read the document from stdin
  %[1]s -format pdf -o report.pdf <input.md>   write a file instead

  %[1]s is md2pdf under another name: it renders to the terminal by
  default. Naming a format with -format, or giving -o a path ending in
  .pdf, .html or .docx, writes a file instead.
`

// usageExamplesConsole closes the help text for a program that renders to the
// terminal by default.
const usageExamplesConsole = `
Examples:
  %[1]s document.md
  %[1]s -style dark -width 100 document.md
  %[1]s -pager=false document.md | cat
  %[1]s -mermaid-render image document.md
  %[1]s -mermaid-render ascii document.md
  %[1]s docs/*.md
  cat doc.md | %[1]s -
  %[1]s -o report.pdf document.md
  %[1]s -format docx document.md
  %[1]s -doctor
`

// usageParts returns the name-dependent halves of the help text for a program
// whose output defaults to defaultFormat.
func usageParts(defaultFormat string) (synopsis, examples string) {
	if defaultFormat == converter.FormatConsole {
		return usageSynopsisConsole, usageExamplesConsole
	}
	return usageSynopsisPDF, usageExamplesPDF
}

// printUsage writes a friendly usage summary to the error stream, naming the
// program as it was invoked.
func (p *program) printUsage() {
	synopsis, examples := usageParts(p.defaultFormat)
	fmt.Fprintf(p.stderr, synopsis, p.name)
	fmt.Fprintf(p.stderr, usageOptions, p.defaultFormat)
	fmt.Fprintf(p.stderr, examples, p.name)
}
