package main

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// fontFixture creates dir and the named font files inside it, returning dir.
func fontFixture(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("font"), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
	}
	return dir
}

func TestFindCJKFont(t *testing.T) {
	tests := []struct {
		name  string
		files []string
		want  string
	}{
		{
			// The layout on the reporter's machine: font-noto-sans-cjk installs a
			// single collection with no weight in the filename.
			name:  "NotoSansCJK.ttc collection",
			files: []string{"NotoSansCJK.ttc"},
			want:  "NotoSansCJK.ttc",
		},
		{
			// font-noto-sans-cjk-jp, which the README recommends, installs
			// per-weight .otf files instead.
			name:  "per-weight otf files",
			files: []string{"NotoSansCJKjp-Regular.otf", "NotoSansCJKjp-Bold.otf"},
			want:  "NotoSansCJKjp-Regular.otf",
		},
		{
			name:  "the Linux packaged name",
			files: []string{"NotoSansCJK-Regular.ttc"},
			want:  "NotoSansCJK-Regular.ttc",
		},
		{
			name:  "a weighted file is preferred over the bare collection",
			files: []string{"NotoSansCJK.ttc", "NotoSansCJK-Regular.ttc"},
			want:  "NotoSansCJK-Regular.ttc",
		},
		{
			name:  "an unrelated font is ignored",
			files: []string{"Helvetica.ttc"},
			want:  "",
		},
		{
			name:  "no fonts at all",
			files: nil,
			want:  "",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := fontFixture(t, tc.files...)
			got := findCJKFont([]string{dir})

			if tc.want == "" {
				if got != "" {
					t.Errorf("findCJKFont() = %q, want no font", got)
				}
				return
			}
			if got != filepath.Join(dir, tc.want) {
				t.Errorf("findCJKFont() = %q, want %q", got, filepath.Join(dir, tc.want))
			}
		})
	}
}

// TestFindCJKFont_SearchesDirectoriesInOrder checks an earlier directory wins,
// so an explicit system location is not shadowed by a later fallback.
func TestFindCJKFont_SearchesDirectoriesInOrder(t *testing.T) {
	first := fontFixture(t, "NotoSansCJK-Regular.ttc")
	second := fontFixture(t, "NotoSansCJK.ttc")

	got := findCJKFont([]string{first, second})
	if want := filepath.Join(first, "NotoSansCJK-Regular.ttc"); got != want {
		t.Errorf("findCJKFont() = %q, want %q", got, want)
	}
}

func TestFindCJKFont_SkipsMissingDirectories(t *testing.T) {
	real := fontFixture(t, "NotoSansCJK.ttc")
	got := findCJKFont([]string{"/definitely/not/here", real})
	if want := filepath.Join(real, "NotoSansCJK.ttc"); got != want {
		t.Errorf("findCJKFont() = %q, want %q", got, want)
	}
}

// TestFontSearchDirs_IncludesMacOSUserFonts is the regression guard for the
// reported bug: the cask installs into ~/Library/Fonts, which was not searched.
func TestFontSearchDirs_IncludesMacOSUserFonts(t *testing.T) {
	home := t.TempDir()
	dirs := fontSearchDirs(home)

	want := filepath.Join(home, "Library", "Fonts")
	found := false
	for _, dir := range dirs {
		if dir == want {
			found = true
		}
	}
	if !found {
		t.Errorf("font search directories %v do not include %q, "+
			"which is where a Homebrew font cask installs on macOS", dirs, want)
	}
}

func TestFontSearchDirs_IncludesSystemAndLinuxLocations(t *testing.T) {
	dirs := fontSearchDirs("/home/someone")

	for _, want := range []string{
		"/home/someone/Library/Fonts",
		"/Library/Fonts",
		"/usr/share/fonts/opentype/noto",
		"/usr/share/fonts/truetype/noto",
	} {
		found := false
		for _, dir := range dirs {
			if dir == want {
				found = true
			}
		}
		if !found {
			t.Errorf("font search directories are missing %q: %v", want, dirs)
		}
	}
}

