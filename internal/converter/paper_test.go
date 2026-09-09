package converter

import (
	"math"
	"strings"
	"testing"
)

// closeTo compares inch measurements, which are floats derived from millimeter
// definitions and so never exactly representable.
func closeTo(got, want float64) bool {
	return math.Abs(got-want) < 0.001
}

func TestPaperSizeInches(t *testing.T) {
	tests := []struct {
		name       string
		size       string
		wantW      float64
		wantH      float64
		wantErrSub string
	}{
		// ISO 216: A4 is 210x297mm, A3 is 297x420mm. 1in = 25.4mm.
		{name: "A4", size: "A4", wantW: 8.2677, wantH: 11.6929},
		{name: "A3", size: "A3", wantW: 11.6929, wantH: 16.5354},
		{name: "Letter", size: "Letter", wantW: 8.5, wantH: 11},
		{name: "lowercase is accepted", size: "a4", wantW: 8.2677, wantH: 11.6929},
		{name: "mixed case is accepted", size: "lEtTeR", wantW: 8.5, wantH: 11},
		{name: "surrounding spaces are trimmed", size: " A4 ", wantW: 8.2677, wantH: 11.6929},
		{name: "empty defaults to A4", size: "", wantW: 8.2677, wantH: 11.6929},
		{name: "unknown size names itself", size: "B5", wantErrSub: "B5"},
		{name: "unknown size lists the options", size: "B5", wantErrSub: "A4"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, h, err := paperSizeInches(tc.size)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("paperSizeInches(%q) = (%v, %v), want an error", tc.size, w, h)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("paperSizeInches(%q) error = %v", tc.size, err)
			}
			if !closeTo(w, tc.wantW) || !closeTo(h, tc.wantH) {
				t.Errorf("paperSizeInches(%q) = (%.4f, %.4f), want (%.4f, %.4f)",
					tc.size, w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestLengthInches(t *testing.T) {
	tests := []struct {
		name       string
		value      string
		want       float64
		wantErrSub string
	}{
		{name: "millimeters", value: "18mm", want: 0.70866},
		{name: "the default left margin", value: "14mm", want: 0.55118},
		{name: "inches", value: "1in", want: 1},
		{name: "fractional inches", value: "0.5in", want: 0.5},
		{name: "centimetres", value: "2cm", want: 0.78740},
		{name: "points", value: "36pt", want: 0.5},
		{name: "CSS pixels", value: "96px", want: 1},
		{name: "a bare number is CSS pixels", value: "48", want: 0.5},
		{name: "zero", value: "0", want: 0},
		{name: "zero with a unit", value: "0mm", want: 0},
		{name: "spaces are tolerated", value: " 18 mm ", want: 0.70866},
		{name: "uppercase units", value: "18MM", want: 0.70866},
		{name: "empty means zero", value: "", want: 0},

		{name: "an unknown unit names the value", value: "18furlong", wantErrSub: "18furlong"},
		{name: "text is rejected", value: "wide", wantErrSub: "wide"},
		{name: "a negative length is rejected", value: "-5mm", wantErrSub: "negative"},
		{name: "a missing number is rejected", value: "mm", wantErrSub: "mm"},
		// ParseFloat accepts these, so they need rejecting explicitly.
		{name: "NaN is rejected", value: "NaN", wantErrSub: "finite"},
		{name: "positive infinity is rejected", value: "+Inf", wantErrSub: "finite"},
		{name: "negative infinity is rejected", value: "-Inf", wantErrSub: "finite"},
		{name: "infinity with a unit is rejected", value: "Infmm", wantErrSub: "finite"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := lengthInches(tc.value)
			if tc.wantErrSub != "" {
				if err == nil {
					t.Fatalf("lengthInches(%q) = %v, want an error", tc.value, got)
				}
				if !strings.Contains(err.Error(), tc.wantErrSub) {
					t.Errorf("error %q does not contain %q", err, tc.wantErrSub)
				}
				return
			}
			if err != nil {
				t.Fatalf("lengthInches(%q) error = %v", tc.value, err)
			}
			if !closeTo(got, tc.want) {
				t.Errorf("lengthInches(%q) = %.5f, want %.5f", tc.value, got, tc.want)
			}
		})
	}
}
