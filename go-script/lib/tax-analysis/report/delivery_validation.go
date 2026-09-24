package report

import (
	"fmt"
)

// deliveryOverallRisk never equates zero confirmed findings with complete data.
func deliveryOverallRisk(model Model) string {
	return assessOverallRisk(model.Findings, model.Evaluations).Level
}

// deliveryChartValue formats counts as integers without rounding financial data.
func deliveryChartValue(chart Chart, series Series, value float64) string {
	unit := chart.Unit
	if series.Axis == "right" && chart.SecondaryUnit != "" {
		unit = chart.SecondaryUnit
	}
	if containsAny(unit, "张", "家", "项", "笔", "次", "个", "户") {
		return fmt.Sprintf("%.0f", value)
	}
	if unit == "%" || containsAny(series.Name, "税负率", "贡献率", "红字率", "毛利率", "利润率", "费用率", "集中度", "资产负债率") {
		return fmt.Sprintf("%.2f%%", value)
	}
	return fmt.Sprintf("%.2f", value)
}
