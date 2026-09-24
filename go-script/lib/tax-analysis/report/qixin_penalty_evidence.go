package report

import (
	"strconv"
	"strings"
)

// qixinCreditText only normalizes missing-value markers for public credit
// evidence. It deliberately leaves the shared JSON lookup contract unchanged.
func qixinCreditText(row map[string]any, aliases ...string) string {
	for _, alias := range aliases {
		value := strings.TrimSpace(mapText(row, alias))
		switch strings.ToLower(value) {
		case "", "-", "—", "--", "——", "/", "null", "undefined", "n/a":
			continue
		}
		return value
	}
	return ""
}

func counterpartyOtherCreditRows(subject map[string]any, field string) []map[string]any {
	root := qixinSubjectData(subject, field)
	if field != qixinAdministrativePunishmentField {
		return qixinItemMaps(root, "InReason", "InDate", "Number", "Content", "Reason")
	}
	return qixinPenaltyEvidenceRows(root)
}

// qixinPenaltyEvidenceRows reads direct record fields rather than flattening
// wrapper objects. A wrapper containing records is not itself a second record.
func qixinPenaltyEvidenceRows(root any) []map[string]any {
	var walk func(any, int) []map[string]any
	walk = func(value any, depth int) []map[string]any {
		if depth > 10 {
			return nil
		}
		var children []map[string]any
		switch item := value.(type) {
		case []any:
			for _, child := range item {
				children = append(children, walk(child, depth+1)...)
			}
		case map[string]any:
			for _, key := range sortedMapKeys(item) {
				children = append(children, walk(item[key], depth+1)...)
			}
			if len(children) == 0 && qixinCreditText(item, "IllegalType", "Reason", "InReason", "Content", "Description", "Number", "InDate", "Date", "PublishDate") != "" {
				return []map[string]any{item}
			}
		}
		return children
	}
	return walk(root, 0)
}

// Keep the event, its stated reason and the decision in separate facts. An
// opaque decision code is traceable but supplies no amount or status meaning.
func addQixinPenaltyDetails(d *SectionAnalysisData, row map[string]any, prefix, source string) []string {
	var refs []string
	for _, field := range []struct{ source, key, label string }{
		{"IllegalType", "illegal-type", "行政处罚记录违法事项"},
		{"Reason", "reason", "行政处罚记录公开原因"},
		{"InReason", "listing-reason", "行政处罚记录列入原因"},
		{"Description", "description", "行政处罚记录公开事项描述"},
		{"Content", "content", "行政处罚记录决定内容"},
	} {
		value := qixinCreditText(row, field.source)
		if value == "" {
			continue
		}
		key, label, kind := field.key, field.label, "state"
		if _, err := strconv.ParseFloat(value, 64); err == nil {
			key, label, kind = key+"-raw", label+"原值（代码含义未取得）", "raw-code"
		}
		refs = append(refs, d.text(prefix+"."+key, label, kind, value, source+"/"+field.source))
	}
	return refs
}
