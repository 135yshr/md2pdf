package converter

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Chromium's Page.printToPDF takes every measurement in inches, so the paper
// sizes and CSS lengths md2pdf accepts on the command line are converted here.
//
// The conversion factors are exact by definition: an inch is 25.4mm, a point is
// 1/72in, and a CSS pixel is 1/96in.
const (
	mmPerInch = 25.4
	cmPerInch = 2.54
	ptPerInch = 72.0
	pxPerInch = 96.0
)

// paperSize is a sheet's dimensions in inches.
type paperSize struct {
	widthInches  float64
	heightInches float64
}

// paperSizes are the sheets -page-size accepts, keyed by lowercase name. The
// ISO sizes are derived from their millimeter definitions: A4 is 210x297mm and
// A3 is 297x420mm.
var paperSizes = map[string]paperSize{
	"a4":     {210 / mmPerInch, 297 / mmPerInch},
	"a3":     {297 / mmPerInch, 420 / mmPerInch},
	"letter": {8.5, 11},
}

// defaultPaperSize is used when -page-size is empty.
const defaultPaperSize = "a4"

// paperSizeInches returns the width and height in inches for a paper size name.
// The comparison is case-insensitive, and an empty name selects the default.
func paperSizeInches(name string) (width, height float64, err error) {
	key := strings.ToLower(strings.TrimSpace(name))
	if key == "" {
		key = defaultPaperSize
	}
	size, ok := paperSizes[key]
	if !ok {
		return 0, 0, fmt.Errorf("unsupported page size %q: must be one of %s",
			name, strings.Join(paperSizeNames(), ", "))
	}
	return size.widthInches, size.heightInches, nil
}

// paperSizeNames lists the accepted size names in a stable order for messages.
func paperSizeNames() []string {
	names := make([]string, 0, len(paperSizes))
	for name := range paperSizes {
		names = append(names, strings.ToUpper(name))
	}
	sort.Strings(names)
	return names
}

// lengthUnits maps a CSS length suffix to how many of it make one inch.
var lengthUnits = map[string]float64{
	"mm": mmPerInch,
	"cm": cmPerInch,
	"in": 1,
	"pt": ptPerInch,
	"px": pxPerInch,
}

// lengthInches converts a CSS length such as "18mm", "1in" or "36pt" to inches.
//
// A bare number is treated as CSS pixels, matching how CSS and the Playwright
// API this replaced interpret unitless lengths. An empty value is zero, so an
// unset margin means no margin rather than an error.
func lengthInches(value string) (float64, error) {
	trimmed := strings.ToLower(strings.ReplaceAll(strings.TrimSpace(value), " ", ""))
	if trimmed == "" {
		return 0, nil
	}

	number, perInch := trimmed, pxPerInch
	for unit, factor := range lengthUnits {
		if strings.HasSuffix(trimmed, unit) {
			number, perInch = strings.TrimSuffix(trimmed, unit), factor
			break
		}
	}

	parsed, err := strconv.ParseFloat(number, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid length %q: expected a number optionally followed by %s",
			value, strings.Join(lengthUnitNames(), ", "))
	}
	if parsed < 0 {
		return 0, fmt.Errorf("invalid length %q: a negative length is not meaningful", value)
	}
	return parsed / perInch, nil
}

// lengthUnitNames lists the accepted units in a stable order for messages.
func lengthUnitNames() []string {
	names := make([]string, 0, len(lengthUnits))
	for unit := range lengthUnits {
		names = append(names, unit)
	}
	sort.Strings(names)
	return names
}
