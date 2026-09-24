package issue

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

type FieldSpec struct {
	Module         string
	Path           string
	Required       bool
	Message        string
	Aliases        []string
	Transform      string
	CommonDictName string
}

var invoiceTypeMap = map[string]string{
	"普票":      "普通发票",
	"普通票":     "普通发票",
	"普通发票":    "普通发票",
	"专票":      "增值税专用发票",
	"增值税专票":   "增值税专用发票",
	"增值税专用发票": "增值税专用发票",
}

var fieldSpecs = []FieldSpec{
	{"发票基础信息", "uscc", true, "请提供销售方税号", []string{"uscc", "销售方税号", "销方税号", "selleruscc", "sellertaxno"}, "string", ""},
	{"发票基础信息", "areaCode", true, "请提供开票地区编码，或先提供明确地区名称用于后续映射", []string{"areacode", "地区编码", "开票地区编码", "区域编码"}, "int", ""},
	{"发票基础信息", "personalAccount", true, "请提供登录账号、手机号或用户名", []string{"personalaccount", "登录账号", "手机号", "用户名", "账号"}, "string", ""},
	{"发票基础信息", "info.0.basicInfo.invoiceType", true, "请提供发票类型，仅支持普通发票或增值税专用发票", []string{"invoicetype", "发票类型", "票种"}, "invoice_type", ""},
	{"发票基础信息", "info.0.basicInfo.giveUpReason", false, "", []string{"giveupreason", "放弃减税优惠原因"}, "string", ""},
	{"销售方信息", "info.0.sellerInfo.name", false, "", []string{"sellerinfoname", "销方名称", "销售方名称"}, "string", ""},
	{"销售方信息", "info.0.sellerInfo.address", false, "", []string{"sellerinfoaddress", "销方地址", "销售方地址"}, "string", ""},
	{"销售方信息", "info.0.sellerInfo.phone", false, "", []string{"sellerinfophone", "销方电话", "销售方电话"}, "string", ""},
	{"销售方信息", "info.0.sellerInfo.bank", false, "", []string{"sellerinfobank", "销方开户银行", "销售方开户银行", "销方银行"}, "string", ""},
	{"销售方信息", "info.0.sellerInfo.bankAccount", false, "", []string{"sellerinfobankaccount", "销方银行账号", "销售方银行账号", "销方账号"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.name", true, "请提供购方名称", []string{"purchaserinfoname", "购方名称", "购买方名称", "客户名称"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.isNaturePerson", false, "", []string{"purchaserinfoisnatureperson", "自然人", "个人开票", "开给个人"}, "bool", ""},
	{"购买方信息", "info.0.purchaserInfo.uscc", false, "", []string{"purchaserinfouscc", "购方税号", "购买方税号"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.address", false, "", []string{"purchaserinfoaddress", "购方地址", "购买方地址"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.phone", false, "", []string{"purchaserinfophone", "购方电话", "购买方电话"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.bank", false, "", []string{"purchaserinfobank", "购方开户银行", "购买方开户银行"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.bankAccount", false, "", []string{"purchaserinfobankaccount", "购方银行账号", "购买方银行账号"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.identificationTypeCode", false, "", []string{"identificationtypecode", "证件类型编码", "购方证件类型编码", "证件类型", "购方证件类型", "自然人证件类型"}, "dict_code", "证件类型"},
	{"购买方信息", "info.0.purchaserInfo.identificationNumber", false, "", []string{"identificationnumber", "证件号码", "购方证件号码"}, "string", ""},
	{"购买方信息", "info.0.purchaserInfo.nationCode", false, "", []string{"nationcode", "国籍编码", "地区编码", "国籍或地区", "购方国籍", "购方国籍或地区", "自然人国籍", "自然人国籍或地区"}, "dict_code", "国籍或地区"},
	{"备注信息", "info.0.noteInfo.note", false, "", []string{"noteinfonote", "备注", "备注信息"}, "string", ""},
	{"备注信息", "info.0.noteInfo.payee", false, "", []string{"noteinfopayee", "收款人"}, "string", ""},
	{"备注信息", "info.0.noteInfo.reviewer", false, "", []string{"noteinforeviewer", "复核人"}, "string", ""},
	{"经办人信息", "info.0.personInfo.name", false, "", []string{"personinfoname", "经办人姓名", "姓名"}, "string", ""},
	{"经办人信息", "info.0.personInfo.nation", false, "", []string{"personinfonation", "经办人国籍", "国籍", "经办人国籍或地区", "国籍或地区"}, "dict_code", "国籍或地区"},
	{"经办人信息", "info.0.personInfo.certificateType", false, "", []string{"personinfocertificatetype", "经办人证件类型", "证件类型", "经办人证件类型编码"}, "dict_code", "证件类型"},
	{"经办人信息", "info.0.personInfo.certificateCode", false, "", []string{"personinfocertificatecode", "经办人证件号码", "证件号"}, "string", ""},
	{"经办人信息", "info.0.personInfo.uscc", false, "", []string{"personinfouscc", "经办人纳税人识别号", "经办人税号"}, "string", ""},
}

var lineItemSpecs = []FieldSpec{
	{"发票明细", "projectName", true, "请提供项目名称", []string{"projectname", "项目名称", "货物名称", "商品名称"}, "string", ""},
	{"发票明细", "spbm", true, "请提供商品编码，或后续走自动赋码流程", []string{"spbm", "商品编码"}, "string", ""},
	{"发票明细", "amount", true, "请提供项目金额", []string{"amount", "不含税金额", "税前金额"}, "currency", ""},
	{"发票明细", "taxInclusiveAmount", false, "", []string{"taxinclusiveamount", "金额", "项目金额", "含税金额", "价税合计金额"}, "currency", ""},
	{"发票明细", "taxRate", false, "", []string{"taxrate", "税率", "征收率"}, "tax_rate", ""},
	{"发票明细", "tax", true, "请提供项目税额", []string{"tax", "税额", "项目税额"}, "currency", ""},
	{"发票明细", "size", false, "", []string{"size", "规格型号", "型号"}, "string", ""},
	{"发票明细", "unit", false, "", []string{"unit", "单位"}, "string", ""},
	{"发票明细", "price", false, "", []string{"不含税单价", "税前单价", "priceexcludingtax"}, "unit_price", ""},
	{"发票明细", "taxInclusivePrice", false, "", []string{"单价", "含税单价", "taxinclusiveprice", "pricewithtax"}, "unit_price", ""},
	{"发票明细", "quantity", false, "", []string{"quantity", "数量"}, "string", ""},
	{"发票明细", "discountInfo.discount", false, "", []string{"discount", "折扣", "折扣比例", "discountRate"}, "number", ""},
	{"发票明细", "discountInfo.discountAmount", false, "", []string{"discountamount", "折扣金额", "优惠金额"}, "currency", ""},
	{"发票明细", "xsyhzcbz", false, "", []string{"xsyhzcbz", "优惠政策标识"}, "string", ""},
	{"发票明细", "zzstsgl", false, "", []string{"zzstsgl", "增值税特殊管理说明"}, "string", ""},
	{"发票明细", "sptm", false, "", []string{"sptm", "商品条码"}, "string", ""},
	{"发票明细", "mtzlDm", false, "", []string{"mtzldm", "煤炭种类", "煤炭种类代码", "煤种"}, "dict_code", "煤炭种类"},
}

var totalFieldSpecs = []FieldSpec{
	{"发票明细", "info.0.invoiceDetail.amount", true, "请提供合计金额", []string{"invoicedetailamount", "合计金额", "明细合计金额"}, "currency", ""},
	{"发票明细", "info.0.invoiceDetail.tax", true, "请提供合计税额", []string{"invoicedetailtax", "合计税额", "明细合计税额"}, "currency", ""},
	{"发票明细", "info.0.invoiceDetail.total", true, "请提供价税合计", []string{"invoicedetailtotal", "价税合计", "合计价税"}, "currency", ""},
}

var paymentFieldSpecs = []FieldSpec{
	{"支付信息", "payChannelCode", false, "", []string{"paychannelcode", "支付渠道编码", "支付渠道", "支付方式"}, "dict_code", "支付渠道"},
	{"支付信息", "transactionOrderNo", false, "", []string{"transactionorderno", "交易单号", "订单号"}, "string", ""},
}

func CheckSingle(inputPath, taskFile string, generateFplsh, pretty bool) (string, error) {
	return CheckSingleWithClient(inputPath, taskFile, generateFplsh, pretty, nil)
}

/**
 * CheckSingleWithClient 创建带企业快照的单张开票检查任务。
 *
 * @param inputPath 原始开票输入 JSON 路径。
 * @param taskFile 可选的任务文件路径。
 * @param generateFplsh 是否生成发票流水号。
 * @param pretty 是否使用格式化 JSON 保存任务。
 * @param client 可选的 Gateway APIClient。
 * @returns 新任务文件路径。
 */
func CheckSingleWithClient(inputPath, taskFile string, generateFplsh, pretty bool, client *common.APIClient) (string, error) {
	raw, err := loadJSONInputFlexible(inputPath)
	if err != nil {
		return "", err
	}
	normalized, enterpriseContext, err := prepareIssueTaskInput(raw, taskFile, client)
	if err != nil {
		return "", err
	}
	result, err := NormalizeInputData(normalized, generateFplsh, false, "")
	if err != nil {
		return "", err
	}
	taskPath, err := PersistCheckResult(normalized, result, taskFile, pretty, "single", enterpriseContext)
	if err != nil {
		return "", err
	}
	return taskPath, nil
}

func CheckBatch(inputPath, taskFile string, generateFplsh, pretty bool) (string, error) {
	return CheckBatchWithClient(inputPath, taskFile, generateFplsh, pretty, nil)
}

/**
 * CheckBatchWithClient 创建带企业快照的批量开票检查任务。
 *
 * @param inputPath 原始批量开票输入 JSON 路径。
 * @param taskFile 可选的任务文件路径。
 * @param generateFplsh 是否生成发票流水号。
 * @param pretty 是否使用格式化 JSON 保存任务。
 * @param client 可选的 Gateway APIClient。
 * @returns 新任务文件路径。
 */
func CheckBatchWithClient(inputPath, taskFile string, generateFplsh, pretty bool, client *common.APIClient) (string, error) {
	raw, err := loadJSONInputFlexible(inputPath)
	if err != nil {
		return "", err
	}
	normalized, enterpriseContext, err := prepareIssueTaskInput(raw, taskFile, client)
	if err != nil {
		return "", err
	}
	result, err := NormalizeBatchInputData(normalized, generateFplsh, false)
	if err != nil {
		return "", err
	}
	taskPath, err := PersistCheckResult(normalized, result, taskFile, pretty, "batch", enterpriseContext)
	if err != nil {
		return "", err
	}
	return taskPath, nil
}

/**
 * PersistCheckResult 将检查结果和企业快照一起写入新的开票任务。
 *
 * @param rawData 已由企业快照补全的原始开票输入。
 * @param result 标准化与字段校验结果。
 * @param taskFile 可选的任务文件路径。
 * @param pretty 是否格式化任务 JSON。
 * @param taskType single 或 batch。
 * @param enterpriseContext 通过企业列表精确匹配得到的企业快照。
 * @returns 已写入的任务文件路径。
 */
func PersistCheckResult(rawData, result map[string]any, taskFile string, pretty bool, taskType string, enterpriseContext map[string]any) (string, error) {
	state, err := CreateTaskState(rawData, taskFile, taskType, enterpriseContext)
	if err != nil {
		return "", err
	}
	persisted := map[string]any{}
	for k, v := range result {
		persisted[k] = v
	}
	if legacyPythonTruthy(persisted["validation_passed"]) {
		if payload, ok := persisted["invoice_payload_json"].(map[string]any); ok {
			previewPath := strings.TrimSpace(common.StringValue(persisted["invoice_preview_path"]))
			if previewPath == "" {
				previewDir := strings.TrimSpace(common.StringValue(GetPath(state.Data, "storage.preview_dir")))
				if previewDir != "" {
					previewPath = filepath.Join(previewDir, fileStem(state.Path)+".html")
				}
				if generatedPath, previewErr := WriteInvoicePreviewFile(payload, previewPath); previewErr == nil {
					previewPath = generatedPath
				} else {
					if outdated, ok := skillupdate.FindOutdatedError(previewErr); ok {
						return "", outdated
					}
					persisted["invoice_preview_error"] = fmt.Sprintf("发票预览生成失败：%v", previewErr)
				}
			}
			persisted["invoice_preview_path"] = previewPath
			persisted["invoice_preview_generated"] = previewPath != ""
			previewMarkdown := buildPreviewMarkdownWithWarnings(persisted, payload, previewPath)
			persisted["invoice_preview_markdown"] = previewMarkdown
			persisted["invoice_preview_markdown_generated"] = previewMarkdown != ""
			if previewMarkdown != "" {
				previewMarkdownPath := strings.TrimSpace(common.StringValue(persisted["invoice_preview_markdown_path"]))
				if previewMarkdownPath == "" {
					previewDir := strings.TrimSpace(common.StringValue(GetPath(state.Data, "storage.preview_dir")))
					if previewDir != "" {
						previewMarkdownPath = filepath.Join(previewDir, fileStem(state.Path)+".md")
					}
				}
				if previewMarkdownPath != "" {
					if err := writeTextFile(previewMarkdownPath, previewMarkdown); err == nil {
						persisted["invoice_preview_markdown_path"] = previewMarkdownPath
					}
					common.AppendMessageLog("发票预览已生成 ", previewMarkdownPath, "读取md文件内容，并原样渲染回对话中\n")
				}
			}
		}
	}
	state.Data["check_result"] = persisted
	state.Data["issue_result"] = map[string]any{}
	state.Data["task_status"] = "checked"
	if !legacyPythonTruthy(persisted["validation_passed"]) {
		state.Data["task_status"] = "check_failed"
	} else if legacyPythonTruthy(persisted["invoice_preview_generated"]) {
		state.Data["task_status"] = "preview_ready"
	}
	if payload, ok := persisted["invoice_payload_json"].(map[string]any); ok {
		state.Data["invoice_payload_json"] = payload
	}
	if err := state.Save(pretty); err != nil {
		return "", err
	}
	return state.Path, nil
}

// NormalizeBatchInputData validates batch issue input and preserves mandatory version-gate errors.
func NormalizeBatchInputData(rawData map[string]any, generateFplsh, generatePreview bool) (map[string]any, error) {
	shared := extractSharedTopLevelFields(rawData)
	sources := extractBatchInvoiceSources(rawData)
	identity := resolveTaxpayerIdentity(
		rawData,
		extractTaxpayerTypeOverride(rawData),
	)
	sellerEnterpriseName := strings.TrimSpace(common.StringValue(rawData["enterpriseName"]))
	itemResults := []map[string]any{}
	normalizedItems := []any{}
	allPassed := len(sources) > 0
	for invoiceIndex, source := range sources {
		singleRaw := buildSingleInvoiceRawData(shared, source)
		result, err := normalizeInputData(singleRaw, normalizeInputOptions{
			generateFplsh:        generateFplsh,
			invoiceIndex:         invoiceIndex + 1,
			identity:             identity,
			sellerEnterpriseName: sellerEnterpriseName,
		})
		if err != nil {
			return nil, err
		}
		itemResults = append(itemResults, buildBatchItemResult(invoiceIndex+1, result))
		if !legacyPythonTruthy(result["validation_passed"]) {
			allPassed = false
		}
		if payload, ok := result["invoice_payload_json"].(map[string]any); ok {
			if info, ok := GetPath(payload, "info.0").(map[string]any); ok {
				normalizedItems = append(normalizedItems, info)
			}
		}
	}
	if len(sources) == 0 {
		allPassed = false
	}
	payload := map[string]any{}
	for _, key := range []string{"uscc", "areaCode", "personalAccount"} {
		if HasMeaningfulValue(shared[key]) {
			payload[key] = shared[key]
		}
	}
	payload["info"] = normalizedItems
	missing := flattenBatchMissingFields(itemResults)
	validationErrors := flattenBatchValidationErrors(itemResults)
	if len(sources) == 0 {
		addMissingField(missing, "批量开票", "info[]", "请至少提供 1 条开票对象")
	}
	previewPath := ""
	previewMarkdown := ""
	if allPassed && generatePreview {
		if path, err := WriteInvoicePreviewFile(payload, ""); err == nil {
			previewPath = path
			previewMarkdown = buildPreviewMarkdownWithWarnings(map[string]any{"item_results": itemResults}, payload, previewPath)
		} else if outdated, ok := skillupdate.FindOutdatedError(err); ok {
			return nil, outdated
		}
	}
	return map[string]any{
		"validation_passed":                  allPassed && len(missing) == 0 && len(validationErrors) == 0,
		"missing_fields_by_module":           missing,
		"validation_errors":                  validationErrors,
		"invoice_payload_json":               pruneEmptyValues(payload),
		"invoice_preview_confirmed":          false,
		"invoice_preview_generated":          previewPath != "",
		"invoice_preview_path":               previewPath,
		"invoice_preview_markdown_path":      "",
		"invoice_preview_markdown_generated": previewMarkdown != "",
		"invoice_preview_markdown":           previewMarkdown,
		"item_results":                       itemResults,
		"taxpayer_resolution":                identity.resultMap(),
	}, nil
}

// NormalizeInputData validates one issue input and preserves mandatory version-gate errors.
func NormalizeInputData(rawData map[string]any, generateFplsh, generatePreview bool, previewOutputPath string) (map[string]any, error) {
	identity := resolveTaxpayerIdentity(
		rawData,
		extractTaxpayerTypeOverride(rawData),
	)
	return normalizeInputData(rawData, normalizeInputOptions{
		generateFplsh:        generateFplsh,
		generatePreview:      generatePreview,
		previewOutputPath:    previewOutputPath,
		invoiceIndex:         1,
		identity:             identity,
		sellerEnterpriseName: strings.TrimSpace(common.StringValue(rawData["enterpriseName"])),
	})
}

type normalizeInputOptions struct {
	generateFplsh        bool
	generatePreview      bool
	previewOutputPath    string
	invoiceIndex         int
	identity             taxpayerIdentity
	sellerEnterpriseName string
}

func normalizeInputData(rawData map[string]any, options normalizeInputOptions) (map[string]any, error) {
	payload := defaultTemplate()
	recognized, missing, conflicts := extractTopLevelFields(rawData, payload)
	applyLocalSellerName(payload, options.sellerEnterpriseName)
	autofillPurchaserInfoResult, err := autofillPurchaserInfoChecked(payload, missing)
	if err != nil {
		return nil, err
	}
	sellerProfileResult := applySellerProfileInvoiceDefaults(payload, missing, options.identity)
	lineItems, lineMissing, err := buildLineItemsChecked(rawData)
	if err != nil {
		return nil, err
	}
	SetPath(payload, "info.0.invoiceDetail.data", lineItems)
	for module, entries := range lineMissing {
		for _, entry := range entries {
			missing[module] = append(missing[module], entry)
		}
	}
	autofillTaxRateResult, err := autofillLineItemTaxRates(taxRateAutofillContext{
		lineItems: lineItems,
		missing:   missing,
		identity:  options.identity,
	})
	if err != nil {
		return nil, err
	}
	amountValidationErrors := normalizeLineItemAmounts(lineItems, missing, options.invoiceIndex)
	normalizeLineItemDiscounts(lineItems, missing)
	autofillInvoiceDetailTotals(payload, missing)
	payments, paymentMissing := buildPaymentInfo(rawData)
	if len(payments) > 0 {
		SetPath(payload, "info.0.paymentInfo", payments)
	}
	for module, entries := range paymentMissing {
		missing[module] = append(missing[module], entries...)
	}
	postProcessPayload(payload, options.generateFplsh)
	validationErrors := append(amountValidationErrors, validatePayload(payload, missing)...)
	taxRateValidation, err := validateTaxRateReferences(payload, options.identity, autofillTaxRateResult)
	if err != nil {
		return nil, err
	}
	serializeInvoiceCurrencyFields(payload)
	prunedPayload, _ := pruneEmptyValues(payload).(map[string]any)
	cleanMissing := cleanMissingFields(missing)
	validationPassed := len(cleanMissing) == 0 && len(validationErrors) == 0
	previewPath := ""
	previewMarkdown := ""
	previewErr := ""
	if validationPassed && options.generatePreview {
		if path, err := WriteInvoicePreviewFile(prunedPayload, options.previewOutputPath); err == nil {
			previewPath = path
			previewMarkdown = BuildPreviewMarkdown(prunedPayload, previewPath)
		} else {
			if outdated, ok := skillupdate.FindOutdatedError(err); ok {
				return nil, outdated
			}
			previewErr = fmt.Sprintf("发票预览生成失败：%v", err)
		}
	}
	out := map[string]any{
		"validation_passed":                  validationPassed,
		"missing_fields_by_module":           cleanMissing,
		"validation_errors":                  validationErrors,
		"recognized_fields_by_module":        recognized,
		"conflicts_by_module":                conflicts,
		"invoice_payload_json":               prunedPayload,
		"invoice_preview_confirmed":          false,
		"invoice_preview_generated":          previewPath != "",
		"invoice_preview_path":               previewPath,
		"invoice_preview_markdown_path":      "",
		"invoice_preview_markdown_generated": previewMarkdown != "",
		"invoice_preview_markdown":           previewMarkdown,
		"tax_rate_validation":                taxRateValidation,
		"purchaser_autofill":                 autofillPurchaserInfoResult,
		"seller_profile":                     sellerProfileResult,
		"taxpayer_resolution":                options.identity.resultMap(),
	}
	if previewErr != "" {
		out["invoice_preview_error"] = previewErr
	}
	return out, nil
}

func defaultTemplate() map[string]any {
	return map[string]any{
		"uscc":            "",
		"areaCode":        0,
		"personalAccount": "",
		"info": []any{map[string]any{
			"basicInfo":     map[string]any{"invoiceType": "", "giveUpReason": ""},
			"sellerInfo":    map[string]any{"name": "", "address": "", "phone": "", "bank": "", "bankAccount": "", "showBankInfo": "", "showPersonInfo": ""},
			"purchaserInfo": map[string]any{"isNaturePerson": "", "name": "", "uscc": "", "address": "", "phone": "", "showBankInfo": "", "bank": "", "bankAccount": "", "showPersonInfo": "", "identificationTypeCode": "", "identificationNumber": "", "nationCode": ""},
			"isIncludeTax":  true,
			"invoiceDetail": map[string]any{"data": []any{}, "amount": "", "tax": "", "total": ""},
			"noteInfo":      map[string]any{"note": "", "payee": "", "reviewer": ""},
			"paymentInfo":   []any{},
			"personInfo":    map[string]any{"name": "", "nation": "", "certificateType": "", "certificateCode": "", "uscc": ""},
			"fplsh":         "",
		}},
	}
}

func extractTopLevelFields(rawData, payload map[string]any) (map[string]any, map[string][]map[string]any, map[string][]map[string]any) {
	flat := buildFlatIndex(rawData)
	recognized := map[string]any{}
	missing := map[string][]map[string]any{}
	conflicts := map[string][]map[string]any{}
	specs := append([]FieldSpec{}, fieldSpecs...)
	specs = append(specs, totalFieldSpecs...)
	for _, spec := range specs {
		value, extras, errMsg := extractField(rawData, flat, spec)
		if HasMeaningfulValue(value) {
			SetPath(payload, spec.Path, value)
			moduleObj, _ := recognized[spec.Module].(map[string]any)
			if moduleObj == nil {
				moduleObj = map[string]any{}
				recognized[spec.Module] = moduleObj
			}
			moduleObj[spec.Path] = value
		} else if errMsg != "" {
			addMissingField(missing, spec.Module, spec.Path, errMsg)
		} else if spec.Required {
			addMissingField(missing, spec.Module, spec.Path, spec.Message)
		}
		if len(extras) > 0 {
			conflicts[spec.Module] = append(conflicts[spec.Module], map[string]any{"field": spec.Path, "selected": value, "candidates": append([]any{value}, extras...)})
		}
	}
	return recognized, missing, conflicts
}

func buildLineItems(rawData map[string]any) ([]any, map[string][]map[string]any) {
	items, missing, _ := buildLineItemsChecked(rawData)
	return items, missing
}

/**
 * buildLineItemsChecked 标准化发票明细，并为缺少商品编码的有效明细执行智能赋码。
 *
 * @param rawData 原始开票输入，包含销售方税号和发票明细。
 * @returns 标准化明细、缺失字段集合以及必须阻断流程的错误。
 * @throws skillupdate.OutdatedError 当当前 Skill 版本过低时返回。
 */
func buildLineItemsChecked(rawData map[string]any) ([]any, map[string][]map[string]any, error) {
	source := findDetailsSource(rawData)
	missing := map[string][]map[string]any{"发票明细": {}}
	items := []any{}
	for index, rawItem := range source {
		flat := buildFlatIndex(rawItem)
		item := map[string]any{}
		for _, spec := range lineItemSpecs {
			value, _, errMsg := extractField(rawItem, flat, spec)
			if HasMeaningfulValue(value) {
				setLineItemPath(item, spec.Path, value)
			} else if errMsg != "" {
				addMissingField(missing, "发票明细", fmt.Sprintf("invoiceDetail.data[%d].%s", index, spec.Path), errMsg)
			} else if spec.Required {
				addMissingField(missing, "发票明细", fmt.Sprintf("invoiceDetail.data[%d].%s", index, spec.Path), spec.Message)
			}
		}
		if !HasMeaningfulValue(item["spbm"]) && HasMeaningfulValue(item["projectName"]) {
			if code := getProductCodeByProjectName(item["projectName"]); code != "" {
				item["spbm"] = code
				removeMissingField(missing, "发票明细", fmt.Sprintf("invoiceDetail.data[%d].spbm", index))
			}
		}
		applyTaxRateSpecialPolicy(item)
		items = append(items, item)
	}
	if err := autofillMissingProductCodes(items, missing, common.StringValue(rawData["uscc"])); err != nil {
		return nil, nil, err
	}
	if len(items) == 0 {
		addMissingField(missing, "发票明细", "invoiceDetail.data[]", "请至少提供 1 条发票明细")
	}
	return items, cleanMissingFields(missing), nil
}

func buildPaymentInfo(rawData map[string]any) ([]any, map[string][]map[string]any) {
	missing := map[string][]map[string]any{}
	paymentValue := firstNonNil(GetPath(rawData, "info.0.paymentInfo"), GetPath(rawData, "paymentInfo"), rawData["paymentInfo"])
	if list, ok := asList(paymentValue); ok {
		out := []any{}
		for idx, itemAny := range list {
			item, ok := itemAny.(map[string]any)
			if !ok {
				continue
			}
			flat := buildFlatIndex(item)
			normalized := map[string]any{}
			for _, spec := range paymentFieldSpecs {
				value, _, errMsg := extractField(item, flat, spec)
				if HasMeaningfulValue(value) {
					normalized[spec.Path] = value
				} else if errMsg != "" {
					addMissingField(missing, "支付信息", fmt.Sprintf("info.0.paymentInfo[%d].%s", idx, spec.Path), errMsg)
				}
			}
			if len(normalized) > 0 {
				out = append(out, normalized)
			}
		}
		return out, missing
	}
	flat := buildFlatIndex(rawData)
	normalized := map[string]any{}
	for _, spec := range paymentFieldSpecs {
		value, _, errMsg := extractField(rawData, flat, spec)
		if HasMeaningfulValue(value) {
			normalized[spec.Path] = value
		} else if errMsg != "" {
			addMissingField(missing, "支付信息", "info.0.paymentInfo[0]."+spec.Path, errMsg)
		}
	}
	if len(normalized) == 0 {
		return []any{}, missing
	}
	return []any{normalized}, missing
}

func extractField(rawData map[string]any, flat []flatEntry, spec FieldSpec) (any, []any, string) {
	explicit := []string{spec.Path}
	if strings.HasPrefix(spec.Path, "info.0.") {
		explicit = append(explicit, strings.TrimPrefix(spec.Path, "info.0."))
	}
	candidates := collectCandidates(rawData, flat, spec.Aliases, explicit)
	transformed := []any{}
	errors := []string{}
	for _, candidate := range candidates {
		value, errMsg := transformSpecValue(spec, candidate)
		if errMsg != "" {
			errors = append(errors, errMsg)
			continue
		}
		if HasMeaningfulValue(value) {
			transformed = append(transformed, value)
		}
	}
	unique := uniqueValues(transformed)
	if len(unique) == 0 {
		if len(errors) > 0 {
			return nil, nil, errors[0]
		}
		return nil, nil, ""
	}
	return unique[0], unique[1:], ""
}

func transformSpecValue(spec FieldSpec, value any) (any, string) {
	switch spec.Transform {
	case "string":
		return strings.TrimSpace(common.StringValue(value)), ""
	case "int":
		code, err := common.NormalizeStandardAreaCode(value)
		if err != nil {
			return nil, err.Error()
		}
		return code, ""
	case "bool":
		return normalizeBool(value), ""
	case "number":
		d, ok := ToDecimal(value)
		if !ok {
			return nil, "必须为有效数字"
		}
		return decimalToNative(d), ""
	case "currency":
		d, ok := ToDecimal(value)
		if !ok {
			return nil, "必须为有效数字"
		}
		return decimalToString(quantizeCurrency(d)), ""
	case "unit_price":
		d, ok := ToDecimal(value)
		if !ok {
			return nil, "必须为有效数字"
		}
		return decimalToString(quantizeUnitPrice(d)), ""
	case "invoice_type":
		text := strings.TrimSpace(common.StringValue(value))
		if mapped := invoiceTypeMap[text]; mapped != "" {
			return mapped, ""
		}
		return nil, "发票类型仅支持普通发票或增值税专用发票"
	case "tax_rate":
		text, err := strictNormalizeTaxRateInput(value)
		if err != "" {
			return nil, err
		}
		return text, ""
	case "dict_code":
		code, err := resolveCommonDictCode(spec.CommonDictName, value)
		if err != "" {
			return nil, err
		}
		return code, ""
	default:
		return value, ""
	}
}

func findDetailsSource(rawData map[string]any) []map[string]any {
	candidates := []any{
		GetPath(rawData, "info.0.invoiceDetail.data"),
		GetPath(rawData, "invoiceDetail.data"),
		rawData["details"],
		rawData["items"],
		rawData["明细"],
	}
	for _, candidate := range candidates {
		if list, ok := asList(candidate); ok {
			out := []map[string]any{}
			for _, item := range list {
				if obj, ok := item.(map[string]any); ok {
					out = append(out, obj)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	return []map[string]any{}
}

func validatePayload(payload map[string]any, missing map[string][]map[string]any) []string {
	errors := []string{}
	detailItems, _ := asList(GetPath(payload, "info.0.invoiceDetail.data"))
	totals := computeTotals(payload)
	for _, path := range []string{"amount", "tax", "total"} {
		full := "info.0.invoiceDetail." + path
		if len(detailItems) > 0 && !HasMeaningfulValue(GetPath(payload, full)) {
			SetPath(payload, full, totals[path])
			removeMissingField(missing, "发票明细", full)
		}
	}
	declaredAmount, aok := ToDecimal(GetPath(payload, "info.0.invoiceDetail.amount"))
	declaredTax, tok := ToDecimal(GetPath(payload, "info.0.invoiceDetail.tax"))
	declaredTotal, took := ToDecimal(GetPath(payload, "info.0.invoiceDetail.total"))
	computedAmount, _ := ToDecimal(totals["amount"])
	computedTax, _ := ToDecimal(totals["tax"])
	computedTotal, _ := ToDecimal(totals["total"])
	if aok && !declaredAmount.Equal(computedAmount) {
		errors = append(errors, fmt.Sprintf("[INVOICE_AMOUNT_SUM_MISMATCH] 整张发票不含税合计与明细汇总不一致：invoiceDetail.amount=%s，明细汇总应为%s，差额为%s。调整建议：请核对整票不含税合计或各明细amount。", declaredAmount.StringFixed(2), computedAmount.StringFixed(2), declaredAmount.Sub(computedAmount).Abs().StringFixed(2)))
	}
	if tok && !declaredTax.Equal(computedTax) {
		errors = append(errors, fmt.Sprintf("[INVOICE_TAX_SUM_MISMATCH] 整张发票税额合计与明细汇总不一致：invoiceDetail.tax=%s，明细汇总应为%s，差额为%s。调整建议：请核对整票税额合计或各明细tax。", declaredTax.StringFixed(2), computedTax.StringFixed(2), declaredTax.Sub(computedTax).Abs().StringFixed(2)))
	}
	if took && !declaredTotal.Equal(computedTotal) {
		errors = append(errors, fmt.Sprintf("[INVOICE_TOTAL_SUM_MISMATCH] 整张发票价税合计与明细汇总不一致：invoiceDetail.total=%s，明细汇总应为%s，差额为%s。调整建议：请核对整票价税合计，或各明细amount和tax。", declaredTotal.StringFixed(2), computedTotal.StringFixed(2), declaredTotal.Sub(computedTotal).Abs().StringFixed(2)))
	}
	invoiceType := strings.TrimSpace(common.StringValue(GetPath(payload, "info.0.basicInfo.invoiceType")))
	purchaser, _ := GetPath(payload, "info.0.purchaserInfo").(map[string]any)
	isNaturePerson := isTruthy(purchaser["isNaturePerson"])
	if invoiceType == "增值税专用发票" && isNaturePerson {
		addMissingField(missing, "购买方信息", "info.0.purchaserInfo.isNaturePerson", "增值税专用发票暂不支持自然人购方；请改用普通发票，或提供企业购方名称和税号并将 isNaturePerson 设为 false")
		return errors
	}
	if invoiceType == "增值税专用发票" && !HasMeaningfulValue(purchaser["uscc"]) {
		addMissingField(missing, "购买方信息", "info.0.purchaserInfo.uscc", "增值税专用发票请提供购方税号")
	}
	return errors
}

func autofillPurchaserInfo(payload map[string]any, missing map[string][]map[string]any) map[string]any {
	result, _ := autofillPurchaserInfoChecked(payload, missing)
	return result
}

// autofillPurchaserInfoChecked preserves an outdated-version error from purchaser search.
func autofillPurchaserInfoChecked(payload map[string]any, missing map[string][]map[string]any) (map[string]any, error) {
	namePath := "info.0.purchaserInfo.name"
	usccPath := "info.0.purchaserInfo.uscc"
	if isTruthy(GetPath(payload, "info.0.purchaserInfo.isNaturePerson")) {
		return map[string]any{"attempted": false, "success": false, "filled_fields": map[string]any{}, "message": "自然人购买方不执行企业搜索自动补全"}, nil
	}
	name := strings.TrimSpace(common.StringValue(GetPath(payload, namePath)))
	uscc := strings.TrimSpace(common.StringValue(GetPath(payload, usccPath)))
	query := purchaserEnterpriseSearchQuery(name, uscc)
	if query == "" {
		return map[string]any{"attempted": false, "success": false, "filled_fields": map[string]any{}, "message": "购买方名称和税号已完整或不满足企业搜索条件，未自动补全"}, nil
	}
	candidates, err := fetchEnterpriseSearchCandidates(query)
	if err != nil {
		if outdated, ok := skillupdate.FindOutdatedError(err); ok {
			return nil, outdated
		}
		return map[string]any{"attempted": true, "success": false, "filled_fields": map[string]any{}, "message": "自动搜索购买方企业信息失败：" + err.Error()}, nil
	}
	if len(candidates) == 0 {
		return map[string]any{"attempted": true, "success": false, "filled_fields": map[string]any{}, "message": "未查询到可用于补全的购买方企业信息"}, nil
	}
	if len(candidates) > 1 {
		return map[string]any{
			"attempted":            true,
			"success":              false,
			"needs_user_selection": true,
			"query":                query,
			"candidate_count":      len(candidates),
			"candidates":           candidates,
			"selected_candidate":   nil,
			"filled_fields":        map[string]any{},
			"message":              "查询到多个购买方企业，可供用户选择；如用户不选择，将继续不带购买方税号生成草稿",
		}, nil
	}
	match := candidates[0]
	filled := map[string]any{}
	matchedName := strings.TrimSpace(common.StringValue(match["name"]))
	matchedUSCC := strings.TrimSpace(common.StringValue(match["uscc"]))
	if matchedName != "" {
		SetPath(payload, namePath, matchedName)
		filled[namePath] = matchedName
		removeMissingFieldByPath(missing, namePath)
	}
	if matchedUSCC != "" && !HasMeaningfulValue(GetPath(payload, usccPath)) {
		SetPath(payload, usccPath, matchedUSCC)
		filled[usccPath] = matchedUSCC
		removeMissingFieldByPath(missing, usccPath)
	}
	if len(filled) == 0 {
		return map[string]any{"attempted": true, "success": false, "filled_fields": filled, "message": "已查询到购买方企业信息，但当前字段无需补全"}, nil
	}
	return map[string]any{"attempted": true, "success": true, "filled_fields": filled, "message": "已根据企业搜索结果自动补全购买方名称或税号"}, nil
}

func purchaserEnterpriseSearchQuery(name, uscc string) string {
	if looksLikeUSCC(name) {
		return name
	}
	if looksLikeUSCC(uscc) {
		if name == "" {
			return uscc
		}
		return ""
	}
	if name != "" {
		return name
	}
	return ""
}

func looksLikeUSCC(value string) bool {
	text := strings.ToUpper(strings.TrimSpace(value))
	if len(text) != 18 {
		return false
	}
	return regexp.MustCompile(`^[0-9A-Z]{18}$`).MatchString(text)
}

func autofillInvoiceDetailTotals(payload map[string]any, missing map[string][]map[string]any) {
	totals := computeTotals(payload)
	for _, key := range []string{"amount", "tax", "total"} {
		path := "info.0.invoiceDetail." + key
		if missingHasField(missing, "发票明细", path) {
			SetPath(payload, path, totals[key])
			removeMissingField(missing, "发票明细", path)
		}
	}
}

func computeTotals(payload map[string]any) map[string]any {
	items, _ := asList(GetPath(payload, "info.0.invoiceDetail.data"))
	amountTotal := decimal.Zero
	taxTotal := decimal.Zero
	for _, itemAny := range items {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		amount, amountOK := ToDecimal(item["amount"])
		tax, taxOK := ToDecimal(item["tax"])
		discount := map[string]any{}
		if obj, ok := item["discountInfo"].(map[string]any); ok {
			discount = obj
		}
		discountAmount, _ := ToDecimal(discount["discountAmount"])
		discountTax, _ := ToDecimal(discount["discountTax"])
		if amountOK {
			amountTotal = amountTotal.Add(amount).Sub(discountAmount)
		}
		if taxOK {
			taxTotal = taxTotal.Add(tax).Sub(discountTax)
		}
	}
	amountTotal = quantizeCurrency(amountTotal)
	taxTotal = quantizeCurrency(taxTotal)
	return map[string]any{
		"amount": amountTotal.StringFixed(2),
		"tax":    taxTotal.StringFixed(2),
		"total":  quantizeCurrency(amountTotal.Add(taxTotal)).StringFixed(2),
	}
}

func postProcessPayload(payload map[string]any, generateFplsh bool) {
	info, _ := GetPath(payload, "info.0").(map[string]any)
	if info == nil {
		return
	}
	if !HasMeaningfulValue(info["fplsh"]) && generateFplsh {
		info["fplsh"] = newUUIDLike()
	}
	info["isIncludeTax"] = true
}

func serializeInvoiceCurrencyFields(payload map[string]any) {
	items, _ := asList(GetPath(payload, "info.0.invoiceDetail.data"))
	for _, itemAny := range items {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		for _, field := range []string{"price", "taxInclusivePrice"} {
			if price, ok := ToDecimal(item[field]); ok {
				item[field] = quantizeUnitPrice(price).String()
			}
		}
		for _, field := range []string{"amount", "taxInclusiveAmount", "tax"} {
			if text, ok := formatCurrencyString(item[field]); ok {
				item[field] = text
			}
		}
		if discount, ok := item["discountInfo"].(map[string]any); ok {
			for _, field := range []string{"discountAmount", "discountTax"} {
				if text, ok := formatCurrencyString(discount[field]); ok {
					discount[field] = text
				}
			}
			if text, ok := formatDecimalString(discount["discount"]); ok {
				discount["discount"] = text
			}
		}
	}
	for _, field := range []string{"amount", "tax", "total"} {
		if text, ok := formatCurrencyString(GetPath(payload, "info.0.invoiceDetail."+field)); ok {
			SetPath(payload, "info.0.invoiceDetail."+field, text)
		}
	}
}

type flatEntry struct {
	Full  string
	Leaf  string
	Value any
}

func buildFlatIndex(raw map[string]any) []flatEntry {
	out := []flatEntry{}
	for _, item := range flattenData(raw, "") {
		if item.path == "" {
			continue
		}
		parts := strings.Split(item.path, ".")
		out = append(out, flatEntry{Full: normalizeKey(item.path), Leaf: normalizeKey(parts[len(parts)-1]), Value: item.value})
	}
	return out
}

type flatPair struct {
	path  string
	value any
}

func flattenData(data any, prefix string) []flatPair {
	out := []flatPair{}
	switch typed := data.(type) {
	case map[string]any:
		for k, v := range typed {
			next := k
			if prefix != "" {
				next = prefix + "." + k
			}
			out = append(out, flattenData(v, next)...)
		}
	case []any:
		for i, v := range typed {
			next := strconv.Itoa(i)
			if prefix != "" {
				next = prefix + "." + next
			}
			out = append(out, flattenData(v, next)...)
		}
	default:
		out = append(out, flatPair{prefix, data})
	}
	return out
}

func collectCandidates(rawData map[string]any, flat []flatEntry, aliases, explicitPaths []string) []any {
	out := []any{}
	for _, path := range explicitPaths {
		if value := GetPath(rawData, path); HasMeaningfulValue(value) {
			out = append(out, value)
		}
	}
	normalizedAliases := map[string]bool{}
	for _, alias := range aliases {
		normalizedAliases[normalizeKey(alias)] = true
	}
	for _, entry := range flat {
		if normalizedAliases[entry.Full] || normalizedAliases[entry.Leaf] {
			out = append(out, entry.Value)
		}
	}
	return out
}

func GetPath(data any, path string) any {
	current := data
	for _, token := range strings.Split(path, ".") {
		switch typed := current.(type) {
		case []any:
			idx, err := strconv.Atoi(token)
			if err != nil || idx < 0 || idx >= len(typed) {
				return nil
			}
			current = typed[idx]
		case map[string]any:
			if _, ok := typed[token]; !ok {
				return nil
			}
			current = typed[token]
		default:
			return nil
		}
	}
	return current
}

func SetPath(data any, path string, value any) {
	tokens := strings.Split(path, ".")
	current := data
	for i, token := range tokens[:len(tokens)-1] {
		nextToken := tokens[i+1]
		switch typed := current.(type) {
		case []any:
			idx, _ := strconv.Atoi(token)
			for len(typed) <= idx {
				typed = append(typed, map[string]any{})
			}
			current = typed[idx]
		case map[string]any:
			if _, ok := typed[token]; !ok {
				if _, err := strconv.Atoi(nextToken); err == nil {
					typed[token] = []any{}
				} else {
					typed[token] = map[string]any{}
				}
			}
			current = typed[token]
		}
	}
	last := tokens[len(tokens)-1]
	switch typed := current.(type) {
	case []any:
		idx, _ := strconv.Atoi(last)
		for len(typed) <= idx {
			typed = append(typed, nil)
		}
		typed[idx] = value
	case map[string]any:
		typed[last] = value
	}
}

func HasMeaningfulValue(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) != ""
	case []any:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	default:
		return true
	}
}

func ToDecimal(value any) (decimal.Decimal, bool) {
	if value == nil {
		return decimal.Zero, false
	}
	if _, ok := value.(bool); ok {
		return decimal.Zero, false
	}
	text := strings.ReplaceAll(strings.TrimSpace(common.StringValue(value)), ",", "")
	if text == "" {
		return decimal.Zero, false
	}
	d, err := decimal.NewFromString(text)
	return d, err == nil
}

func quantizeCurrency(value decimal.Decimal) decimal.Decimal {
	return value.Round(2)
}

func quantizeUnitPrice(value decimal.Decimal) decimal.Decimal {
	return value.Round(13)
}

func decimalToString(value decimal.Decimal) string {
	return value.String()
}

func decimalToNative(value decimal.Decimal) any {
	if value.Equal(value.Truncate(0)) {
		return int(value.IntPart())
	}
	f, _ := value.Float64()
	return f
}

func normalizeTaxRateInput(value any) (string, string) {
	d, ok := parseTaxRateDecimal(value)
	if !ok {
		return "", "税率必须为有效百分比，例如 13%、0.13 或 13"
	}
	if d.LessThan(decimal.Zero) || d.GreaterThan(decimal.NewFromInt(1)) {
		return "", "税率必须在 0 到 100% 之间"
	}
	return formatTaxRatePercentage(d), ""
}

func parseTaxRateDecimal(value any) (decimal.Decimal, bool) {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return decimal.Zero, false
	}
	if strings.Contains(text, "%") {
		d, ok := ToDecimal(strings.ReplaceAll(text, "%", ""))
		if !ok {
			return decimal.Zero, false
		}
		return d.Div(decimal.NewFromInt(100)), true
	}
	d, ok := ToDecimal(value)
	if !ok {
		return decimal.Zero, false
	}
	if d.GreaterThan(decimal.NewFromInt(1)) {
		d = d.Div(decimal.NewFromInt(100))
	}
	return d, true
}

func formatTaxRatePercentage(value decimal.Decimal) string {
	percent := value.Mul(decimal.NewFromInt(100))
	return strings.TrimRight(strings.TrimRight(percent.StringFixed(6), "0"), ".") + "%"
}

func normalizeBool(value any) any {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		if text == "true" || text == "1" || text == "yes" || text == "是" || text == "自然人" || text == "个人" {
			return true
		}
		if text == "false" || text == "0" || text == "no" || text == "否" {
			return false
		}
	}
	return value
}

func loadJSONInput(inputPath string) (map[string]any, error) {
	if strings.TrimSpace(inputPath) == "" {
		return nil, fmt.Errorf("请通过 --input 提供 JSON 文件")
	}
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return nil, err
	}
	data = []byte(strings.TrimPrefix(string(data), "\uFEFF"))
	return decodeJSONObjectPreservingNumbers(data)
}

func decodeJSONObjectPreservingNumbers(data []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	var payload any
	if err := decoder.Decode(&payload); err != nil {
		return nil, err
	}
	obj, ok := payload.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("输入必须是 JSON 对象")
	}
	return obj, nil
}

func addMissingField(missing map[string][]map[string]any, module, field, message string) {
	if missingHasField(missing, module, field) {
		return
	}
	missing[module] = append(missing[module], map[string]any{"field": field, "message": message})
}

func replaceMissingFieldMessage(missing map[string][]map[string]any, module, field, message string) bool {
	for _, entry := range missing[module] {
		if common.StringValue(entry["field"]) == field {
			entry["message"] = message
			return true
		}
	}
	return false
}

func removeMissingField(missing map[string][]map[string]any, module, field string) {
	entries := missing[module]
	out := []map[string]any{}
	for _, entry := range entries {
		if common.StringValue(entry["field"]) != field {
			out = append(out, entry)
		}
	}
	if len(out) == 0 {
		delete(missing, module)
	} else {
		missing[module] = out
	}
}

func removeMissingFieldByPath(missing map[string][]map[string]any, field string) {
	for module := range missing {
		removeMissingField(missing, module, field)
	}
}

func missingHasField(missing map[string][]map[string]any, module, field string) bool {
	for _, entry := range missing[module] {
		if common.StringValue(entry["field"]) == field {
			return true
		}
	}
	return false
}

func cleanMissingFields(missing map[string][]map[string]any) map[string][]map[string]any {
	out := map[string][]map[string]any{}
	for module, entries := range missing {
		if len(entries) > 0 {
			out[module] = entries
		}
	}
	return out
}

func uniqueValues(values []any) []any {
	out := []any{}
	seen := map[string]bool{}
	for _, value := range values {
		key, _ := json.Marshal(value)
		if !seen[string(key)] {
			seen[string(key)] = true
			out = append(out, value)
		}
	}
	return out
}

func normalizeKey(value string) string {
	re := regexp.MustCompile(`[^0-9a-zA-Z\p{Han}]+`)
	return strings.ToLower(re.ReplaceAllString(value, ""))
}

func setLineItemPath(item map[string]any, path string, value any) {
	parts := strings.Split(path, ".")
	if len(parts) == 1 {
		item[path] = value
		return
	}
	current := item
	for _, part := range parts[:len(parts)-1] {
		child, _ := current[part].(map[string]any)
		if child == nil {
			child = map[string]any{}
			current[part] = child
		}
		current = child
	}
	current[parts[len(parts)-1]] = value
}

func formatCurrencyString(value any) (string, bool) {
	d, ok := ToDecimal(value)
	if !ok {
		return "", false
	}
	return quantizeCurrency(d).StringFixed(2), true
}

func formatDecimalString(value any) (string, bool) {
	d, ok := ToDecimal(value)
	if !ok {
		return "", false
	}
	return strings.TrimRight(strings.TrimRight(d.String(), "0"), "."), true
}

func pruneEmptyValues(data any) any {
	switch typed := data.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range typed {
			pruned := pruneEmptyValues(v)
			if isEmptyPruned(pruned) {
				continue
			}
			out[k] = pruned
		}
		return out
	case []any:
		out := []any{}
		for _, v := range typed {
			pruned := pruneEmptyValues(v)
			if !isEmptyPruned(pruned) {
				out = append(out, pruned)
			}
		}
		return out
	default:
		return data
	}
}

func isEmptyPruned(value any) bool {
	if value == nil {
		return true
	}
	switch typed := value.(type) {
	case string:
		return typed == ""
	case []any:
		return len(typed) == 0
	case map[string]any:
		return len(typed) == 0
	default:
		return false
	}
}

func asList(value any) ([]any, bool) {
	switch typed := value.(type) {
	case []any:
		return typed, true
	default:
		return nil, false
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func isTruthy(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		return text == "true" || text == "1" || text == "是" || text == "自然人" || text == "个人"
	default:
		return HasMeaningfulValue(value)
	}
}

func responseCode(resp map[string]any) int {
	switch v := resp["code"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	default:
		return math.MinInt
	}
}

func newUUIDLike() string {
	h := strconv.FormatInt(time.Now().UnixNano(), 16)
	return h + "-0000-4000-8000-" + shortHash()
}

func assetPath(name string) string {
	candidates := []string{
		filepath.Join(common.SkillInstallDir(), "assets", name),
		filepath.Join(common.SkillInstallDir(), "assets", "issue", name),
		filepath.Join(common.SkillInstallDir(), "internal", "assets", "issue", name),
		filepath.Join("internal", "assets", "issue", name),
	}
	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return candidates[0]
}

func strictNormalizeTaxRateInput(value any) (string, string) {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "免税" || text == "不征税" {
		return text, ""
	}
	if text == "0税率" {
		return "0%", ""
	}
	d, ok := strictParseTaxRateDecimal(value)
	if !ok {
		return "", buildTaxRateInputError(value)
	}
	return formatTaxRatePercentage(d), ""
}

func strictParseTaxRateDecimal(value any) (decimal.Decimal, bool) {
	if value == nil {
		return decimal.Zero, false
	}
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return decimal.Zero, false
	}
	if strings.HasSuffix(text, "%") {
		d, ok := ToDecimal(strings.TrimSpace(strings.TrimSuffix(text, "%")))
		if !ok || d.LessThan(decimal.Zero) {
			return decimal.Zero, false
		}
		return d.Div(decimal.NewFromInt(100)), true
	}
	if text == "0" {
		return decimal.Zero, true
	}
	d, ok := ToDecimal(text)
	if !ok || !strings.Contains(text, ".") || d.LessThan(decimal.Zero) || d.GreaterThan(decimal.NewFromInt(1)) {
		return decimal.Zero, false
	}
	return d, true
}

func buildTaxRateInputError(value any) string {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		text = "空值"
	}
	return fmt.Sprintf("税率输入不合理：%s。仅支持 `1%%` 或 `0.01` 这两类格式；零税率可输入 `0` 或 `0%%`，请按示例重新输入。", text)
}

func applyTaxRateSpecialPolicy(item map[string]any) {
	text := strings.TrimSpace(common.StringValue(item["taxRate"]))
	item["xsyhzcbz"] = "N"
	switch text {
	case "免税":
		item["taxRate"] = "0%"
		item["xsyhzcbz"] = "Y"
		item["zzstsgl"] = "03"
	case "不征税":
		item["taxRate"] = "0%"
		item["xsyhzcbz"] = "Y"
		item["zzstsgl"] = "04"
	default:
		if strings.ToUpper(strings.TrimSpace(common.StringValue(item["xsyhzcbz"]))) == "Y" {
			policy := strings.TrimSpace(common.StringValue(item["zzstsgl"]))
			if policy == "03" || policy == "04" {
				return
			}
		}
		item["zzstsgl"] = ""
	}
}

func extractTaxpayerTypeOverride(rawData map[string]any) string {
	for _, key := range []string{"taxpayerType", "taxpayer_type", "纳税人类型"} {
		if value := strings.TrimSpace(common.StringValue(rawData[key])); value != "" {
			return value
		}
	}
	return ""
}

func normalizeTaxpayerTypeInput(value any) string {
	text := strings.TrimSpace(common.StringValue(value))
	switch text {
	case "1", "一般纳税人", "一般纳税人企业", "一般":
		return "一般纳税人"
	case "2", "小规模纳税人", "小规模", "小规模企业":
		return "小规模纳税人"
	default:
		return ""
	}
}

type taxRateAutofillContext struct {
	lineItems []any
	missing   map[string][]map[string]any
	identity  taxpayerIdentity
}

func autofillLineItemTaxRates(input taxRateAutofillContext) (map[string]any, error) {
	enabled := input.identity.isKnown()
	taxpayerType := input.identity.taxpayerType()
	result := map[string]any{"validation_enabled": enabled, "taxpayer_type": taxpayerType, "skipped_reason": "", "confirmations": []any{}}
	if !enabled {
		result["skipped_reason"] = input.identity.warningMessage()
	}
	confirmations := []any{}
	for idx, itemAny := range input.lineItems {
		item, ok := itemAny.(map[string]any)
		if !ok || HasMeaningfulValue(item["taxRate"]) || !HasMeaningfulValue(item["spbm"]) {
			continue
		}
		fieldPath := fmt.Sprintf("invoiceDetail.data[%d].taxRate", idx)
		referenceRates := []decimal.Decimal{}
		if enabled {
			var err error
			referenceRates, err = getReferenceTaxRatesChecked(item["spbm"], taxpayerType)
			if err != nil {
				return nil, err
			}
		}
		if len(referenceRates) > 0 {
			minRate := referenceRates[0]
			for _, rate := range referenceRates[1:] {
				if rate.LessThan(minRate) {
					minRate = rate
				}
			}
			text := formatTaxRatePercentage(minRate)
			item["taxRate"] = text
			removeMissingField(input.missing, "发票明细", fieldPath)
			confirmations = append(confirmations, map[string]any{"index": idx + 1, "spbm": common.StringValue(item["spbm"]), "project_name": common.StringValue(item["projectName"]), "input_mode": "missing", "autofilled_tax_rate": text, "message": fmt.Sprintf("明细%d未提供税率，已自动匹配为 %s，请确认是否按该税率开票", idx+1, text)})
			continue
		}
		if enabled {
			addMissingField(input.missing, "发票明细", fieldPath, "未提供税率，且当前商品编码未匹配到该纳税人类型的参考税率；请直接提供税率，或确认纳税人类型后重新匹配")
		} else {
			addMissingField(input.missing, "发票明细", fieldPath, "未提供税率，且纳税人身份缺失，无法自动匹配最低税率；请直接提供税率，或补充合法 taxpayerType 人工确认后重试")
		}
	}
	result["confirmations"] = confirmations
	return result, nil
}

func validateTaxRateReferences(payload map[string]any, identity taxpayerIdentity, autofillResult map[string]any) (map[string]any, error) {
	taxpayerType := identity.taxpayerType()
	if !identity.isKnown() {
		warning := map[string]any{"code": identity.warningCode(), "message": identity.warningMessage()}
		return map[string]any{
			"validation_enabled": false,
			"taxpayer_type":      taxpayerType,
			"confirmations":      []any{},
			"warnings":           []any{warning},
			"warning_markdown":   "**纳税人身份提示**\n- " + identity.warningMessage(),
			"skipped_reason":     identity.warningMessage(),
		}, nil
	}
	confirmations, _ := asList(autofillResult["confirmations"])
	warnings := []any{}
	items, _ := asList(GetPath(payload, "info.0.invoiceDetail.data"))
	for idx, itemAny := range items {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		currentRate, ok := strictParseTaxRateDecimal(item["taxRate"])
		if !ok {
			currentRate, ok = parseTaxRateDecimal(item["taxRate"])
		}
		if !ok {
			continue
		}
		referenceRates, err := getReferenceTaxRatesChecked(item["spbm"], taxpayerType)
		if err != nil {
			return nil, err
		}
		if len(referenceRates) == 0 {
			continue
		}
		matched := false
		refTexts := []string{}
		for _, rate := range referenceRates {
			refTexts = append(refTexts, formatTaxRatePercentage(rate))
			if rate.Equal(currentRate) {
				matched = true
			}
		}
		if !matched {
			currentText := getTaxRateDisplayText(item)
			msg := fmt.Sprintf("明细%d当前税率为 %s，参考税率为 %s", idx+1, currentText, strings.Join(refTexts, "、"))
			warnings = append(warnings, map[string]any{"index": idx + 1, "spbm": common.StringValue(item["spbm"]), "project_name": common.StringValue(item["projectName"]), "current_tax_rate": currentText, "reference_tax_rates": refTexts, "message": msg})
		}
	}
	result := map[string]any{"validation_enabled": true, "taxpayer_type": taxpayerType, "confirmations": confirmations, "warnings": warnings, "warning_markdown": "", "skipped_reason": ""}
	result["warning_markdown"] = buildTaxRateValidationMarkdown(result)
	return result, nil
}

func buildTaxRateValidationMarkdown(result map[string]any) string {
	confirmations, _ := asList(result["confirmations"])
	warnings, _ := asList(result["warnings"])
	if len(confirmations) == 0 && len(warnings) == 0 {
		return ""
	}
	lines := []string{"**税率参考提示**"}
	if taxpayerType := strings.TrimSpace(common.StringValue(result["taxpayer_type"])); taxpayerType != "" {
		lines = append(lines, "- 纳税人类型："+taxpayerType)
	}
	for _, entryAny := range confirmations {
		entry, _ := entryAny.(map[string]any)
		if msg := strings.TrimSpace(common.StringValue(entry["message"])); msg != "" {
			lines = append(lines, "- "+msg)
		}
	}
	for _, entryAny := range warnings {
		entry, _ := entryAny.(map[string]any)
		if msg := strings.TrimSpace(common.StringValue(entry["message"])); msg != "" {
			lines = append(lines, "- "+msg)
		}
	}
	if len(confirmations) > 0 {
		lines = append(lines, "- 以上税率为系统自动匹配结果，请先核对；如需修改，请直接说明税率。")
	}
	if len(warnings) > 0 {
		lines = append(lines, "- 以上仅为参考提示，不强制拦截开票，请核对后继续。")
	}
	return strings.Join(lines, "\n")
}

func getTaxRateDisplayText(item map[string]any) string {
	if strings.ToUpper(strings.TrimSpace(common.StringValue(item["xsyhzcbz"]))) == "Y" {
		switch strings.TrimSpace(common.StringValue(item["zzstsgl"])) {
		case "03":
			return "免税"
		case "04":
			return "不征税"
		}
	}
	if rate, ok := strictParseTaxRateDecimal(item["taxRate"]); ok {
		return formatTaxRatePercentage(rate)
	}
	return strings.TrimSpace(common.StringValue(item["taxRate"]))
}

func splitTaxRateCandidates(value any) []decimal.Decimal {
	text := strings.TrimSpace(common.StringValue(value))
	if text == "" {
		return nil
	}
	out := []decimal.Decimal{}
	for _, part := range regexp.MustCompile(`[、,，;\s]+`).Split(text, -1) {
		if rate, ok := strictParseTaxRateDecimal(part); ok {
			exists := false
			for _, existing := range out {
				if existing.Equal(rate) {
					exists = true
					break
				}
			}
			if !exists {
				out = append(out, rate)
			}
		}
	}
	return out
}

func getReferenceTaxRates(spbm any, taxpayerType string) []decimal.Decimal {
	rates, _ := getReferenceTaxRatesChecked(spbm, taxpayerType)
	return rates
}

/**
 * getReferenceTaxRatesChecked 优先读取新版赋码缓存，再回退查询手工商品编码的详情税率。
 *
 * @param spbm 税收分类编码。
 * @param taxpayerType 已解析的纳税人类型。
 * @returns 对应纳税人类型的参考税率；没有可用税率时返回空列表。
 * @throws skillupdate.OutdatedError 当回退查询时发现当前 Skill 版本过低则返回。
 */
func getReferenceTaxRatesChecked(spbm any, taxpayerType string) ([]decimal.Decimal, error) {
	code := strings.TrimSpace(common.StringValue(spbm))
	if code == "" {
		return nil, nil
	}
	if rates := getCachedTaxCodingRecommendationRates(code, taxpayerType); len(rates) > 0 {
		return splitTaxRateCandidates(strings.Join(rates, ",")), nil
	}
	detail, err := getProductClassifyCodeDetailChecked(code)
	if err != nil {
		return nil, err
	}
	if strings.Contains(common.StringValue(taxpayerType), "小") || strings.Contains(common.StringValue(taxpayerType), "灏") {
		return splitTaxRateCandidates(detail["SllbSmallScale"]), nil
	}
	return splitTaxRateCandidates(detail["Sllb"]), nil
}

func loadJSONInputFlexible(inputPath string) (map[string]any, error) {
	if strings.TrimSpace(inputPath) != "" {
		return loadJSONInput(inputPath)
	}
	stat, _ := os.Stdin.Stat()
	if stat == nil || (stat.Mode()&os.ModeCharDevice) != 0 {
		return nil, fmt.Errorf("请通过 --input 提供 JSON 文件，或通过 stdin 传入 JSON 内容")
	}
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		return nil, err
	}
	data = []byte(strings.TrimPrefix(string(data), "\uFEFF"))
	return decodeJSONObjectPreservingNumbers(data)
}

