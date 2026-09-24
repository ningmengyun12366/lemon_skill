package taxanalysis

import (
	"context"
	"errors"
	"fmt"
	"strings"

	invoicecommon "invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

// LoginReadinessResult 保存税务分析任务发起前的企业快照和登录检查结果。
type LoginReadinessResult struct {
	Enterprise  map[string]any `json:"enterprise"`
	LoginStatus map[string]any `json:"loginStatus"`
}

// PrepareLoginReadiness 按当前 API Key 和企业名称或 USCC 确认绑定，并复用开票、取票的登录状态及快速登录流程。
//
// @param ctx 调用上下文；进入前已取消时直接返回上下文错误。
// @param companyOrUSCC 用户提供的完整企业名称或统一社会信用代码。
// @returns 企业快照和登录状态；登录未就绪时返回包含手动登录网页入口的错误。
func (c *Client) PrepareLoginReadiness(ctx context.Context, companyOrUSCC string) (LoginReadinessResult, error) {
	if c == nil {
		return LoginReadinessResult{}, errors.New("tax analysis client is required")
	}
	if ctx == nil {
		return LoginReadinessResult{}, errors.New("login readiness context is required")
	}
	if err := ctx.Err(); err != nil {
		return LoginReadinessResult{}, err
	}
	selector := strings.TrimSpace(companyOrUSCC)
	if selector == "" {
		return LoginReadinessResult{}, errors.New("company name or uscc is required")
	}

	enterprise, err := c.ResolveBoundEnterprise(ctx, selector)
	if err != nil {
		return LoginReadinessResult{}, fmt.Errorf("resolve tax analysis enterprise: %w", err)
	}
	baseClient := &invoicecommon.APIClient{
		BaseURL:        c.baseURL,
		Client:         c.httpClient,
		VersionChecker: c.versionChecker,
		RequestSender:  c.requestSender(ctx),
	}
	requestContext, err := invoicecommon.EnterpriseRequestContextFromSnapshot(enterprise, DefaultCapabilityCode)
	if err != nil {
		return LoginReadinessResult{Enterprise: enterprise}, fmt.Errorf("prepare tax analysis login context: %w", err)
	}
	config := map[string]any{invoicecommon.APIKey: c.apiKey}
	loginClient := invoicecommon.NewEnterpriseAPIClient(baseClient, config, requestContext)
	readiness, err := invoicecommon.PrepareLoginReadiness(invoicecommon.LoginReadinessRequest{
		Enterprise: enterprise,
		Config:     config,
		Client:     loginClient,
	})
	result := LoginReadinessResult{
		Enterprise:  enterprise,
		LoginStatus: readiness.LoginStatus,
	}
	if err != nil {
		return result, fmt.Errorf("tax analysis login readiness: %w", err)
	}
	return result, nil
}
