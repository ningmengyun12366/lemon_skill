package issue

import (
	"fmt"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

type issueTaskContext struct {
	Enterprise map[string]any
	Request    common.EnterpriseRequestContext
}

/**
 * resolveIssueTaskContext 校验开票任务中的输入、正式 payload 和企业快照使用同一 USCC。
 *
 * @param taskState 已加载的开票任务状态。
 * @param payload 即将提交或查询的开票业务参数。
 * @returns 只由任务企业快照构造的业务企业和请求头上下文。
 */
func resolveIssueTaskContext(taskState, payload map[string]any) (issueTaskContext, error) {
	enterprise, err := common.EnterpriseContextFromTask(taskState)
	if err != nil {
		return issueTaskContext{}, err
	}
	raw, _ := taskState["raw_input_json"].(map[string]any)
	rawUSCC := strings.TrimSpace(common.StringValue(raw["uscc"]))
	payloadUSCC := extractIssueUSCC(payload)
	contextUSCC := strings.TrimSpace(common.StringValue(enterprise["uscc"]))
	if rawUSCC != "" && payloadUSCC != "" && rawUSCC != payloadUSCC {
		return issueTaskContext{}, fmt.Errorf("task enterprise uscc does not match invoice payload uscc")
	}
	for source, value := range map[string]string{"raw_input_json": rawUSCC, "invoice_payload_json": payloadUSCC} {
		if value == "" || value != contextUSCC {
			return issueTaskContext{}, fmt.Errorf("%s uscc=%s does not match enterprise_context uscc=%s", source, value, contextUSCC)
		}
	}
	requestContext, err := common.EnterpriseRequestContextFromSnapshot(enterprise, common.InvoiceIssueCapabilityCode)
	if err != nil {
		return issueTaskContext{}, err
	}
	return issueTaskContext{
		Enterprise: enterprise,
		Request:    requestContext,
	}, nil
}
