package common

import (
	"fmt"
	"strings"
	"time"
)

const enterpriseListRequestURL = "/share/enterprise-list"

var enterpriseContextFields = []string{
	"enterpriseName",
	"uscc",
	"areaCode",
	"agencyUSCC",
	"loginType",
	"identityType",
	"personalAccount",
	"nsrzg",
}

/**
 * ResolveBoundEnterprise 按显式 USCC 查询 API Key 绑定的企业，并生成任务企业快照。
 *
 * @param client 用于调用 Gateway 的基础 APIClient。
 * @param apiKey 当前工作区配置的 API Key。
 * @param uscc 新任务显式提供的企业统一社会信用代码。
 * @returns 只包含白名单字段且已完成必填校验的企业快照。
 */
func ResolveBoundEnterprise(client *APIClient, apiKey, uscc string) (map[string]any, error) {
	targetUSCC := strings.TrimSpace(uscc)
	if targetUSCC == "" {
		return nil, fmt.Errorf("task uscc is required")
	}
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("workspace API Key is required to resolve task enterprise")
	}
	if client == nil {
		client = NewAPIClient()
	}
	enterpriseListClient := *client
	enterpriseListClient.HeaderProvider = func() map[string]string {
		return map[string]string{"Content-Type": "application/json"}
	}
	response, err := enterpriseListClient.PostJSON(
		enterpriseListRequestURL,
		map[string]any{"apiKey": strings.TrimSpace(apiKey)},
		15*time.Second,
	)
	if err != nil {
		return nil, fmt.Errorf("query bound enterprises: %w", err)
	}
	items, ok := response["data"].([]any)
	if !codeEquals(response["code"], 200) || !ok {
		message := strings.TrimSpace(StringValue(response["message"]))
		if message == "" {
			message = "enterprise-list did not return a successful list response"
		}
		return nil, fmt.Errorf("query bound enterprises: %s", message)
	}
	for _, item := range items {
		record, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if strings.TrimSpace(StringValue(record["uscc"])) != targetUSCC {
			continue
		}
		context := whitelistEnterpriseContext(record)
		context["uscc"] = targetUSCC
		if err := ValidateEnterpriseContext(context); err != nil {
			return nil, err
		}
		return context, nil
	}
	return nil, fmt.Errorf("task uscc %s is not bound to the configured API Key", targetUSCC)
}

/**
 * ValidateEnterpriseContext 校验任务企业快照是否满足后续鉴权和业务调用要求。
 *
 * @param enterpriseContext 待校验的企业快照。
 * @returns 快照完整时返回 nil，否则返回缺失或非法字段错误。
 */
func ValidateEnterpriseContext(enterpriseContext map[string]any) error {
	if enterpriseContext == nil {
		return fmt.Errorf("任务缺少 enterprise_context，属于旧版本任务，请重新创建任务")
	}
	for _, key := range []string{"uscc", "areaCode", "loginType", "personalAccount"} {
		if !HasMeaningfulValue(enterpriseContext[key]) {
			return fmt.Errorf("task enterprise_context is missing %s", key)
		}
	}
	// 这里只验证服务端返回值可被业务层识别，不改写不可变快照中的原始 areaCode。
	if _, err := NormalizeStandardAreaCode(enterpriseContext["areaCode"]); err != nil {
		return fmt.Errorf("task enterprise_context areaCode is invalid: %w", err)
	}
	loginType := strings.TrimSpace(StringValue(enterpriseContext["loginType"]))
	if loginType != "1" && loginType != "2" {
		return fmt.Errorf("task enterprise_context loginType must be 1 or 2")
	}
	if loginType == "1" && strings.TrimSpace(StringValue(enterpriseContext["agencyUSCC"])) == "" {
		return fmt.Errorf("task enterprise_context is missing agencyUSCC for proxy login")
	}
	return nil
}

/**
 * EnterpriseContextFromTask 读取并复制任务文件中的不可变企业快照。
 *
 * @param task 取票或开票任务状态对象。
 * @returns 校验完成的企业快照副本。
 */
func EnterpriseContextFromTask(task map[string]any) (map[string]any, error) {
	raw, ok := task[EnterpriseContextKey].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("任务缺少 enterprise_context，属于旧版本任务，请重新创建任务")
	}
	context := whitelistEnterpriseContext(raw)
	if err := ValidateEnterpriseContext(context); err != nil {
		return nil, err
	}
	return context, nil
}

/**
 * ApplyEnterpriseContext 将服务端企业快照写入任务业务参数，并覆盖不可信的同名输入字段。
 *
 * @param params 新任务的业务参数。
 * @param enterpriseContext 已通过校验的服务端企业快照。
 * @returns 写入企业字段后的参数副本。
 */
func ApplyEnterpriseContext(params, enterpriseContext map[string]any) map[string]any {
	result := map[string]any{}
	for key, value := range params {
		if key == EnterpriseContextKey {
			continue
		}
		result[key] = value
	}
	for _, key := range enterpriseContextFields {
		if value, ok := enterpriseContext[key]; ok && HasMeaningfulValue(value) {
			result[key] = value
		}
	}
	return result
}

/**
 * EnterpriseRequestContextFromSnapshot 构造企业专用请求头所需的显式上下文。
 *
 * @param enterpriseContext 任务企业快照。
 * @param capabilityCode 当前 Skill 能力编码。
 * @returns 用于 NewEnterpriseAPIClient 的请求上下文。
 */
func EnterpriseRequestContextFromSnapshot(enterpriseContext map[string]any, capabilityCode string) (EnterpriseRequestContext, error) {
	context := whitelistEnterpriseContext(enterpriseContext)
	if err := ValidateEnterpriseContext(context); err != nil {
		return EnterpriseRequestContext{}, err
	}
	return EnterpriseRequestContext{
		USCC:            strings.TrimSpace(StringValue(context["uscc"])),
		PersonalAccount: strings.TrimSpace(StringValue(context["personalAccount"])),
		LoginType:       strings.TrimSpace(StringValue(context["loginType"])),
		CapabilityCode:  strings.TrimSpace(capabilityCode),
	}, nil
}

/**
 * whitelistEnterpriseContext 复制允许进入任务文件的企业字段，丢弃密码、Cookie 和未知字段。
 *
 * @param source 企业列表响应或任务中的原始企业对象。
 * @returns 只包含 enterpriseContextFields 白名单字段的新对象。
 */
func whitelistEnterpriseContext(source map[string]any) map[string]any {
	result := map[string]any{}
	for _, key := range enterpriseContextFields {
		if value, ok := source[key]; ok && HasMeaningfulValue(value) {
			result[key] = value
		}
	}
	return result
}
