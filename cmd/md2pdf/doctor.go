package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/135yshr/md2pdf/internal/converter"
)

// runDoctor writes the environment report to out and reports whether every
// output format can run. The caller turns that into the exit status, so
// -doctor is usable as a check in a setup script.
//
// The report is assembled in full and written once, so there is a single write
// error to account for rather than one per line.
func runDoctor(out io.Writer, report converter.Report) bool {
	if _, err := io.WriteString(out, formatReport(report)); err != nil {
		return false
	}
	return report.Ready()
}

// formatReport renders the report as the text -doctor prints.
func formatReport(report converter.Report) string {
	var b strings.Builder

	width := 0
	for _, tool := range report.Tools {
		if n := len(tool.Name); n > width {
			width = n
		}
	}

	for _, tool := range report.Tools {
		state, detail := "missing", tool.Hint
		if tool.Found {
			state, detail = "ok", tool.Path
		}
		fmt.Fprintf(&b, "%-*s  %-7s  %s\n", width, tool.Name, state, detail)

		purpose := tool.Purpose
		switch {
		case tool.Conditional:
			purpose += " — only needed for " + tool.When
		case tool.Optional:
			purpose += " — output still works without it"
		}
		fmt.Fprintf(&b, "%-*s           %s\n", width, "", purpose)
	}

	b.WriteString("\n")
	for _, format := range report.Formats {
		if format.Ready {
			fmt.Fprintf(&b, "%-8s ready\n", format.Format)
			continue
		}
		fmt.Fprintf(&b, "%-8s not ready (needs %s)\n",
			format.Format, strings.Join(format.Missing, ", "))
	}

	if !report.Ready() {
		b.WriteString("\nSome formats cannot run. Install what is listed above,\n")
		b.WriteString("or use -format console, which needs no external tools.\n")
	}
	return b.String()
}
