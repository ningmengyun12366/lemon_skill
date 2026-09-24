package report

// reportColor stores one shared visual token for DOCX and PDF renderers.
// Hex is the OOXML value; R, G, and B are the PDF drawing values.
type reportColor struct {
	Hex     string
	R, G, B uint8
}

var (
	reportInkColor            = reportColor{Hex: "242A33", R: 36, G: 42, B: 51}
	reportPrimaryBlueColor    = reportColor{Hex: "3D7FFF", R: 61, G: 127, B: 255}
	reportPaleBlueColor       = reportColor{Hex: "D8E5FF", R: 216, G: 229, B: 255}
	reportChapterFillColor    = reportColor{Hex: "F6F9FF", R: 246, G: 249, B: 255}
	reportHeaderFillColor     = reportColor{Hex: "F9FBFF", R: 249, G: 251, B: 255}
	reportTableHeaderColor    = reportColor{Hex: "EFF2F7", R: 239, G: 242, B: 247}
	reportTableTextColor      = reportColor{Hex: "333333", R: 51, G: 51, B: 51}
	reportTableBorderColor    = reportColor{Hex: "CFD7E3", R: 207, G: 215, B: 227}
	reportTableGridColor      = reportColor{Hex: "DCE2EB", R: 220, G: 226, B: 235}
	reportTableRowColor       = reportColor{Hex: "FFFFFF", R: 255, G: 255, B: 255}
	reportTableAlternateColor = reportColor{Hex: "FCFDFF", R: 252, G: 253, B: 255}
	reportRiskAttentionColor  = reportColor{Hex: "EEF4FA", R: 238, G: 244, B: 250}
	reportOverallRiskFill     = reportColor{Hex: "FFF8F2", R: 255, G: 248, B: 242}
	reportOverallRiskText     = reportColor{Hex: "FF5C00", R: 255, G: 92, B: 0}
	reportTOCPageColor        = reportColor{Hex: "949494", R: 148, G: 148, B: 148}
	reportTOCLeaderColor      = reportColor{Hex: "999999", R: 153, G: 153, B: 153}
	reportSubheadingLineColor = reportColor{Hex: "BBD2FF", R: 187, G: 210, B: 255}
)
