package report

import "encoding/json"

type deductionSummary struct {
	Format        string
	Chunks        []deductionSummaryChunk
	Applicability struct{ Status, ReasonCode, Message string }
	Association   struct{ Matched, Missing, Restricted, SelectedAbnormal int }
}
type deductionSummaryChunk struct {
	Month, StartTime, EndTime                                                                  string
	List                                                                                       *deductionDatasetSummary
	AbnormalInvoiceList                                                                        *deductionDatasetSummary
	CustomsPaymentList, WithholdingList, AgriProductAdditionalList, AgriProductCustomsDocsList *deductionDatasetSummary
}
type deductionDatasetSummary struct {
	Count, NormalAbnormal, Flagged, SelectedAbnormal, Conflict int
	Purchases                                                  []json.RawMessage
	SelectedTax                                                []struct {
		Period   string
		Amount   float64
		Complete bool
	}
	Confirmations []deductionConfirmationFact
	Signals       []struct {
		Counterparty, Period string
		Amount               float64
	}
}

func readDeductionSummary(raw any) deductionSummary {
	var result deductionSummary
	data, err := json.Marshal(raw)
	if err == nil {
		_ = json.Unmarshal(data, &result)
	}
	return result
}

func deductionSignals(module ModuleSnapshot) []ruleSignal {
	var result []ruleSignal
	for _, chunk := range readDeductionSummary(module.Raw).Chunks {
		for _, dataset := range []*deductionDatasetSummary{chunk.List, chunk.AbnormalInvoiceList, chunk.CustomsPaymentList, chunk.WithholdingList, chunk.AgriProductAdditionalList, chunk.AgriProductCustomsDocsList} {
			if dataset == nil {
				continue
			}
			for _, signal := range dataset.Signals {
				result = append(result, ruleSignal{Type: "abnormal_voucher_deducted", Counterparty: signal.Counterparty, Period: signal.Period, Amount: signal.Amount, Direct: true, Evidence: "用途确认数据中存在已勾选或已抵扣的异常凭证。"})
			}
		}
	}
	return result
}

func deductionConfirmationFacts(raw any) []deductionConfirmationFact {
	var result []deductionConfirmationFact
	for _, chunk := range readDeductionSummary(raw).Chunks {
		if chunk.List != nil {
			result = append(result, chunk.List.Confirmations...)
		}
	}
	return result
}
