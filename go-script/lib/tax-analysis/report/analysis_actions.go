package report

import (
	"fmt"
	"strings"
)

// AnalysisAction is a script-owned, evidence-bound action. An absent deadline,
// owner or completion result stays absent; attention never produces an action.
type AnalysisAction struct {
	FindingID       string   `json:"findingId"`
	Object          string   `json:"object"`
	Periods         []string `json:"periods"`
	Count           int      `json:"count"`
	Amount          float64  `json:"amount"`
	Evidence        string   `json:"evidence"`
	Materials       []string `json:"materials"`
	Steps           string   `json:"steps"`
	CompletionCheck string   `json:"completionCheck"`
	Text            string   `json:"text"`
}

func analysisActions(model Model) []AnalysisAction {
	var actions []AnalysisAction
	for _, f := range model.Findings {
		a := AnalysisAction{FindingID: f.ID, Object: f.Title, Periods: f.Periods, Amount: f.Amount, Evidence: f.Evidence, Steps: f.Recommendation}
		for _, o := range f.Occurrences {
			a.Count += o.Count
		}
		switch f.RiskThemeID {
		case "payment-late-fee":
			a.Materials = []string{"对应税种及所属期的缴款记录", "应缴/实缴日期", "本金与滞纳金处理结果"}
			a.Steps = fmt.Sprintf("逐笔核对%s的%d笔滞纳金缴款记录，合计%s元，查明对应税种、应缴与实缴日期及本金处理结果", f.Period, a.Count, moneyText(f.Amount))
			a.CompletionCheck = "以对应申报、缴款及当前欠税查询结果确认是否仍有待办；已缴滞纳金不作为当前欠税"
		case "filing-confirmed-exception":
			a.Materials = []string{"异常事项的应申报清册", "该税种所属期的申报回执及当前状态"}
			a.Steps = strings.TrimRight(f.Evidence, "。；") + "；按该事项逐项比对应申报清册、申报回执及当前状态，仅对仍未处理的事项办理补报或更正"
			a.CompletionCheck = "逐项保存处理回执并复查申报状态，确认异常已处理或记录仍待办理的原因"
		default:
			a.Materials = []string{"风险证据中列明对象和期间的原始记录", "对应业务凭证及已办理结果"}
			a.CompletionCheck = "将核对或办理结果逐项对应原风险证据，记录已解决事项与剩余差异"
		}
		a.Steps = strings.TrimSuffix(a.Steps, "；"+a.CompletionCheck+"。")
		a.Text = strings.TrimRight(a.Steps, "。；") + "；" + a.CompletionCheck + "。"
		actions = append(actions, a)
	}
	return actions
}

// concretizeAnalysisActions updates every presentation of an existing finding
// together, including remediation tables and recommendation placeholders.
func concretizeAnalysisActions(model Model) Model {
	actions := map[string]AnalysisAction{}
	replacements := map[string]string{}
	for _, a := range analysisActions(model) {
		actions[a.FindingID] = a
	}
	for i := range model.Findings {
		f := &model.Findings[i]
		if a, ok := actions[f.ID]; ok {
			if f.Recommendation != "" {
				replacements[f.Recommendation] = a.Text
			}
			f.Recommendation = a.Text
		}
	}
	for i := range model.Evaluations {
		if f := model.Evaluations[i].Finding; f != nil {
			if a, ok := actions[f.ID]; ok {
				copyFinding := *f
				copyFinding.Recommendation = a.Text
				model.Evaluations[i].Finding = &copyFinding
			}
		}
	}
	replace := func(value string) string {
		for before, after := range replacements {
			value = strings.ReplaceAll(value, before, after)
		}
		return value
	}
	for si := range model.Sections {
		for bi := range model.Sections[si].Blocks {
			b := &model.Sections[si].Blocks[bi]
			for i := range b.Paragraphs {
				b.Paragraphs[i] = replace(b.Paragraphs[i])
			}
			for ri := range b.Rows {
				for ci := range b.Rows[ri] {
					b.Rows[ri][ci] = replace(b.Rows[ri][ci])
				}
			}
			if b.Finding != nil {
				copyFinding := *b.Finding
				if a, ok := actions[copyFinding.ID]; ok {
					copyFinding.Recommendation = a.Text
				}
				b.Finding = &copyFinding
			}
		}
	}
	return model
}
