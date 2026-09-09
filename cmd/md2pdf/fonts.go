package main

import (
	"os"
	"path/filepath"
	"strings"
)

// Noto Sans CJK ships under several names depending on how it was installed,
// and md2pdf previously looked for only one of them:
//
//   - font-noto-sans-cjk (Homebrew cask) installs a single collection,
//     NotoSansCJK.ttc, with no weight in the filename
//   - font-noto-sans-cjk-jp installs per-weight files, NotoSansCJKjp-Regular.otf
//     and siblings
//   - Debian and Ubuntu package it as NotoSansCJK-Regular.ttc
//
// Weighted names come first so a collection is only used when nothing more
// specific is available: a .ttc holds every weight in one file, and CSS loading
// it by URL gets the first face rather than the requested weight.
var cjkFontNames = []string{
	"NotoSansCJK-Regular.ttc",
	"NotoSansCJKjp-Regular.otf",
	"NotoSansJP-Regular.otf",
	"NotoSansCJK.ttc",
	"NotoSansCJKjp.ttc",
}

// fontSearchDirs are the directories searched for a CJK font, in order. home is
// the user's home directory, taken as an argument so the list is testable.
//
// ~/Library/Fonts comes first because that is where a Homebrew font cask
// installs on macOS — the location whose absence made -doctor report an
// installed font as missing.
func fontSearchDirs(home string) []string {
	dirs := []string{}
	if home != "" {
		dirs = append(dirs, filepath.Join(home, "Library", "Fonts"))
	}
	return append(dirs,
		// macOS, system-wide
		"/Library/Fonts",
		"/System/Library/Fonts",
		// Linux (Debian/Ubuntu)
		"/usr/share/fonts/opentype/noto",
		"/usr/share/fonts/truetype/noto",
		"/usr/share/fonts/noto-cjk",
		"/usr/local/share/fonts/noto",
		// Homebrew on Linux and Intel macOS
		"/opt/homebrew/share/fonts/noto-cjk",
		"/usr/local/share/fonts/noto-cjk",
	)
}

// findCJKFont returns the first regular-weight Noto Sans CJK file found in dirs,
// or an empty string when none is installed. An empty result is not an error:
// md2pdf then emits no @font-face and the browser falls back to a system font.
func findCJKFont(dirs []string) string {
	for _, dir := range dirs {
		for _, name := range cjkFontNames {
			path := filepath.Join(dir, name)
			if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
				return path
			}
		}
	}
	return ""
}

// deriveFontWeight returns the file for a weight given the regular-weight path,
// falling back to the regular file when there is no separate one.
//
// Per-weight installs name their files NotoSansCJKjp-Bold.otf and so on, so the
// weight can be substituted. A collection has no weight in its name and carries
// every weight inside, so reusing it is the only option.
func deriveFontWeight(regular, weight string) string {
	if regular == "" {
		return ""
	}
	if !strings.Contains(regular, "Regular") {
		return regular
	}
	candidate := strings.ReplaceAll(regular, "Regular", weight)
	if info, err := os.Stat(candidate); err == nil && info.Mode().IsRegular() {
		return candidate
	}
	return regular
}

// userHomeDir returns the home directory, or an empty string when it cannot be
// determined, which only drops the per-user font directory from the search.
func userHomeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return home
}
