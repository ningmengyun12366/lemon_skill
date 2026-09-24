package report

import "strings"

// Only independent profit-total lines participate; subtotals, "其中" rows and
// income tax expense are excluded to avoid counting the same amount twice.
var financialProfitComponents = []struct {
	key, label string
	sign       float64
}{
	{"surtax", "税金及附加", -1},
	{"other-income", "其他收益", 1},
	{"investment", "投资收益", 1},
	{"hedging", "净敞口套期收益", 1},
	{"fair-value", "公允价值变动收益", 1},
	{"credit-loss", "信用减值损失", 1},
	{"asset-loss", "资产减值损失", 1},
	{"asset-disposal", "资产处置收益", 1},
	{"non-operating-income", "营业外收入", 1},
	{"non-operating-expense", "营业外支出", -1},
}

func financialProfitComponentLabels() []string {
	labels := make([]string, 0, len(financialProfitComponents))
	for _, c := range financialProfitComponents {
		labels = append(labels, c.label)
	}
	return labels
}

func financialProfitComponent(label string) bool {
	for _, c := range financialProfitComponents {
		if c.label == label {
			return true
		}
	}
	return false
}

// Impairment changed presentation between form versions. An unsigned label
// alone cannot determine whether a loss is deducted or already signed.
func recordFinancialProfitSign(f *financialFact, sourceLabel string) {
	label := normalizeFinancialMetricLabel(sourceLabel)
	if !financialProfitComponent(label) {
		return
	}
	if f.ProfitComponentSigns == nil {
		f.ProfitComponentSigns = map[string]float64{}
	}
	if _, exists := f.ProfitComponentSigns[label]; exists {
		return
	}
	sign := 0.0
	for _, c := range financialProfitComponents {
		if c.label == label {
			sign = c.sign
		}
	}
	if label == "资产减值损失" || label == "信用减值损失" {
		raw := canonicalFormLabel(sourceLabel)
		switch {
		case strings.HasPrefix(raw, "减："), strings.HasPrefix(raw, "减:"):
			sign = -1
		case strings.HasPrefix(raw, "加："), strings.HasPrefix(raw, "加:"), strings.Contains(raw, "损失以") && (strings.Contains(raw, "-") || strings.Contains(raw, "－")):
			sign = 1
		default:
			sign = 0
		}
	}
	f.ProfitComponentSigns[label] = sign
}
