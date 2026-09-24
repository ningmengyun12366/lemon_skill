package report

import "strings"

// validateDeliveryShape guards the audited customer-visible scope. Tax source
// rows belong in TaxFields, not in a second set of client-facing raw tables.
func validateDeliveryShape(model Model) []string {
	errors := []string{}
	for _, section := range model.Sections {
		for _, block := range section.Blocks {
			if block.Kind == BlockTable {
				if section.ID == "1.2" {
					errors = append(errors, "1.2不得重复输出风险等级表，三列表保留在1.1")
				}
				if containsAny(strings.Join(block.Headers, "|"), "税局中文字段", "原值") {
					errors = append(errors, section.ID+"包含税局取数底稿表头")
				}
				if section.ID == "1.5" && model.StartTime != "" && model.EndTime != "" {
					want := append(append([]string{"风险事项"}, completeTrendPeriods(model.StartTime, model.EndTime, true)...), "趋势")
					if !equalStringSlices(want, block.Headers) {
						errors = append(errors, "1.5必须按完整季度轴汇总")
					}
				}
				if section.ID == "5.7" && len(block.Rows) > 4 {
					errors = append(errors, "5.7只允许四类抵扣检查汇总，不得倾倒申报明细")
				}
			}
			if chart := block.Chart; chart != nil {
				if stringSet([]string{"6.1", "8.1", "8.2", "8.3", "8.4"})[section.ID] {
					for _, label := range chart.Labels {
						if strings.Contains(label, "年度") {
							errors = append(errors, section.ID+"年度值不得作为季度趋势的附加数据点")
						}
					}
				}
			}
		}
	}
	return errors
}
