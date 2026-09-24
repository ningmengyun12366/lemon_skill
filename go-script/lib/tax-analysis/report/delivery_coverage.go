package report

import (
	"math"
	"sort"
	"strings"
	"time"
)

// InvoiceCoverage limits interpretations, without changing an API task's status.
type InvoiceCoverage struct {
	Complete       bool     `json:"complete"`
	Reconciled     bool     `json:"reconciled"`
	ExpectedMonths []string `json:"expectedMonths"`
	AcquiredMonths []string `json:"acquiredMonths"`
	MissingMonths  []string `json:"missingMonths"`
	Notice         string   `json:"notice,omitempty"`
}

// buildInvoiceCoverage uses the same signed invoice facts as every report view.
// Missing declaration periods cannot prove that a sample covers the enterprise.
func buildInvoiceCoverage(ctx *reportContext) map[string]InvoiceCoverage {
	months := deliveryMonths(ctx.task.StartTime, ctx.task.EndTime)
	expected, declared := map[string]bool{}, map[string]bool{}
	for _, fact := range ctx.vat {
		if !fact.HasSales {
			continue
		}
		covered := deliveryMonths(fact.TaxPeriod.Start, fact.TaxPeriod.End)
		if len(covered) == 0 && fact.BucketMonth != "" {
			covered = []string{fact.BucketMonth}
		}
		for _, month := range covered {
			declared[month] = true
			if fact.Sales > 0 {
				expected[month] = true
			}
		}
	}
	output := []string{}
	for _, month := range months {
		if expected[month] || !declared[month] {
			output = append(output, month)
		}
	}
	return map[string]InvoiceCoverage{
		"issued_invoices":   assessInvoiceCoverage(ctx.issued, output, ctx.modules["issued_invoices"].Complete, "销项发票"),
		"received_invoices": assessInvoiceCoverage(ctx.received, months, ctx.modules["received_invoices"].Complete, "进项发票"),
	}
}

func assessInvoiceCoverage(facts []invoiceGroup, expected []string, sourceComplete bool, label string) InvoiceCoverage {
	result := InvoiceCoverage{Reconciled: true, ExpectedMonths: expected}
	months := map[string]bool{}
	total, monthly := 0.0, 0.0
	for _, fact := range facts {
		if !fact.HasAmount {
			sourceComplete = false
			continue
		}
		total += fact.Amount
		if fact.Month != "" {
			months[fact.Month] = true
			monthly += fact.Amount
		}
	}
	for month := range months {
		result.AcquiredMonths = append(result.AcquiredMonths, month)
	}
	sort.Strings(result.AcquiredMonths)
	for _, month := range expected {
		if !months[month] {
			result.MissingMonths = append(result.MissingMonths, month)
		}
	}
	result.Reconciled = math.Abs(total-monthly) < 0.005
	topTotal := 0.0
	for i, item := range counterpartyAmounts(facts) {
		if i >= 10 {
			break
		}
		topTotal += item.amount
	}
	if topTotal-total > 0.005 {
		result.Reconciled = false
	}
	result.Complete = sourceComplete && len(result.MissingMonths) == 0 && result.Reconciled
	if !result.Reconciled {
		result.Notice = "本期" + label + "的月度汇总或主要交易方合计与总额未能勾稽一致，暂不据此作相关判断。"
	} else if !result.Complete {
		result.Notice = "本期取得的" + label + "数据覆盖不完整，以下内容仅反映已取得样本，不能代表企业整体情况。"
	}
	return result
}

// deliveryMonths emits month identities; it never substitutes missing values.
func deliveryMonths(start, end string) []string {
	if len(start) < 7 || len(end) < 7 {
		return nil
	}
	a, e1 := time.Parse("2006-01", start[:7])
	b, e2 := time.Parse("2006-01", end[:7])
	if e1 != nil || e2 != nil || a.After(b) {
		return nil
	}
	var result []string
	for !a.After(b) {
		result = append(result, a.Format("2006-01"))
		a = a.AddDate(0, 1, 0)
	}
	return result
}

// sectionInvoiceSources identifies sample-dependent interpretations. Direct
// authority-confirmed risks remain governed by their own evidence.
func sectionInvoiceSources(key string) []string {
	switch key {
	case "invoice.customers", "invoice.customer-concentration", "invoice.business-match", "enterprise.industry", "vat.invoice-declaration-difference":
		return []string{"issued_invoices"}
	case "invoice.suppliers", "invoice.supplier-concentration":
		return []string{"received_invoices"}
	case "invoice.category-match":
		return []string{"issued_invoices", "received_invoices"}
	case "invoice.association-clues":
		return []string{"issued_invoices", "received_invoices"}
	}
	if strings.HasPrefix(key, "industry.") {
		return []string{"issued_invoices"}
	}
	return nil
}