func TestDeriveFontWeight(t *testing.T) {
	tests := []struct {
		name    string
		regular string
		weight  string
		present []string
		want    string
	}{
		{
			name:    "per-weight otf resolves to its sibling",
			regular: "NotoSansCJKjp-Regular.otf",
			weight:  "Bold",
			present: []string{"NotoSansCJKjp-Regular.otf", "NotoSansCJKjp-Bold.otf"},
			want:    "NotoSansCJKjp-Bold.otf",
		},
		{
			name:    "medium too",
			regular: "NotoSansCJKjp-Regular.otf",
			weight:  "Medium",
			present: []string{"NotoSansCJKjp-Regular.otf", "NotoSansCJKjp-Medium.otf"},
			want:    "NotoSansCJKjp-Medium.otf",
		},
		{
			// A .ttc collection carries every weight in one file, so there is no
			// sibling to find and reusing it is correct.
			name:    "a bare collection falls back to itself",
			regular: "NotoSansCJK.ttc",
			weight:  "Bold",
			present: []string{"NotoSansCJK.ttc"},
			want:    "NotoSansCJK.ttc",
		},
		{
			name:    "a missing sibling falls back to regular",
			regular: "NotoSansCJK-Regular.ttc",
			weight:  "Bold",
			present: []string{"NotoSansCJK-Regular.ttc"},
			want:    "NotoSansCJK-Regular.ttc",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := fontFixture(t, tc.present...)
			got := deriveFontWeight(filepath.Join(dir, tc.regular), tc.weight)
			if want := filepath.Join(dir, tc.want); got != want {
				t.Errorf("deriveFontWeight() = %q, want %q", got, want)
			}
		})
	}
}

func TestDeriveFontWeight_EmptyRegularStaysEmpty(t *testing.T) {
	if got := deriveFontWeight("", "Bold"); got != "" {
		t.Errorf("deriveFontWeight(\"\", …) = %q, want empty", got)
	}
}

// TestLocalFaceNames covers which fonts need an installed face named alongside
// the file. Only a weightless collection does: CSS cannot select a face inside
// one, so loading it by URL yields its first face — Thin — for every weight.
func TestLocalFaceNames(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		weight string
		want   []string
	}{
		{
			// The layout on the reporter's machine, and the reason the whole
			// document rendered in hairlines with no bold.
			name:   "the bare collection is named per weight",
			path:   "/Users/someone/Library/Fonts/NotoSansCJK.ttc",
			weight: "Bold",
			want:   []string{"Noto Sans CJK JP Bold", "NotoSansCJKjp-Bold"},
		},
		{
			name:   "regular too",
			path:   "/Users/someone/Library/Fonts/NotoSansCJK.ttc",
			weight: "Regular",
			want:   []string{"Noto Sans CJK JP Regular", "NotoSansCJKjp-Regular"},
		},
		{
			name:   "the jp collection as well",
			path:   "/usr/share/fonts/noto-cjk/NotoSansCJKjp.ttc",
			weight: "Medium",
			want:   []string{"Noto Sans CJK JP Medium", "NotoSansCJKjp-Medium"},
		},
		{
			// A weighted collection is still a collection, but its first face is
			// the weight it is named for, so the URL already resolves correctly.
			name:   "a weighted collection needs no local name",
			path:   "/usr/share/fonts/opentype/noto/NotoSansCJK-Bold.ttc",
			weight: "Bold",
			want:   nil,
		},
		{
			name:   "a per-weight file needs no local name",
			path:   "/usr/share/fonts/noto/NotoSansCJKjp-Bold.otf",
			weight: "Bold",
			want:   nil,
		},
		{
			name:   "an unknown font is left alone",
			path:   "/tmp/MyFont.ttc",
			weight: "Bold",
			want:   nil,
		},
		{
			name:   "no font at all",
			path:   "",
			weight: "Bold",
			want:   nil,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := localFaceNames(tc.path, tc.weight)
			if !slices.Equal(got, tc.want) {
				t.Errorf("localFaceNames(%q, %q) = %v, want %v", tc.path, tc.weight, got, tc.want)
			}
		})
	}
}
