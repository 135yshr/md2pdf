package cli

import (
	"strings"
	"testing"

	"github.com/135yshr/md2pdf/internal/converter"
)

func TestPrintUsageForMD2PDF(t *testing.T) {
	p, stdout, stderr := testProgram(defaultProgramName, converter.Report{})

	p.printUsage()

	got := stderr.String()
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want the usage on stderr only", stdout.String())
	}
	if !strings.HasPrefix(got, "\nUsage:\n  md2pdf [options] <input.md>\n") {
		t.Errorf("usage does not open with the md2pdf synopsis:\n%s", got)
	}
	for _, want := range []string{
		"default: pdf;",
		"\nExamples:\n  md2pdf document.md\n",
		"  -mermaid-render <mode>",
		"\nInputs:\n",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("usage does not contain %q", want)
		}
	}
	// The three pieces are assembled with Fprintf, so a stray verb in the text
	// would surface as a formatting error rather than a compile failure.
	if strings.Contains(got, "%!") {
		t.Errorf("usage contains a formatting error:\n%s", got)
	}
}
