package report

import (
	"strings"

	"golang.org/x/image/font"
	"golang.org/x/image/font/sfnt"
	"golang.org/x/image/math/fixed"
)

const (
	coverCompanyMaximumFontSize = 20.0
	coverCompanyMinimumFontSize = 13.0
	coverCompanyFontStep        = 0.5
	coverCompanyLineWidth       = 360.0
	coverCompanyMaximumLines    = 2.0
)

// fitCoverCompanyFontSize uses the bundled Noto Sans SC glyph advances to fit
// the dynamic company name into at most two Figma-sized cover lines.
//
// Parameters: fontBytes is the verified Noto Regular font and text is the
// company name. Return value is the font size in points.
func fitCoverCompanyFontSize(fontBytes []byte, text string) float64 {
	value := strings.TrimSpace(text)
	if value == "" {
		return coverCompanyMaximumFontSize
	}
	for size := coverCompanyMaximumFontSize; size >= coverCompanyMinimumFontSize; size -= coverCompanyFontStep {
		width, err := notoTextWidth(fontBytes, value, size)
		if err == nil && width <= coverCompanyLineWidth*coverCompanyMaximumLines {
			return size
		}
	}
	return coverCompanyMinimumFontSize
}

// notoTextWidth measures text with the bundled Noto glyph advances. Parameters
// are verified font bytes, text and point size. Returns width in points or a
// font parsing error.
func notoTextWidth(fontBytes []byte, text string, size float64) (float64, error) {
	parsed, err := sfnt.Parse(fontBytes)
	if err != nil {
		return 0, err
	}
	var buffer sfnt.Buffer
	ppem := fixed.Int26_6(size * 64)
	total := fixed.Int26_6(0)
	for _, character := range text {
		glyph, err := parsed.GlyphIndex(&buffer, character)
		if err != nil {
			return 0, err
		}
		advance, err := parsed.GlyphAdvance(&buffer, glyph, ppem, font.HintingNone)
		if err != nil {
			return 0, err
		}
		total += advance
	}
	return float64(total) / 64, nil
}
