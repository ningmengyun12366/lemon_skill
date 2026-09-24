package report

import "fmt"

type qixinListingCounts struct{ Total, Current, Removed, Unknown, Conflicting int }

func qixinListingRows(subject map[string]any, field string) []map[string]any {
	data := qixinSubjectData(subject, field)
	if field == qixinDetailField {
		data, _, _ = findFirst(data, "AbnormalItems")
	}
	return qixinItemMaps(data, "InReason", "InDate", "OutDate", "InDepartment")
}

func qixinListingCount(rows []map[string]any) qixinListingCounts {
	result := qixinListingCounts{Total: len(rows)}
	for _, row := range rows {
		switch qixinListingState(row) {
		case "当前在列":
			result.Current++
		case "已移出或处理":
			result.Removed++
		case "状态冲突":
			result.Conflicting++
		default:
			result.Unknown++
		}
	}
	return result
}

// qixinListingSummary labels historical counts and current evidence separately.
func qixinListingSummary(subject map[string]any, field, label string) string {
	result := qixinQueryRecords(subject, field)
	if !result.Known {
		return label + result.Reason
	}
	c := qixinListingCount(result.Rows)
	if c.Total == 0 {
		return label + "未查询到记录"
	}
	text := fmt.Sprintf("%s历史%d项，其中已移出或处理%d项、明确当前在列%d项、当前状态未取得%d项", label, c.Total, c.Removed, c.Current, c.Unknown)
	if c.Conflicting > 0 {
		text += fmt.Sprintf("、状态冲突%d项", c.Conflicting)
	}
	return text
}

func addQixinListingCounts(d *SectionAnalysisData, subject map[string]any) {
	name := mapText(subject, qixinSubjectNameField)
	for _, item := range []struct{ field, label string }{{qixinDetailField, "经营异常"}, {qixinSeriousIllegalField, "严重违法"}} {
		result := qixinQueryRecords(subject, item.field)
		counts := qixinListingCount(result.Rows)
		prefix := "credit-count." + analysisToken(name+item.field)
		source := "source-snapshot.json/moduleResults[code=qixin_external]/data/subjects[name=" + name + "]/" + item.field
		d.text(prefix+".summary", name+item.label+"数量口径", "state", qixinListingSummary(subject, item.field, item.label), source)
		if !result.Known {
			continue
		}
		for _, n := range []struct {
			key, label string
			value      int
		}{{"history", "历史总数", counts.Total}, {"current", "明确当前在列", counts.Current}, {"removed", "已移出或处理", counts.Removed}, {"unknown", "当前状态未取得", counts.Unknown}, {"conflicting", "状态冲突", counts.Conflicting}} {
			d.number(prefix+"."+n.key, name+item.label+n.label, "count", "条", "公开信息查询时点", "", "按每条记录的明确状态分类计数", float64(n.value), source)
		}
	}
}
