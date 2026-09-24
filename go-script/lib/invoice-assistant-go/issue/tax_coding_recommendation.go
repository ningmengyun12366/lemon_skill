package issue

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
	"invoice-assistant-skill/go-script/lib/skillupdate"
)

type taxCodingRecommendationRequestItem struct {
	ItemID        string `json:"itemId"`
	ProductName   string `json:"productName"`
	Specification string `json:"specification"`
	Unit          string `json:"unit"`
}

type taxCodingRecommendationRequest struct {
	RequestID string                               `json:"requestId"`
	TaxNo     string                               `json:"taxNo,omitempty"`
	Items     []taxCodingRecommendationRequestItem `json:"items"`
}

type taxCodingRecommendationAPIResponse struct {
	Code    int                                 `json:"code"`
	Message string                              `json:"message"`
	Data    taxCodingRecommendationResponseData `json:"data"`
}

type taxCodingRecommendationResponseData struct {
	RequestID string                              `json:"requestId"`
	TraceID   string                              `json:"traceId"`
	Items     []taxCodingRecommendationItemResult `json:"items"`
}

type taxCodingRecommendationItemResult struct {
	ItemID          string                    `json:"itemId"`
	ProductName     string                    `json:"productName"`
	Status          string                    `json:"status"`
	Recommendations []taxCodingRecommendation `json:"recommendations"`
	ErrorCode       string                    `json:"errorCode"`
	ErrorMessage    string                    `json:"errorMessage"`
}

type taxCodingRecommendation struct {
	Rank                           int      `json:"rank"`
	TaxCode                        string   `json:"taxCode"`
	ShortName                      string   `json:"shortName"`
	GeneralTaxpayerRates           []string `json:"generalTaxpayerRates"`
	GeneralTaxpayerRecommendedRate string   `json:"generalTaxpayerRecommendedRate"`
	SmallTaxpayerRates             []string `json:"smallTaxpayerRates"`
	SmallTaxpayerRecommendedRate   string   `json:"smallTaxpayerRecommendedRate"`
}

type taxCodingPendingItem struct {
	Index       int
	RequestItem taxCodingRecommendationRequestItem
}

type taxCodingRecommendationOutcome struct {
	Recommendation taxCodingRecommendation
	FailureReason  string
	Matched        bool
}

type cachedTaxCodingRecommendation struct {
	ProductName          string
	ShortName            string
	GeneralTaxpayerRates []string
	SmallTaxpayerRates   []string
}

var taxCodingRecommendationCache map[string]cachedTaxCodingRecommendation

/**
 * autofillMissingProductCodes 调用新版智能赋码接口，为缺少 spbm 的明细补全商品编码。
 *
 * @param items 已标准化的发票明细。
 * @param missing 当前缺失字段集合，成功赋码后会移除对应 spbm 项。
 * @param taxNo 销售方税号；为空时省略，由下游使用全局历史。
 * @returns 必须阻断流程的版本错误；普通赋码失败会写入 missing 并返回 nil。
 * @throws skillupdate.OutdatedError 当当前 Skill 版本过低时返回。
 */
func autofillMissingProductCodes(items []any, missing map[string][]map[string]any, taxNo string) error {
	pending := buildTaxCodingPendingItems(items)
	if len(pending) == 0 {
		return nil
	}

	outcomes, requestFailureReason, err := fetchTaxCodingRecommendations(taxNo, pending)
	if err != nil {
		return err
	}
	for _, pendingItem := range pending {
		item, _ := items[pendingItem.Index].(map[string]any)
		field := fmt.Sprintf("invoiceDetail.data[%d].spbm", pendingItem.Index)
		outcome := outcomes[pendingItem.RequestItem.ItemID]
		if outcome.Matched {
			item["spbm"] = outcome.Recommendation.TaxCode
			cacheTaxCodingRecommendation(pendingItem.RequestItem.ProductName, outcome.Recommendation)
			removeMissingField(missing, "发票明细", field)
			continue
		}

		failureReason := outcome.FailureReason
		if failureReason == "" {
			failureReason = requestFailureReason
		}
		if failureReason == "" {
			failureReason = taxCodingRecommendationNoMatchPrefix + pendingItem.RequestItem.ProductName
		}
		message := taxCodingRecommendationMissingPrefix + failureReason
		if !replaceMissingFieldMessage(missing, "发票明细", field, message) {
			addMissingField(missing, "发票明细", field, message)
		}
	}
	return nil
}

