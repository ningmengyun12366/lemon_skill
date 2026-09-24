package fetchrisk

import (
	"crypto/sha256"
	"fmt"
	"io"
	"os"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

const (
	summarizeStep               = "summarize"
	generateInvoiceListStep     = "generate_invoice_list"
	analyzeInvoiceDataStep      = "analyze"
	buildInvoiceContextDataStep = "build_invoice_context_data"
	riskAnalysisStep            = "risk_analysis"
)

var analysisAndRiskOutputKeys = []string{
	"analysis_excel_path",
	"invoice_context_data_path",
	"risk_context_summary",
	"risk_report_body",
	"risk_report_pdf_path",
	"risk_report_markdown_path",
	"risk_report_md_path",
	"report_markdown_path",
	"report_md_path",
	"report_pdf_path",
}

var riskReportOutputKeys = []string{
	"risk_report_body",
	"risk_report_pdf_path",
	"risk_report_markdown_path",
	"risk_report_md_path",
	"report_markdown_path",
	"report_md_path",
	"report_pdf_path",
}

var staleArtifactKeys = []string{
	"invoice_context_data_path",
	"invoice_context_data",
	"invoice_context_warnings",
	"risk_report_markdown_path",
	"risk_report_md_path",
	"report_markdown_path",
	"report_md_path",
	"report_pdf_path",
}

func prepareTaskSteps(steps []map[string]any) []any {
	out := make([]any, 0, len(steps))
	for index, source := range steps {
		step := map[string]any{}
		for key, value := range source {
			step[key] = value
		}
		if strings.TrimSpace(common.StringValue(step["name"])) == "" {
			continue
		}
		if _, ok := step["index"]; !ok {
			step["index"] = index + 1
		}
		enabled := true
		if value, ok := step["enabled"].(bool); ok {
			enabled = value
		}
		step["enabled"] = enabled
		if enabled {
			step["status"] = "pending"
		} else {
			step["status"] = "skipped"
			step["status_reason"] = "任务初始化时已禁用"
		}
		out = append(out, step)
	}
	return out
}

func markPlannedStepStatus(data map[string]any, name, status, reason string) {
	for _, step := range taskStepObjects(data["steps"]) {
		if strings.TrimSpace(common.StringValue(step["name"])) != name {
			continue
		}
		step["status"] = status
		step["status_updated_at"] = common.NowISO()
		if strings.TrimSpace(reason) == "" {
			delete(step, "status_reason")
		} else {
			step["status_reason"] = reason
		}
		return
	}
}

func taskStepObjects(raw any) []map[string]any {
	out := []map[string]any{}
	switch steps := raw.(type) {
	case []any:
		for _, item := range steps {
			if step, ok := item.(map[string]any); ok {
				out = append(out, step)
			}
		}
	case []map[string]any:
		out = append(out, steps...)
	}
	return out
}

func deductionDependencyFingerprint(data map[string]any) (string, error) {
	intent, _ := data["intent"].(map[string]any)
	params, _ := intent["params"].(map[string]any)
	taxpayerType := NormalizeTaxpayerType(params["taxpayer_type"])
	if taxpayerType == "" {
		return "", fmt.Errorf("风险分析前置条件未满足：任务中缺少纳税人类型；请先执行 fetch-deduction --task-file <task.json>，该命令会查询企业画像并记录纳税人类型")
	}

	execution, _ := data["execution"].(map[string]any)
	fetchDeduction, _ := execution[fetchDeductionStep].(map[string]any)
	status := strings.TrimSpace(common.StringValue(fetchDeduction["status"]))
	fingerprintSource := fmt.Sprintf("taxpayer_type=%s\nstatus=%s\n", taxpayerType, status)

	if taxpayerType == "一般纳税人" {
		if status != "success" {
			return "", fmt.Errorf("一般纳税人勾选认证数据未就绪：execution.fetch_deduction.status=%q；必须先成功执行 fetch-deduction --task-file <task.json>", status)
		}
		outputs, _ := data["outputs"].(map[string]any)
		deductionPath := strings.TrimSpace(common.StringValue(outputs["deduction_data_file"]))
		if deductionPath == "" {
			return "", fmt.Errorf("一般纳税人勾选认证数据未就绪：任务缺少 outputs.deduction_data_file；请重新执行 fetch-deduction --task-file <task.json>")
		}
		fileHash, err := sha256File(deductionPath)
		if err != nil {
			return "", fmt.Errorf("读取勾选认证数据失败，请重新执行 fetch-deduction --task-file <task.json>：%w", err)
		}
		fingerprintSource += "deduction_sha256=" + fileHash + "\n"
	} else if status != "skipped" {
		return "", fmt.Errorf("勾选认证步骤尚未完成：纳税人类型为%s，execution.fetch_deduction.status=%q；请先执行 fetch-deduction --task-file <task.json> 记录跳过结果", taxpayerType, status)
	}

	sum := sha256.Sum256([]byte(fingerprintSource))
	return fmt.Sprintf("%x", sum), nil
}

func sha256File(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", hash.Sum(nil)), nil
}

func ensureFreshInvoiceListForRisk(state *TaskState) (string, error) {
	fingerprint, err := deductionDependencyFingerprint(state.Data)
	if err != nil {
		return "", err
	}
	execution, _ := state.Data["execution"].(map[string]any)
	generateInvoiceList, _ := execution[generateInvoiceListStep].(map[string]any)
	status := strings.TrimSpace(common.StringValue(generateInvoiceList["status"]))
	if status != "success" && status != "partial_success" {
		return "", fmt.Errorf("风险分析前置条件未满足：发票清单未在勾选步骤完成后生成；请执行 generate-invoice-list --task-file <task.json>")
	}
	recordedFingerprint := strings.TrimSpace(common.StringValue(generateInvoiceList["deduction_dependency_fingerprint"]))
	if recordedFingerprint == "" || recordedFingerprint != fingerprint {
		return "", fmt.Errorf("勾选认证数据已变化或发票清单来源状态无法确认；请重新执行 generate-invoice-list --task-file <task.json> 后再构建风险上下文")
	}
	storage, _ := state.Data["storage"].(map[string]any)
	outputs, _ := state.Data["outputs"].(map[string]any)
	invoiceListDir := strings.TrimSpace(common.StringValue(firstNonEmpty(outputs["invoice_list_dir"], storage["invoice_list_dir"])))
	if invoiceListDir == "" || len(listXLSX(invoiceListDir)) == 0 {
		return "", fmt.Errorf("风险分析前置条件未满足：未找到已生成的发票清单；请重新执行 generate-invoice-list --task-file <task.json>")
	}
	return fingerprint, nil
}

func invalidateAfterDeduction(data map[string]any, reason string) {
	invalidateExecutionSteps(data, reason, generateInvoiceListStep, analyzeInvoiceDataStep, buildInvoiceContextDataStep, riskAnalysisStep)
	clearOutputKeys(common.ObjectValue(data, "outputs"), analysisAndRiskOutputKeys...)
	clearOutputKeys(common.ObjectValue(data, "artifacts"), staleArtifactKeys...)
	for _, step := range []string{generateInvoiceListStep, analyzeInvoiceDataStep, buildInvoiceContextDataStep, riskAnalysisStep} {
		markPlannedStepStatus(data, step, "pending", reason)
	}
}

func invalidateAfterInvoiceList(data map[string]any, reason string) {
	invalidateExecutionSteps(data, reason, analyzeInvoiceDataStep, buildInvoiceContextDataStep, riskAnalysisStep)
	clearOutputKeys(common.ObjectValue(data, "outputs"), analysisAndRiskOutputKeys...)
	clearOutputKeys(common.ObjectValue(data, "artifacts"), staleArtifactKeys...)
	for _, step := range []string{analyzeInvoiceDataStep, buildInvoiceContextDataStep, riskAnalysisStep} {
		markPlannedStepStatus(data, step, "pending", reason)
	}
}

func invalidateAfterRiskContext(data map[string]any, reason string) {
	invalidateExecutionSteps(data, reason, riskAnalysisStep)
	clearOutputKeys(common.ObjectValue(data, "outputs"), riskReportOutputKeys...)
	clearOutputKeys(common.ObjectValue(data, "artifacts"), riskReportOutputKeys...)
	markPlannedStepStatus(data, riskAnalysisStep, "pending", reason)
}

func invalidateExecutionSteps(data map[string]any, reason string, names ...string) {
	execution := common.ObjectValue(data, "execution")
	for _, name := range names {
		step, ok := execution[name].(map[string]any)
		if !ok {
			continue
		}
		step["status"] = "stale"
		step["stale_reason"] = reason
		step["stale_at"] = common.NowISO()
	}
}

func clearOutputKeys(outputs map[string]any, keys ...string) {
	for _, key := range keys {
		delete(outputs, key)
	}
}