func normalizeDiscountInfoEquivalent(item map[string]any) string {
	discount, ok := item["discountInfo"].(map[string]any)
	if !ok {
		return ""
	}
	rawDiscountAmount, amountOK := ToDecimal(discount["discountAmount"])
	rawDiscountRatio, ratioOK := ToDecimal(discount["discount"])
	taxInclusiveAmount, ok := ToDecimal(item["taxInclusiveAmount"])
	if !ok || taxInclusiveAmount.IsZero() {
		amount, _ := ToDecimal(item["amount"])
		tax, _ := ToDecimal(item["tax"])
		taxInclusiveAmount = quantizeCurrency(amount.Add(tax))
	}
	if !amountOK && !ratioOK {
		delete(item, "discountInfo")
		return ""
	}
	if ratioOK && (rawDiscountRatio.LessThan(decimal.Zero) || !rawDiscountRatio.LessThan(decimal.NewFromInt(100))) {
		return "折扣比例必须大于等于 0 且小于 100"
	}
	discountAmount := rawDiscountAmount
	if !amountOK && ratioOK {
		discountAmount = quantizeCurrency(taxInclusiveAmount.Mul(rawDiscountRatio).Div(decimal.NewFromInt(100)))
		amountOK = true
	}
	if !amountOK {
		return "折扣信息无法识别，请提供有效的折扣金额或折扣比例"
	}
	discountAmount = quantizeCurrency(discountAmount.Abs())
	if discountAmount.IsZero() {
		return "折扣金额必须大于 0"
	}
	if !taxInclusiveAmount.GreaterThan(decimal.Zero) {
		return "当前明细金额必须大于 0 后才能计算折扣"
	}
	if !discountAmount.LessThan(taxInclusiveAmount) {
		return "折扣金额必须小于当前明细含税金额"
	}
	taxRate, ok := strictParseTaxRateDecimal(item["taxRate"])
	if !ok {
		taxRate, _ = parseTaxRateDecimal(item["taxRate"])
	}
	discountWithoutTax := discountAmount
	discountTax := decimal.Zero
	if taxRate.GreaterThan(decimal.Zero) {
		discountWithoutTax = quantizeCurrency(discountAmount.Div(decimal.NewFromInt(1).Add(taxRate)))
		discountTax = quantizeCurrency(discountAmount.Sub(discountWithoutTax))
	}
	normalized := map[string]any{
		"discountMode":   1,
		"discountAmount": discountWithoutTax.StringFixed(2),
		"discountTax":    discountTax.Abs().StringFixed(2),
	}
	if ratioOK {
		normalized["discount"] = decimalToNative(rawDiscountRatio)
	}
	item["discountInfo"] = normalized
	return ""
}