/**
 * buildTaxCodingPendingItems 将待赋码明细转换为新版接口所需的强类型商品列表。
 *
 * @param items 已标准化的发票明细。
 * @returns 仅包含缺少 spbm 且商品名称非空的待赋码项，itemId 使用原明细下标。
 */
func buildTaxCodingPendingItems(items []any) []taxCodingPendingItem {
	pending := make([]taxCodingPendingItem, 0, len(items))
	for index, itemAny := range items {
		item, ok := itemAny.(map[string]any)
		if !ok || HasMeaningfulValue(item["spbm"]) || !HasMeaningfulValue(item["projectName"]) {
			continue
		}
		pending = append(pending, taxCodingPendingItem{
			Index: index,
			RequestItem: taxCodingRecommendationRequestItem{
				ItemID:        strconv.Itoa(index),
				ProductName:   strings.TrimSpace(common.StringValue(item["projectName"])),
				Specification: strings.TrimSpace(common.StringValue(item["size"])),
				Unit:          strings.TrimSpace(common.StringValue(item["unit"])),
			},
		})
	}
	return pending
}

/**
 * fetchTaxCodingRecommendations 请求新版智能赋码接口并按 itemId 整理商品级结果。
 *
 * @param taxNo 销售方税号；为空时不发送 taxNo 字段。
 * @param pending 待赋码商品及其原明细位置。
 * @returns itemId 对应的推荐结果、顶层失败原因以及必须阻断流程的错误。
 * @throws skillupdate.OutdatedError 当当前 Skill 版本过低时返回。
 */
func fetchTaxCodingRecommendations(taxNo string, pending []taxCodingPendingItem) (map[string]taxCodingRecommendationOutcome, string, error) {
	request := taxCodingRecommendationRequest{
		RequestID: taxCodingRecommendationRequestIDPrefix + newUUIDLike(),
		TaxNo:     strings.TrimSpace(taxNo),
		Items:     make([]taxCodingRecommendationRequestItem, 0, len(pending)),
	}
	for _, pendingItem := range pending {
		request.Items = append(request.Items, pendingItem.RequestItem)
	}
	payload, err := taxCodingRecommendationRequestPayload(request)
	if err != nil {
		return map[string]taxCodingRecommendationOutcome{}, taxCodingRecommendationFailurePrefix + taxCodingRecommendationRequestError + err.Error(), nil
	}
	resp, err := postTaxCodingRecommendationWithRetry(payload)
	if err != nil {
		if outdated, ok := skillupdate.FindOutdatedError(err); ok {
			return nil, "", outdated
		}
		return map[string]taxCodingRecommendationOutcome{}, taxCodingRecommendationFailurePrefix + taxCodingRecommendationTransportError + err.Error(), nil
	}
	typedResponse, err := decodeTaxCodingRecommendationResponse(resp)
	if err != nil {
		return map[string]taxCodingRecommendationOutcome{}, taxCodingRecommendationFailurePrefix + taxCodingRecommendationResponseError + err.Error(), nil
	}
	if typedResponse.Code != taxCodingRecommendationSuccessCode {
		message := strings.TrimSpace(typedResponse.Message)
		if message == "" {
			message = taxCodingRecommendationUnknownError
		}
		return map[string]taxCodingRecommendationOutcome{}, taxCodingRecommendationFailurePrefix + message, nil
	}

	outcomes := make(map[string]taxCodingRecommendationOutcome, len(typedResponse.Data.Items))
	for _, result := range typedResponse.Data.Items {
		itemID := strings.TrimSpace(result.ItemID)
		if itemID == "" {
			continue
		}
		recommendation, matched := selectTopTaxCodingRecommendation(result)
		failureReason := ""
		if !matched {
			failureReason = taxCodingRecommendationFailureReason(result)
		}
		outcomes[itemID] = taxCodingRecommendationOutcome{
			Recommendation: recommendation,
			FailureReason:  failureReason,
			Matched:        matched,
		}
	}
	return outcomes, "", nil
}

