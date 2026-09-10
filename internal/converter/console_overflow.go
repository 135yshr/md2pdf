package converter

import (
	"path/filepath"
	"strings"
)

// lessBinary is the only pager md2pdf knows how to make scroll sideways.
const lessBinary = "less"

// isLess reports whether argv invokes less. resolvePager rewrites argv[0] to an
// absolute path, so the base name is what has to be compared.
func isLess(argv []string) bool {
	return len(argv) > 0 && filepath.Base(argv[0]) == lessBinary
}

// pagerCanPan reports whether the pager can show a line wider than the screen
// and let the reader scroll right to the rest of it.
//
// Only less qualifies. more folds long lines and cannot scroll horizontally at
// all, and a pager md2pdf does not recognise gets no benefit of the doubt: a
// diagram is only shown over-wide when there is a way to reach its right edge.
func pagerCanPan(argv []string) bool {
	return isLess(argv)
}

// ensureLessNoWrap appends -S when the configured pager is less without a
// chop-long-lines flag. Without it less folds an over-wide diagram into
// fragments instead of letting the reader scroll right.
//
// The check is case sensitive: less spells blank-line squeezing -s, and
// treating that as -S would leave the diagram folded.
func ensureLessNoWrap(argv []string) []string {
	if !isLess(argv) {
		return argv
	}
	for _, arg := range argv[1:] {
		switch {
		case strings.HasPrefix(arg, "--chop"):
			return argv
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.Contains(arg, "S"):
			return argv
		}
	}
	return append(argv, "-S")
}

// stripQuitIfOneScreen removes less's -F from the pager command line.
//
// -F prints the file and exits when it fits on one screen, which hands the wide
// lines to the terminal to fold with no chance to scroll. Panning is worth more
// than the convenience of not entering the pager for a short document, so the
// flag is dropped — including out of a cluster such as -RF — whenever a diagram
// is being shown over-wide.
func stripQuitIfOneScreen(argv []string) []string {
	if !isLess(argv) {
		return argv
	}
	// A fresh slice, not argv[:1]: appending into argv's own array would
	// clobber the caller's flags.
	out := make([]string, 0, len(argv))
	out = append(out, argv[0])
	for _, arg := range argv[1:] {
		switch {
		case arg == "--quit-if-one-screen":
			continue
		case strings.HasPrefix(arg, "-") && !strings.HasPrefix(arg, "--") &&
			strings.Contains(arg, "F"):
			if kept := "-" + strings.ReplaceAll(arg[1:], "F", ""); kept != "-" {
				out = append(out, kept)
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

// resolveConsolePan reports whether an over-wide diagram may be written in full
// rather than dropped to its Mermaid source.
//
// Redirected output always can: there is no terminal to fold the lines, and the
// consumer of the stream is better placed than md2pdf to decide what to do with
// a wide one. An interactive terminal can only pan through a pager that scrolls
// sideways, so the answer there follows the pager that will actually be used —
// which is why the pager has to be resolved before the diagrams are drawn.
func resolveConsolePan(pagerEnabled, isTTY bool, pagerArgv []string, pagerAvailable bool) bool {
	if !isTTY {
		return true
	}
	return pagerEnabled && pagerAvailable && pagerCanPan(pagerArgv)
}
