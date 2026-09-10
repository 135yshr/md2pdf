package converter

import (
	"strings"
	"testing"
)

// TestPagerCanPan pins which pagers can scroll horizontally. Only less is
// known to chop long lines and scroll right; anything else would fold or
// truncate an over-wide diagram with no way to reach its right edge.
func TestPagerCanPan(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want bool
	}{
		{"less", []string{"less", "-R", "-F"}, true},
		{"absolute less", []string{"/usr/bin/less", "-R"}, true},
		{"more", []string{"more"}, false},
		{"cat", []string{"/bin/cat"}, false},
		{"bat", []string{"bat", "--paging=always"}, false},
		{"empty", nil, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := pagerCanPan(tc.argv); got != tc.want {
				t.Errorf("pagerCanPan(%v) = %t, want %t", tc.argv, got, tc.want)
			}
		})
	}
}

// TestEnsureLessNoWrap covers the -S that turns folding into horizontal
// scrolling. Without it less wraps an over-wide diagram into fragments, which
// is the state fitting set out to avoid.
func TestEnsureLessNoWrap(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"adds -S", []string{"less", "-R"}, []string{"less", "-R", "-S"}},
		{"keeps an existing -S", []string{"less", "-S"}, []string{"less", "-S"}},
		{"keeps -S in a cluster", []string{"less", "-RS"}, []string{"less", "-RS"}},
		{"keeps --chop-long-lines", []string{"less", "--chop-long-lines"}, []string{"less", "--chop-long-lines"}},
		// -s squeezes blank lines and is not -S; the check must be case
		// sensitive or an over-wide diagram would still fold.
		{"does not mistake -s for -S", []string{"less", "-s"}, []string{"less", "-s", "-S"}},
		{"leaves other pagers alone", []string{"more"}, []string{"more"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := ensureLessNoWrap(append([]string(nil), tc.argv...))
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("ensureLessNoWrap(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}

// TestStripQuitIfOneScreen covers -F, which makes less print the file and exit
// when it fits on one screen. That leaves the terminal to fold the wide lines
// with no chance to scroll, so panning has to give it up.
func TestStripQuitIfOneScreen(t *testing.T) {
	tests := []struct {
		name string
		argv []string
		want []string
	}{
		{"drops a standalone -F", []string{"less", "-R", "-F"}, []string{"less", "-R"}},
		{"drops --quit-if-one-screen", []string{"less", "--quit-if-one-screen"}, []string{"less"}},
		{"drops F from a cluster", []string{"less", "-RF"}, []string{"less", "-R"}},
		{"drops a cluster that was only F", []string{"less", "-F", "-R"}, []string{"less", "-R"}},
		{"leaves argv without -F alone", []string{"less", "-R", "-S"}, []string{"less", "-R", "-S"}},
		{"leaves other pagers alone", []string{"more", "-F"}, []string{"more", "-F"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := stripQuitIfOneScreen(append([]string(nil), tc.argv...))
			if strings.Join(got, " ") != strings.Join(tc.want, " ") {
				t.Errorf("stripQuitIfOneScreen(%v) = %v, want %v", tc.argv, got, tc.want)
			}
		})
	}
}
