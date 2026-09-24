package report

const (
	chartKindLine              = "line"
	chartKindMultiLine         = "multi_line"
	chartKindGroupedColumn     = "grouped_column"
	chartKindStackedBar        = "stacked_bar"
	chartKindHorizontalBar     = "horizontal_bar"
	chartKindDonut             = "donut"
	chartKindPie               = "pie"
	chartKindCombo             = "combo"
	chartKindDualAxis          = "dual_axis"
	chartKindRadar             = "radar"
	chartKindNetwork           = "network"
	chartKindTimeline          = "timeline"
	chartKindQuadrant          = "quadrant"
	chartKindPriorityMatrix    = "priority_matrix"
	chartAxisRight             = "right"
	chartRenderBar             = "bar"
	chartRenderLine            = "line"
	defaultChartEmptyTitle     = "数据不足"
	defaultChartEmptyMessage   = "当前接口数据不足，暂无法形成该图表。"
	noMatchingDataTitle        = "未识别到匹配数据"
	riskTotalCenterLabel       = "已完成检测"
	taxTotalCenterLabel        = "税费总额"
	chartColorPrimary          = "#3D7FFF"
	chartColorPositive         = "#28B950"
	chartColorWarning          = "#F7B500"
	chartColorAttention        = "#F86602"
	chartColorPurple           = "#7D63E8"
	chartColorPurpleBright     = "#9B6BFF"
	chartColorHighRisk         = "#FF3333"
	chartColorMediumRisk       = "#F86602"
	chartColorLowRisk          = "#F7B500"
	chartColorDonutHighRisk    = chartColorHighRisk
	chartColorDonutMediumRisk  = chartColorMediumRisk
	chartColorDonutLowRisk     = chartColorLowRisk
	chartColorModuleHighRisk   = "#CC0000"
	chartColorModuleMediumRisk = "#F86602"
	chartColorModuleLowRisk    = "#F7B500"
)

var chartSeriesPalette = []string{
	chartColorPrimary,
	chartColorPositive,
	chartColorWarning,
	chartColorAttention,
	chartColorPurple,
}

var figmaRequiredChartKinds = []string{
	chartKindDonut,
	chartKindPie,
	chartKindLine,
	chartKindMultiLine,
	chartKindGroupedColumn,
	chartKindStackedBar,
	chartKindHorizontalBar,
	chartKindCombo,
	chartKindDualAxis,
	chartKindRadar,
	chartKindNetwork,
}
