package issue

import (
	"strings"
)

const taxpayerIdentityMissingCode = "taxpayer_identity_missing"

type taxpayerIdentityKind uint8

const (
	taxpayerIdentityUnknown taxpayerIdentityKind = iota
	taxpayerIdentityGeneral
	taxpayerIdentitySmallScale
)

type taxpayerIdentitySource uint8

const (
	taxpayerIdentitySourceMissing taxpayerIdentitySource = iota
	taxpayerIdentitySourceCompanyNsrzg
	taxpayerIdentitySourceManualConfirmation
)

type taxpayerIdentity struct {
	kind   taxpayerIdentityKind
	source taxpayerIdentitySource
}

func applyLocalSellerName(payload map[string]any, enterpriseName string) {
	const sellerNamePath = "info.0.sellerInfo.name"
	if HasMeaningfulValue(GetPath(payload, sellerNamePath)) {
		return
	}
	if name := strings.TrimSpace(enterpriseName); name != "" {
		SetPath(payload, sellerNamePath, name)
	}
}

/**
 * resolveTaxpayerIdentity 从任务企业快照解析纳税人身份，必要时接受合法人工确认。
 *
 * @param enterpriseContext 当前开票任务的不可变企业快照。
 * @param manual 用户显式提供的 taxpayerType 人工确认值。
 * @returns 已解析的纳税人身份、来源和可诊断错误。
 */
func resolveTaxpayerIdentity(enterpriseContext map[string]any, manual any) taxpayerIdentity {
	if identity := parseEnterpriseNsrzg(enterpriseContext["nsrzg"]); identity.isKnown() {
		return identity
	}

	switch normalizeTaxpayerTypeInput(manual) {
	case "一般纳税人":
		return taxpayerIdentity{kind: taxpayerIdentityGeneral, source: taxpayerIdentitySourceManualConfirmation}
	case "小规模纳税人":
		return taxpayerIdentity{kind: taxpayerIdentitySmallScale, source: taxpayerIdentitySourceManualConfirmation}
	default:
		return taxpayerIdentity{kind: taxpayerIdentityUnknown, source: taxpayerIdentitySourceMissing}
	}
}

func parseEnterpriseNsrzg(value any) taxpayerIdentity {
	kind := taxpayerIdentityUnknown
	switch typed := value.(type) {
	case float64:
		switch typed {
		case 0:
			kind = taxpayerIdentityGeneral
		case 1:
			kind = taxpayerIdentitySmallScale
		}
	case string:
		switch typed {
		case "0":
			kind = taxpayerIdentityGeneral
		case "1":
			kind = taxpayerIdentitySmallScale
		}
	}
	if kind == taxpayerIdentityUnknown {
		return taxpayerIdentity{kind: kind, source: taxpayerIdentitySourceMissing}
	}
	return taxpayerIdentity{kind: kind, source: taxpayerIdentitySourceCompanyNsrzg}
}

func (identity taxpayerIdentity) isKnown() bool {
	return identity.kind == taxpayerIdentityGeneral || identity.kind == taxpayerIdentitySmallScale
}

func (identity taxpayerIdentity) taxpayerType() string {
	switch identity.kind {
	case taxpayerIdentityGeneral:
		return "一般纳税人"
	case taxpayerIdentitySmallScale:
		return "小规模纳税人"
	case taxpayerIdentityUnknown:
		return ""
	}
	return ""
}

func (identity taxpayerIdentity) sourceLabel() string {
	switch identity.source {
	case taxpayerIdentitySourceCompanyNsrzg:
		return "enterprise_context_nsrzg"
	case taxpayerIdentitySourceManualConfirmation:
		return "manual_confirmation"
	case taxpayerIdentitySourceMissing:
		return "missing"
	}
	return "missing"
}

func (identity taxpayerIdentity) warningCode() string {
	if identity.isKnown() {
		return ""
	}
	return taxpayerIdentityMissingCode
}

func (identity taxpayerIdentity) warningMessage() string {
	if identity.isKnown() {
		return ""
	}
	return taxpayerIdentityMissingCode + "：任务企业快照未提供有效 nsrzg，且未提供合法 taxpayerType 人工确认。已保留显式票种和税率，并跳过自动税率及参考税率判断"
}

func (identity taxpayerIdentity) resultMap() map[string]any {
	return map[string]any{
		"resolved":      identity.isKnown(),
		"source":        identity.sourceLabel(),
		"taxpayer_type": identity.taxpayerType(),
		"warning_code":  identity.warningCode(),
		"warning":       identity.warningMessage(),
	}
}

func applySellerProfileInvoiceDefaults(payload map[string]any, missing map[string][]map[string]any, identity taxpayerIdentity) map[string]any {
	_ = payload
	_ = missing
	taxpayerType := identity.taxpayerType()
	if !identity.isKnown() {
		return sellerProfileResult(false, false, "", false, identity.warningMessage())
	}
	return sellerProfileResult(false, true, taxpayerType, false, "已从任务企业快照解析纳税人身份；发票类型必须由每张发票显式提供")
}

func sellerProfileResult(attempted, success bool, taxpayerType string, defaulted bool, message string) map[string]any {
	return map[string]any{
		"attempted":              attempted,
		"success":                success,
		"taxpayer_type":          taxpayerType,
		"defaulted_invoice_type": defaulted,
		"message":                message,
	}
}
