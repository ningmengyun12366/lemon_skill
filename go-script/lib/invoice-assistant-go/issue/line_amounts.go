package issue

import (
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

const (
	lineAmountTaxRelationMismatchCode    = "LINE_AMOUNT_TAX_RELATION_MISMATCH"
	lineUnitPriceTaxRelationMismatchCode = "LINE_UNIT_PRICE_TAX_RELATION_MISMATCH"
	lineTaxInclusiveTotalMismatchCode    = "LINE_TAX_INCLUSIVE_TOTAL_MISMATCH"
	lineTaxExclusiveTotalMismatchCode    = "LINE_TAX_EXCLUSIVE_TOTAL_MISMATCH"
	lineAmountBasisMissingCode           = "LINE_AMOUNT_BASIS_MISSING"
	linePriceQuantityPairMismatchCode    = "LINE_PRICE_QUANTITY_PAIR_MISMATCH"
)

var lineAmountTolerance = decimal.RequireFromString("0.01")

type lineAmountPresence struct {
	amount             bool
	taxInclusiveAmount bool
	price              bool
	taxInclusivePrice  bool
	quantity           bool
	tax                bool
}

func captureLineAmountPresence(item map[string]any) lineAmountPresence {
	return lineAmountPresence{
		amount:             HasMeaningfulValue(item["amount"]),
		taxInclusiveAmount: HasMeaningfulValue(item["taxInclusiveAmount"]),
		price:              HasMeaningfulValue(item["price"]),
		taxInclusivePrice:  HasMeaningfulValue(item["taxInclusivePrice"]),
		quantity:           HasMeaningfulValue(item["quantity"]),
		tax:                HasMeaningfulValue(item["tax"]),
	}
}

/**
 * normalizeLineItemAmounts applies the complete amount/unit-price matrix and returns all arithmetic conflicts.
 *
 * @param lineItems normalized invoice detail objects before amount autofill.
 * @param missing current missing-field collection, updated when amount and tax are derived.
 * @param invoiceIndex one-based invoice index used in agent-facing errors.
 * @returns detailed validation errors without stopping after the first conflict.
 */
func normalizeLineItemAmounts(lineItems []any, missing map[string][]map[string]any, invoiceIndex int) []string {
	if invoiceIndex < 1 {
		invoiceIndex = 1
	}
	errors := []string{}
	for itemIndex, itemAny := range lineItems {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		presence := captureLineAmountPresence(item)
		itemErrors := normalizeLineItemAmount(item, presence, invoiceIndex, itemIndex+1)
		errors = append(errors, itemErrors...)
		amountPath := fmt.Sprintf("invoiceDetail.data[%d].amount", itemIndex)
		taxPath := fmt.Sprintf("invoiceDetail.data[%d].tax", itemIndex)
		if HasMeaningfulValue(item["amount"]) {
			removeMissingField(missing, "发票明细", amountPath)
		}
		if HasMeaningfulValue(item["tax"]) {
			removeMissingField(missing, "发票明细", taxPath)
		}
		if !presence.amount && !presence.taxInclusiveAmount && !presence.price && !presence.taxInclusivePrice {
			removeMissingField(missing, "发票明细", taxPath)
			replaceMissingFieldMessage(missing, "发票明细", amountPath, lineAmountBasisMissingError(invoiceIndex, itemIndex+1))
		}
	}
	return errors
}

func normalizeLineItemAmount(item map[string]any, presence lineAmountPresence, invoiceIndex, itemIndex int) []string {
	a, aOK := ToDecimal(item["amount"])
	at, atOK := ToDecimal(item["taxInclusiveAmount"])
	p, pOK := ToDecimal(item["price"])
	pt, ptOK := ToDecimal(item["taxInclusivePrice"])
	r, rOK := parseTaxRateDecimal(item["taxRate"])
	q, qOK := ToDecimal(item["quantity"])

	if pOK {
		p = quantizeUnitPrice(p)
		item["price"] = decimalToString(p)
	}
	if ptOK {
		pt = quantizeUnitPrice(pt)
		item["taxInclusivePrice"] = decimalToString(pt)
	}

	onePlusRate := decimal.NewFromInt(1).Add(r)
	if rOK && pOK && !ptOK {
		pt = quantizeUnitPrice(p.Mul(onePlusRate))
		ptOK = true
		item["taxInclusivePrice"] = decimalToString(pt)
	}
	if rOK && ptOK && !pOK {
		p = quantizeUnitPrice(pt.Div(onePlusRate))
		pOK = true
		item["price"] = decimalToString(p)
	}

	priceWasProvided := presence.price || presence.taxInclusivePrice
	if priceWasProvided && !presence.quantity {
		q = decimal.NewFromInt(1)
		qOK = true
		item["quantity"] = "1"
	}
	qEffective := decimal.NewFromInt(1)
	if qOK {
		qEffective = q
	}

	if !aOK && !atOK {
		if pOK && ptOK {
			a = quantizeCurrency(p.Mul(qEffective))
			at = quantizeCurrency(pt.Mul(qEffective))
			aOK, atOK = true, true
			item["amount"] = a.StringFixed(2)
			item["taxInclusiveAmount"] = at.StringFixed(2)
		}
	} else if aOK && !atOK && rOK {
		a = quantizeCurrency(a)
		at = quantizeCurrency(a.Mul(onePlusRate))
		atOK = true
		item["amount"] = a.StringFixed(2)
		item["taxInclusiveAmount"] = at.StringFixed(2)
	} else if !aOK && atOK && rOK {
		at = quantizeCurrency(at)
		a = quantizeCurrency(at.Div(onePlusRate))
		aOK = true
		item["amount"] = a.StringFixed(2)
		item["taxInclusiveAmount"] = at.StringFixed(2)
	} else {
		if aOK {
			a = quantizeCurrency(a)
			item["amount"] = a.StringFixed(2)
		}
		if atOK {
			at = quantizeCurrency(at)
			item["taxInclusiveAmount"] = at.StringFixed(2)
		}
	}

	if !presence.tax && aOK && atOK {
		item["tax"] = at.Sub(a).Round(2).StringFixed(2)
	} else if tax, ok := ToDecimal(item["tax"]); ok {
		item["tax"] = quantizeCurrency(tax).StringFixed(2)
	}

	errors := []string{}
	if presence.quantity && !priceWasProvided {
		errors = append(errors, buildPriceQuantityPairError(invoiceIndex, itemIndex, item["quantity"], q, qOK))
	}
	if !presence.amount && !presence.taxInclusiveAmount && !presence.price && !presence.taxInclusivePrice {
		errors = append(errors, lineAmountBasisMissingError(invoiceIndex, itemIndex))
		return errors
	}
	if !rOK {
		return errors
	}

	if aOK && atOK {
		expectedAT := quantizeCurrency(a.Mul(onePlusRate))
		if differenceExceedsTolerance(expectedAT, at) {
			errors = append(errors, buildAmountTaxRelationError(invoiceIndex, itemIndex, a, at, r, expectedAT, presence))
		}
	}
	if priceWasProvided && pOK && ptOK {
		expectedPT := quantizeUnitPrice(p.Mul(onePlusRate))
		if differenceExceedsTolerance(expectedPT, pt) {
			errors = append(errors, buildUnitPriceTaxRelationError(invoiceIndex, itemIndex, p, pt, r, expectedPT, presence))
		}
		if atOK {
			expectedAT := quantizeCurrency(pt.Mul(qEffective))
			if differenceExceedsTolerance(expectedAT, at) {
				errors = append(errors, buildTaxInclusiveTotalError(invoiceIndex, itemIndex, pt, qEffective, at, r, expectedAT, presence))
			}
		}
		if aOK {
			expectedA := quantizeCurrency(p.Mul(qEffective))
			if differenceExceedsTolerance(expectedA, a) {
				errors = append(errors, buildTaxExclusiveTotalError(invoiceIndex, itemIndex, p, qEffective, a, r, expectedA, presence))
			}
		}
	}
	return errors
}

func buildPriceQuantityPairError(invoiceIndex, itemIndex int, rawQuantity any, quantity decimal.Decimal, quantityOK bool) string {
	quantityText := strings.TrimSpace(fmt.Sprint(rawQuantity))
	if quantityOK {
		quantityText = decimalPlainText(quantity)
	}
	return fmt.Sprintf(
		"[%s] 第%d张发票第%d条明细：提供了数量但未提供单价，无法形成合法的单价数量组合。quantity=%s（用户提供），price未提供，taxInclusivePrice未提供；最终开票请求要求price与quantity同时存在或同时不存在，系统不会根据总额反推单价。调整建议：若按单价和数量开票，请补充不含税单价price或含税单价taxInclusivePrice中的任意一种，系统会按税率补齐另一种；若仅按总额开票，请删除quantity。",
		linePriceQuantityPairMismatchCode, invoiceIndex, itemIndex, quantityText,
	)
}

func lineAmountBasisMissingError(invoiceIndex, itemIndex int) string {
	return fmt.Sprintf(
		"[%s] 第%d张发票第%d条明细无法确定开票金额：请至少提供含税总额、不含税总额、含税单价或不含税单价中的一种；仅提供税额或数量不能确定开票金额。",
		lineAmountBasisMissingCode, invoiceIndex, itemIndex,
	)
}

func differenceExceedsTolerance(expected, actual decimal.Decimal) bool {
	return expected.Sub(actual).Abs().GreaterThan(lineAmountTolerance)
}

func buildAmountTaxRelationError(invoiceIndex, itemIndex int, a, at, r, expectedAT decimal.Decimal, presence lineAmountPresence) string {
	expectedA := quantizeCurrency(at.Div(decimal.NewFromInt(1).Add(r)))
	suggestion := amountPairSuggestion("不含税总额", a, presence.amount, expectedA, "含税总额", at, presence.taxInclusiveAmount, expectedAT)
	return fmt.Sprintf(
		"[%s] 第%d张发票第%d条明细：不含税总额、含税总额与税率不一致。amount=%s（%s），taxInclusiveAmount=%s（%s），税率=%s；按“%s×(1+%s)”计算，taxInclusiveAmount应为%s，当前为%s，差额为%s，允许差额为0.01。调整建议：%s",
		lineAmountTaxRelationMismatchCode, invoiceIndex, itemIndex,
		moneyText(a), valueSource(presence.amount), moneyText(at), valueSource(presence.taxInclusiveAmount), rateText(r),
		moneyText(a), rateText(r), moneyText(expectedAT), moneyText(at), differenceText(expectedAT, at), suggestion,
	)
}

func buildUnitPriceTaxRelationError(invoiceIndex, itemIndex int, p, pt, r, expectedPT decimal.Decimal, presence lineAmountPresence) string {
	expectedP := quantizeUnitPrice(pt.Div(decimal.NewFromInt(1).Add(r)))
	suggestion := amountPairSuggestion("不含税单价", p, presence.price, expectedP, "含税单价", pt, presence.taxInclusivePrice, expectedPT)
	return fmt.Sprintf(
		"[%s] 第%d张发票第%d条明细：不含税单价、含税单价与税率不一致。price=%s（%s），taxInclusivePrice=%s（%s），税率=%s；按“%s×(1+%s)”计算，taxInclusivePrice应为%s，当前为%s，差额为%s，允许差额为0.01。调整建议：%s",
		lineUnitPriceTaxRelationMismatchCode, invoiceIndex, itemIndex,
		unitPriceText(p), valueSource(presence.price), unitPriceText(pt), valueSource(presence.taxInclusivePrice), rateText(r),
		unitPriceText(p), rateText(r), unitPriceText(expectedPT), unitPriceText(pt), differenceText(expectedPT, pt), suggestion,
	)
}

func buildTaxInclusiveTotalError(invoiceIndex, itemIndex int, pt, q, at, r, expectedAT decimal.Decimal, presence lineAmountPresence) string {
	quantity := quantityDescription(q, presence.quantity)
	suggestion := totalSuggestion("含税单价", pt, presence.taxInclusivePrice, "含税总额", at, presence.taxInclusiveAmount, q, presence.quantity, true)
	return fmt.Sprintf(
		"[%s] 第%d张发票第%d条明细：含税单价乘数量与含税总额不一致。taxInclusivePrice=%s（%s），%s，taxInclusiveAmount=%s（%s），税率=%s；按“%s×%s”计算，taxInclusiveAmount应为%s，当前为%s，差额为%s，允许差额为0.01。调整建议：%s",
		lineTaxInclusiveTotalMismatchCode, invoiceIndex, itemIndex,
		unitPriceText(pt), valueSource(presence.taxInclusivePrice), quantity, moneyText(at), valueSource(presence.taxInclusiveAmount), rateText(r),
		unitPriceText(pt), decimalPlainText(q), moneyText(expectedAT), moneyText(at), differenceText(expectedAT, at), suggestion,
	)
}

func buildTaxExclusiveTotalError(invoiceIndex, itemIndex int, p, q, a, r, expectedA decimal.Decimal, presence lineAmountPresence) string {
	quantity := quantityDescription(q, presence.quantity)
	suggestion := totalSuggestion("不含税单价", p, presence.price, "不含税总额", a, presence.amount, q, presence.quantity, false)
	return fmt.Sprintf(
		"[%s] 第%d张发票第%d条明细：不含税单价乘数量与不含税总额不一致。price=%s（%s），%s，amount=%s（%s），税率=%s；按“%s×%s”计算，amount应为%s，当前为%s，差额为%s，允许差额为0.01。调整建议：%s",
		lineTaxExclusiveTotalMismatchCode, invoiceIndex, itemIndex,
		unitPriceText(p), valueSource(presence.price), quantity, moneyText(a), valueSource(presence.amount), rateText(r),
		unitPriceText(p), decimalPlainText(q), moneyText(expectedA), moneyText(a), differenceText(expectedA, a), suggestion,
	)
}

func amountPairSuggestion(leftName string, left decimal.Decimal, leftUser bool, expectedLeft decimal.Decimal, rightName string, right decimal.Decimal, rightUser bool, expectedRight decimal.Decimal) string {
	parts := []string{}
	if rightUser {
		parts = append(parts, fmt.Sprintf("若%s正确，请将%s调整为%s", leftName, rightName, valueForSuggestion(expectedRight, strings.Contains(rightName, "单价"))))
	}
	if leftUser {
		parts = append(parts, fmt.Sprintf("若%s正确，请将%s调整为%s", rightName, leftName, valueForSuggestion(expectedLeft, strings.Contains(leftName, "单价"))))
	}
	if len(parts) == 0 {
		return "请核对本条明细中用户提供的金额、单价、数量和税率后重新检查"
	}
	return strings.Join(parts, "；") + "。"
}

func totalSuggestion(priceName string, price decimal.Decimal, priceUser bool, totalName string, total decimal.Decimal, totalUser bool, quantity decimal.Decimal, quantityUser, inclusive bool) string {
	parts := []string{}
	expectedTotal := quantizeCurrency(price.Mul(quantity))
	if totalUser {
		parts = append(parts, fmt.Sprintf("若%s正确，请将%s调整为%s", priceName, totalName, moneyText(expectedTotal)))
	}
	if priceUser && !quantity.IsZero() {
		expectedPrice := quantizeUnitPrice(total.Div(quantity))
		parts = append(parts, fmt.Sprintf("若%s正确，请将%s调整为%s", totalName, priceName, unitPriceText(expectedPrice)))
	}
	if quantityUser && !price.IsZero() {
		expectedQuantity := total.Div(price)
		parts = append(parts, fmt.Sprintf("若单价和总额正确，请将数量调整为%s", decimalPlainText(expectedQuantity)))
	}
	if !quantityUser {
		parts = append(parts, "系统当前将数量默认为1；若实际数量不是1，请提供正确的quantity；若数量确实为1，请核对用户提供的单价或总额")
	}
	if len(parts) == 0 {
		basis := "不含税"
		if inclusive {
			basis = "含税"
		}
		return fmt.Sprintf("请核对用户提供的%s单价、总额或数量后重新检查", basis)
	}
	return strings.Join(parts, "；") + "。"
}

func valueSource(userProvided bool) string {
	if userProvided {
		return "用户提供"
	}
	return "系统补算"
}

func quantityDescription(quantity decimal.Decimal, userProvided bool) string {
	if !userProvided {
		return "quantity=1（系统默认；用户未提供，最终请求将传1）"
	}
	return fmt.Sprintf("quantity=%s（用户提供）", decimalPlainText(quantity))
}

func moneyText(value decimal.Decimal) string {
	return quantizeCurrency(value).StringFixed(2)
}

func unitPriceText(value decimal.Decimal) string {
	return decimalPlainText(quantizeUnitPrice(value))
}

func rateText(value decimal.Decimal) string {
	return formatTaxRatePercentage(value)
}

func differenceText(expected, actual decimal.Decimal) string {
	return decimalPlainText(expected.Sub(actual).Abs())
}

func decimalPlainText(value decimal.Decimal) string {
	return value.String()
}

func valueForSuggestion(value decimal.Decimal, unitPrice bool) string {
	if unitPrice {
		return unitPriceText(value)
	}
	return moneyText(value)
}

func normalizeLineItemDiscounts(lineItems []any, missing map[string][]map[string]any) {
	for index, itemAny := range lineItems {
		item, ok := itemAny.(map[string]any)
		if !ok {
			continue
		}
		if discountErr := normalizeDiscountInfoEquivalent(item); discountErr != "" {
			addMissingField(missing, "发票明细", fmt.Sprintf("invoiceDetail.data[%d].discountInfo", index), discountErr)
		}
	}
}
