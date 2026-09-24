package issue

import "time"

const (
	taxCodingRecommendationPath              = "/intelligent-tax-coding/recommend"
	taxCodingRecommendationSuccessCode       = 200
	taxCodingRecommendationRateLimitedCode   = 429
	taxCodingRecommendationBadGatewayCode    = 502
	taxCodingRecommendationUnavailableCode   = 503
	taxCodingRecommendationTimeoutCode       = 504
	taxCodingRecommendationSuccessStatus     = "RESULT_STATUS_SUCCESS"
	taxCodingRecommendationTopRank           = 1
	taxCodingRecommendationRequestIDPrefix   = "invoice-issue-tax-coding-"
	taxCodingRecommendationTimeout           = 30 * time.Second
	taxCodingRecommendationGeneralTaxpayer   = "一般纳税人"
	taxCodingRecommendationSmallTaxpayerHint = "小"
	taxCodingRecommendationFailurePrefix     = "自动赋码失败："
	taxCodingRecommendationMissingPrefix     = "请提供商品编码；"
	taxCodingRecommendationNoMatchPrefix     = "自动赋码未匹配到商品编码："
	taxCodingRecommendationRequestError      = "请求构造异常，"
	taxCodingRecommendationTransportError    = "接口请求异常，"
	taxCodingRecommendationResponseError     = "响应解析异常，"
	taxCodingRecommendationUnknownError      = "服务返回未知错误"
)
