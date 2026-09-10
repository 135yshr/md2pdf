package converter

import (
	"path/filepath"
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

// pagerArgvFor returns the pager command line for a document, adjusted for a
// diagram being shown wider than the screen.
//
// Two changes are needed together: -S so less chops the long lines instead of
// folding them, and -+F so it does not print the document and exit before the
// reader can scroll. A pager md2pdf does not recognise is left untouched —
// resolveConsolePan has already ruled out panning in that case.
//
// Both are appended rather than merged into the existing flags, because the
// existing flags cannot be parsed safely from here. In `less -PSTATUS` the S
// belongs to the prompt string, not to a cluster of boolean options, so testing
// for a letter would both miss a real -F and mangle a prompt. Appending is
// enough: less applies command-line options left to right, so a trailing -S sets
// chop-long-lines whatever came before, and -+F resets quit-if-one-screen even
// when it was set in $LESS.
func pagerArgvFor(argv []string, panned bool) []string {
	if !panned || !isLess(argv) {
		return argv
	}

	// A standalone -F is dropped only to keep the logged command line readable.
	// The flag that actually matters is the trailing -+F, which cancels the
	// option however it arrived — including through $LESS, where less also reads
	// its options and where no rewrite of argv could reach it.
	out := make([]string, 0, len(argv)+2)
	for _, arg := range argv {
		if arg == "-F" || arg == "--quit-if-one-screen" {
			continue
		}
		out = append(out, arg)
	}
	return append(out, "-S", "-+F")
}

// panBlockedReason reports whether an over-wide diagram may be shown in full,
// and why not when it may not.
//
// Two things have to hold. The output must be able to scroll sideways at all,
// and no inline image may be drawn in the same run: renderConsole skips the
// pager whenever one is, and without the pager there is nothing to scroll. The
// second condition is deliberately about what *might* be drawn rather than what
// was, because the pager is chosen only after every diagram already exists.
func panBlockedReason(canPan, mayDrawImages bool) (reason string, allowed bool) {
	switch {
	case mayDrawImages:
		return "inline images in this run bypass the pager, so nothing can scroll", false
	case !canPan:
		return "the output cannot scroll sideways", false
	default:
		return "", true
	}
}
