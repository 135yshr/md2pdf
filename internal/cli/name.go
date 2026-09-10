package cli

import (
	"path/filepath"
	"strings"

	"github.com/135yshr/md2pdf/internal/converter"
)

// consoleProgramName is the alternative name this binary answers to, and under
// which it renders to the terminal instead of writing a file. Installing it is
// a symlink or a copy: the mechanism reads the name, not the file.
const consoleProgramName = "mdview"

// programName returns the name the binary was invoked under: argv[0] without
// its directory or any Windows executable suffix, in the spelling the caller
// used.
//
// This deliberately reads os.Args[0] rather than os.Executable: the latter
// resolves a symlink back to the file it points at, so an mdview symlink would
// report md2pdf and the whole mechanism would do nothing.
func programName(argv []string) string {
	if len(argv) == 0 {
		return defaultProgramName
	}
	base := filepath.Base(argv[0])
	if ext := filepath.Ext(base); strings.EqualFold(ext, ".exe") {
		base = strings.TrimSuffix(base, ext)
	}
	// filepath.Base never returns an empty string, but it does return "." for
	// an empty path and a separator for a path made only of separators.
	if base == "" || base == "." || base == string(filepath.Separator) {
		return defaultProgramName
	}
	return base
}

// defaultFormat returns the output format a program name implies, used when
// neither -format nor the -o extension chooses one. The mdview name renders to
// the terminal; every other name, a renamed or wrapped binary included, keeps
// md2pdf's pdf default.
//
// The comparison folds case because Windows filenames do, and folding on Unix
// too costs nothing: nobody installs an MDVIEW by accident.
func defaultFormat(name string) string {
	if strings.EqualFold(name, consoleProgramName) {
		return converter.FormatConsole
	}
	return converter.FormatPDF
}
