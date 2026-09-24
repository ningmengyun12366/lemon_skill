package report

import "strings"

// TableVariant identifies a Figma table component without coupling renderers
// to user-visible titles. DOCX and PDF both switch on this semantic value.
type TableVariant string

const (
	TableVariantDefault     TableVariant = ""
	TableVariantRiskSummary TableVariant = "risk_summary"
	TableVariantKeyRisk     TableVariant = "key_risk"
	TableVariantAttention   TableVariant = "attention"
	TableVariantRemediation TableVariant = "remediation"
)

var (
	riskSummaryColumnRatios = []float64{434.0 / 2081.0, 424.0 / 2081.0, 1223.0 / 2081.0}
	// Word fixes the key-risk table order as sequence, risk item, level,
	// abnormal period, and abnormal amount/situation. The component-library
	// geometry originally listed level before risk item, so its two ratios are
	// deliberately reordered here without changing the visible Word schema.
	keyRiskColumnRatios     = []float64{120.0 / 2081.0, 750.0 / 2081.0, 121.0 / 2081.0, 522.0 / 2081.0, 568.0 / 2081.0}
	remediationColumnRatios = []float64{0.07, 0.28, 0.09, 0.22, 0.34}
)

type riskTablePalette struct {
	Fill       reportColor
	Text       reportColor
	LeftBorder reportColor
}

// riskBandPalette stores the Figma alert-band fill, foreground, and border.
type riskBandPalette struct {
	Fill   reportColor
	Text   reportColor
	Border reportColor
}

// riskBandStyle returns the Figma alert-band palette for a confirmed level.
// Parameter level is a deterministic risk level. Return values are the palette
// and whether the level is supported.
func riskBandStyle(level RiskLevel) (riskBandPalette, bool) {
	switch level {
	case RiskHigh:
		return riskBandPalette{
			Fill:   reportColor{Hex: "FF3333", R: 255, G: 51, B: 51},
			Text:   reportColor{Hex: "FFFFFF", R: 255, G: 255, B: 255},
			Border: reportColor{Hex: "FF3333", R: 255, G: 51, B: 51},
		}, true
	case RiskMedium:
		return riskBandPalette{
			Fill:   reportColor{Hex: "FF991E", R: 255, G: 153, B: 30},
			Text:   reportColor{Hex: "FFFFFF", R: 255, G: 255, B: 255},
			Border: reportColor{Hex: "FF991E", R: 255, G: 153, B: 30},
		}, true
	case RiskLow:
		return riskBandPalette{
			Fill:   reportColor{Hex: "FFCE45", R: 255, G: 206, B: 69},
			Text:   reportColor{Hex: "FFFFFF", R: 255, G: 255, B: 255},
			Border: reportColor{Hex: "FFCE45", R: 255, G: 206, B: 69},
		}, true
	default:
		return riskBandPalette{}, false
	}
}

var riskTablePalettes = map[RiskLevel]riskTablePalette{
	RiskHigh: {
		Fill:       reportColor{Hex: "FFF5F4", R: 255, G: 245, B: 244},
		Text:       reportColor{Hex: "B42318", R: 180, G: 35, B: 24},
		LeftBorder: reportColor{Hex: "F97066", R: 249, G: 112, B: 102},
	},
	RiskMedium: {
		Fill:       reportColor{Hex: "FFF8EB", R: 255, G: 248, B: 235},
		Text:       reportColor{Hex: "B54708", R: 181, G: 71, B: 8},
		LeftBorder: reportColor{Hex: "FDB022", R: 253, G: 176, B: 34},
	},
	RiskLow: {
		Fill:       reportColor{Hex: "FFFDF0", R: 255, G: 253, B: 240},
		Text:       reportColor{Hex: "8A6100", R: 138, G: 97, B: 0},
		LeftBorder: reportColor{Hex: "F4C95D", R: 244, G: 201, B: 93},
	},
}

var clearTablePalette = riskTablePalette{
	Fill:       reportColor{Hex: "F1FAF5", R: 241, G: 250, B: 245},
	Text:       reportColor{Hex: "1F6F46", R: 31, G: 111, B: 70},
	LeftBorder: reportColor{Hex: "62C28B", R: 98, G: 194, B: 139},
}

// tableColumnRatios returns the Figma-measured widths for specialized tables.
// Parameter variant selects the component and columnCount validates its shape.
// The return value is nil when normal content-based widths should be used.
func tableColumnRatios(variant TableVariant, columnCount int) []float64 {
	var ratios []float64
	switch variant {
	case TableVariantRiskSummary:
		ratios = riskSummaryColumnRatios
	case TableVariantKeyRisk:
		ratios = keyRiskColumnRatios
	case TableVariantRemediation:
		ratios = remediationColumnRatios
	default:
		return nil
	}
	if len(ratios) != columnCount {
		return nil
	}
	return append([]float64(nil), ratios...)
}

// tableHasRiskColumnStyle reports whether the component colors its semantic
// risk-level column. Parameter variant selects the table component. Return
// value keeps DOCX and PDF styling decisions identical.
func tableHasRiskColumnStyle(variant TableVariant) bool {
	return variant == TableVariantRiskSummary || variant == TableVariantKeyRisk || variant == TableVariantRemediation
}

// riskTableStyle resolves a user-visible risk label to the Figma palette.
// Parameter value may contain either the full risk name or the compact grade.
// Return values are the palette and whether the value is a supported row type.
func riskTableStyle(value string) (riskTablePalette, bool) {
	value = strings.TrimSpace(value)
	switch {
	case value == overallMediumHigh:
		return riskTablePalettes[RiskMedium], true
	case value == "高" || strings.Contains(value, string(RiskHigh)):
		return riskTablePalettes[RiskHigh], true
	case value == "中" || strings.Contains(value, string(RiskMedium)):
		return riskTablePalettes[RiskMedium], true
	case value == "低" || strings.Contains(value, string(RiskLow)):
		return riskTablePalettes[RiskLow], true
	case value == "未见明显异常":
		return clearTablePalette, true
	default:
		return riskTablePalette{}, false
	}
}

// riskLevelColumn returns the column that carries the visible risk grade.
// Parameter headers is the fixed Word table schema. The return value is -1
// when the table has no risk-level column and therefore needs no risk palette.
func riskLevelColumn(headers []string) int {
	for index, header := range headers {
		switch strings.TrimSpace(header) {
		case "风险等级", "等级":
			return index
		}
	}
	return -1
}

// compactRiskLevel converts a full risk level to the single-character grade
// used by the Figma key-risk table. Parameter level is a deterministic rule
// result; the return value never invents a new grade.
func compactRiskLevel(level RiskLevel) string {
	switch level {
	case RiskHigh:
		return "高"
	case RiskMedium:
		return "中"
	case RiskLow:
		return "低"
	default:
		return "—"
	}
}