/**
 * taxCodingRecommendationRequestPayload 将强类型请求转换为共享 HTTP Client 接受的 JSON 对象。
 *
 * @param request 新版智能赋码请求。
 * @returns 保留 optional presence 的 JSON 对象；taxNo 为空时不会出现该字段。
 * @throws error 当请求无法完成 JSON 编解码时返回。
 */
func taxCodingRecommendationRequestPayload(request taxCodingRecommendationRequest) (map[string]any, error) {
	raw, err := json.Marshal(request)
	if err != nil {
		return nil, err
	}
	payload := map[string]any{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return payload, nil
}

/**
 * postTaxCodingRecommendationWithRetry 按开票 Skill 的既有请求策略调用新版赋码接口。
 *
 * @param payload 新版智能赋码 HTTP 请求体；重试时复用同一个 requestId。
 * @returns 首个成功响应，或最后一个业务失败响应。
 * @throws error 当所有尝试均发生传输错误或版本检查失败时返回。
 */
func postTaxCodingRecommendationWithRetry(payload map[string]any) (map[string]any, error) {
	var lastResponse map[string]any
	var lastError error
	for attempt := 0; attempt < issueAPIMaxAttempts; attempt++ {
		// requestId 在所有尝试中保持不变，让下游可以识别同一次逻辑请求。
		resp, err := newAPIClient().PostJSON(taxCodingRecommendationPath, payload, taxCodingRecommendationTimeout)
		if err != nil {
			if outdated, ok := skillupdate.FindOutdatedError(err); ok {
				return nil, outdated
			}
			lastResponse = nil
			lastError = err
			continue
		}
		if responseCode(resp) == taxCodingRecommendationSuccessCode {
			return resp, nil
		}
		lastResponse = resp
		lastError = nil
		if !isTaxCodingRecommendationRetryableCode(responseCode(resp)) {
			return lastResponse, nil
		}
	}
	if lastResponse != nil {
		return lastResponse, nil
	}
	return nil, lastError
}

/**
 * isTaxCodingRecommendationRetryableCode 判断顶层响应码是否表示临时故障。
 *
 * @param code 新版智能赋码 HTTP 响应体中的 code。
 * @returns 限流、网关错误、服务不可用或超时时返回 true。
 */
func isTaxCodingRecommendationRetryableCode(code int) bool {
	switch code {
	case taxCodingRecommendationRateLimitedCode,
		taxCodingRecommendationBadGatewayCode,
		taxCodingRecommendationUnavailableCode,
		taxCodingRecommendationTimeoutCode:
		return true
	default:
		return false
	}
}

/**
 * decodeTaxCodingRecommendationResponse 将通用 JSON 对象收窄为新版赋码响应。
 *
 * @param response 通用 HTTP Client 解码后的响应对象。
 * @returns 强类型智能赋码响应。
 * @throws error 当响应无法完成 JSON 编解码时返回。
 */
func decodeTaxCodingRecommendationResponse(response map[string]any) (taxCodingRecommendationAPIResponse, error) {
	raw, err := json.Marshal(response)
	if err != nil {
		return taxCodingRecommendationAPIResponse{}, err
	}
	var typedResponse taxCodingRecommendationAPIResponse
	if err := json.Unmarshal(raw, &typedResponse); err != nil {
		return taxCodingRecommendationAPIResponse{}, err
	}
	return typedResponse, nil
}

/**
 * selectTopTaxCodingRecommendation 选择成功商品中 rank=1 的推荐，缺失时使用首个有效编码。
 *
 * @param result 单个商品的智能赋码结果。
 * @returns 选中的推荐以及是否匹配成功。
 */
func selectTopTaxCodingRecommendation(result taxCodingRecommendationItemResult) (taxCodingRecommendation, bool) {
	if strings.TrimSpace(result.Status) != taxCodingRecommendationSuccessStatus {
		return taxCodingRecommendation{}, false
	}
	var firstValid taxCodingRecommendation
	found := false
	for _, recommendation := range result.Recommendations {
		if strings.TrimSpace(recommendation.TaxCode) == "" {
			continue
		}
		if !found {
			firstValid = recommendation
			found = true
		}
		if recommendation.Rank == taxCodingRecommendationTopRank {
			return recommendation, true
		}
	}
	return firstValid, found
}

/**
 * taxCodingRecommendationFailureReason 生成商品级赋码失败的稳定提示。
 *
 * @param result 单个商品的智能赋码结果。
 * @returns 优先包含下游错误消息和错误码的失败原因。
 */
func taxCodingRecommendationFailureReason(result taxCodingRecommendationItemResult) string {
	message := strings.TrimSpace(result.ErrorMessage)
	code := strings.TrimSpace(result.ErrorCode)
	if message != "" && code != "" {
		return fmt.Sprintf("%s%s（%s）", taxCodingRecommendationFailurePrefix, message, code)
	}
	if message != "" {
		return taxCodingRecommendationFailurePrefix + message
	}
	if code != "" {
		return taxCodingRecommendationFailurePrefix + code
	}
	return ""
}

/**
 * cacheTaxCodingRecommendation 缓存自动推荐的原商品名、税收分类简称和税率。
 *
 * @param productName 请求中使用的原商品名称。
 * @param recommendation 已选中的新版智能赋码推荐。
 * @returns 无返回值。
 */
func cacheTaxCodingRecommendation(productName string, recommendation taxCodingRecommendation) {
	code := strings.TrimSpace(recommendation.TaxCode)
	if code == "" {
		return
	}
	if taxCodingRecommendationCache == nil {
		taxCodingRecommendationCache = map[string]cachedTaxCodingRecommendation{}
	}
	taxCodingRecommendationCache[code] = cachedTaxCodingRecommendation{
		ProductName:          strings.TrimSpace(productName),
		ShortName:            strings.TrimSpace(recommendation.ShortName),
		GeneralTaxpayerRates: normalizedTaxCodingRates(recommendation.GeneralTaxpayerRates, recommendation.GeneralTaxpayerRecommendedRate),
		SmallTaxpayerRates:   normalizedTaxCodingRates(recommendation.SmallTaxpayerRates, recommendation.SmallTaxpayerRecommendedRate),
	}
}

/**
 * normalizedTaxCodingRates 清洗税率列表，并把下游推荐税率放在首位且去重。
 *
 * @param rates 下游返回的可用税率列表。
 * @param recommendedRate 下游返回的推荐税率。
 * @returns 去除空值和重复值后的税率列表。
 */
func normalizedTaxCodingRates(rates []string, recommendedRate string) []string {
	values := append([]string{recommendedRate}, rates...)
	result := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	return result
}

/**
 * getCachedTaxCodingRecommendation 读取指定商品编码的新版推荐缓存。
 *
 * @param taxCode 税收分类编码。
 * @returns 缓存结果以及是否存在。
 */
func getCachedTaxCodingRecommendation(taxCode string) (cachedTaxCodingRecommendation, bool) {
	if taxCodingRecommendationCache == nil {
		return cachedTaxCodingRecommendation{}, false
	}
	recommendation, ok := taxCodingRecommendationCache[strings.TrimSpace(taxCode)]
	return recommendation, ok
}

/**
 * getCachedTaxCodingRecommendationRates 按纳税人类型读取新版推荐税率。
 *
 * @param taxCode 税收分类编码。
 * @param taxpayerType 已解析的纳税人类型。
 * @returns 对应纳税人类型的税率列表；缓存不存在或未返回税率时为空。
 */
func getCachedTaxCodingRecommendationRates(taxCode, taxpayerType string) []string {
	recommendation, ok := getCachedTaxCodingRecommendation(taxCode)
	if !ok {
		return nil
	}
	if strings.Contains(taxpayerType, taxCodingRecommendationSmallTaxpayerHint) {
		return append([]string(nil), recommendation.SmallTaxpayerRates...)
	}
	if strings.TrimSpace(taxpayerType) == taxCodingRecommendationGeneralTaxpayer {
		return append([]string(nil), recommendation.GeneralTaxpayerRates...)
	}
	return nil
}
