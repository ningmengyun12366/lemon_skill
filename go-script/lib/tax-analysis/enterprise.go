package taxanalysis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	invoicecommon "invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

// BoundEnterpriseChoice 是允许展示给用户选择的绑定企业最小信息。
type BoundEnterpriseChoice struct {
	EnterpriseName string `json:"enterpriseName"`
	USCC           string `json:"uscc"`
}

// ListBoundEnterprises 查询当前 API Key 下可供用户选择的绑定企业。
//
// @param ctx 调用上下文；进入前已取消时直接返回上下文错误。
// @returns 按接口顺序排列且按 USCC 去重的企业名称和 USCC；不返回登录账号等内部上下文。
func (c *Client) ListBoundEnterprises(ctx context.Context) ([]BoundEnterpriseChoice, error) {
	snapshots, err := c.listBoundEnterpriseSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	choices := make([]BoundEnterpriseChoice, 0, len(snapshots))
	seenUSCC := make(map[string]struct{}, len(snapshots))
	for _, enterprise := range snapshots {
		name := strings.TrimSpace(invoicecommon.StringValue(enterprise["enterpriseName"]))
		uscc := strings.TrimSpace(invoicecommon.StringValue(enterprise["uscc"]))
		if name == "" && uscc == "" {
			continue
		}
		deduplicationKey := strings.ToUpper(uscc)
		if deduplicationKey != "" {
			if _, exists := seenUSCC[deduplicationKey]; exists {
				continue
			}
			seenUSCC[deduplicationKey] = struct{}{}
		}
		choices = append(choices, BoundEnterpriseChoice{EnterpriseName: name, USCC: uscc})
	}
	if len(choices) == 0 {
		return nil, errors.New("当前 API Key 未绑定可用企业")
	}
	return choices, nil
}

// ResolveBoundEnterprise 在当前 API Key 的绑定企业中按企业名称或统一社会信用代码精确选择企业。
//
// @param ctx 调用上下文；进入前已取消时直接返回上下文错误。
// @param companyOrUSCC 用户提供的完整企业名称或统一社会信用代码。
// @returns 服务端企业列表中的白名单快照；未绑定或企业名称不唯一时返回可读错误。
func (c *Client) ResolveBoundEnterprise(ctx context.Context, companyOrUSCC string) (map[string]any, error) {
	if c == nil {
		return nil, errors.New("tax analysis client is required")
	}
	if ctx == nil {
		return nil, errors.New("bound enterprise context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	selector := strings.TrimSpace(companyOrUSCC)
	if selector == "" {
		return nil, errors.New("company name or uscc is required")
	}

	snapshots, err := c.listBoundEnterpriseSnapshots(ctx)
	if err != nil {
		return nil, err
	}
	matches := make([]map[string]any, 0, 1)
	for _, record := range snapshots {
		if !boundEnterpriseMatches(record, selector) {
			continue
		}
		matches = append(matches, record)
	}
	if len(matches) == 0 {
		return nil, fmt.Errorf("企业 %s 未绑定到当前 API Key", selector)
	}
	if len(matches) > 1 {
		return nil, fmt.Errorf("企业名称 %s 对应多个绑定企业，请改用统一社会信用代码", selector)
	}
	if err := invoicecommon.ValidateEnterpriseContext(matches[0]); err != nil {
		return nil, fmt.Errorf("绑定企业信息不完整: %w", err)
	}
	return matches[0], nil
}

// listBoundEnterpriseSnapshots 查询并清理当前 API Key 的绑定企业列表。
//
// @param ctx 调用上下文。
// @returns 仅含税务分析登录与鉴权白名单字段的企业快照列表。
func (c *Client) listBoundEnterpriseSnapshots(ctx context.Context) ([]map[string]any, error) {
	if c == nil {
		return nil, errors.New("tax analysis client is required")
	}
	if ctx == nil {
		return nil, errors.New("bound enterprise context is required")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	baseClient := &invoicecommon.APIClient{
		BaseURL:        c.baseURL,
		Client:         c.httpClient,
		VersionChecker: c.versionChecker,
		RequestSender:  c.requestSender(ctx),
		HeaderProvider: func() map[string]string {
			return map[string]string{"Content-Type": "application/json"}
		},
	}
	response, err := baseClient.PostJSON(
		BoundEnterprisePath,
		map[string]any{"apiKey": c.apiKey},
		BoundEnterpriseTimeout,
	)
	if err != nil {
		return nil, fmt.Errorf("query bound enterprises: %w", err)
	}
	items, ok := response["data"].([]any)
	if !boundEnterpriseResponseSucceeded(response["code"]) || !ok {
		message := strings.TrimSpace(invoicecommon.StringValue(response["message"]))
		if message == "" {
			message = "enterprise-list did not return a successful list response"
		}
		return nil, fmt.Errorf("query bound enterprises: %s", message)
	}

	result := make([]map[string]any, 0, len(items))
	for _, item := range items {
		record, recordOK := item.(map[string]any)
		if !recordOK {
			continue
		}
		result = append(result, whitelistBoundEnterprise(record))
	}
	return result, nil
}

// boundEnterpriseMatches 判断服务端企业是否与用户输入精确匹配。
//
// @param enterprise 服务端返回的单个绑定企业。
// @param selector 已去除首尾空白的企业名称或统一社会信用代码。
// @returns 名称完全相同或 USCC 忽略大小写完全相同时返回 true。
func boundEnterpriseMatches(enterprise map[string]any, selector string) bool {
	name := strings.TrimSpace(invoicecommon.StringValue(enterprise["enterpriseName"]))
	uscc := strings.TrimSpace(invoicecommon.StringValue(enterprise["uscc"]))
	return name == selector || strings.EqualFold(uscc, selector)
}

// whitelistBoundEnterprise 复制税务分析登录和鉴权需要的企业字段。
//
// @param source 企业列表返回的原始企业对象。
// @returns 不包含密码、Cookie 或未知字段的企业快照。
func whitelistBoundEnterprise(source map[string]any) map[string]any {
	result := make(map[string]any, len(boundEnterpriseContextFields))
	for _, field := range boundEnterpriseContextFields {
		if value, ok := source[field]; ok && invoicecommon.HasMeaningfulValue(value) {
			result[field] = value
		}
	}
	return result
}

// boundEnterpriseResponseSucceeded 判断企业列表接口是否返回成功业务码。
//
// @param value 接口 JSON 中的 code 字段。
// @returns code 等于企业列表成功码时返回 true。
func boundEnterpriseResponseSucceeded(value any) bool {
	switch code := value.(type) {
	case float64:
		return code == float64(BoundEnterpriseCode)
	case int:
		return code == BoundEnterpriseCode
	case int64:
		return code == int64(BoundEnterpriseCode)
	case string:
		return strings.TrimSpace(code) == fmt.Sprintf("%d", BoundEnterpriseCode)
	default:
		return false
	}
}
