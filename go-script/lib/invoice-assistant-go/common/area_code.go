package common

import (
	"fmt"
	"strconv"
	"strings"
)

var areaCodeByName = map[string]int{
	"北京": 11, "北京市": 11,
	"天津": 12, "天津市": 12,
	"河北": 13, "河北省": 13,
	"山西": 14, "山西省": 14,
	"内蒙古": 15, "内蒙古自治区": 15,
	"辽宁": 21, "辽宁省": 21, "大连": 2102, "大连市": 2102,
	"吉林": 22, "吉林省": 22,
	"黑龙江": 23, "黑龙江省": 23,
	"上海": 31, "上海市": 31,
	"江苏": 32, "江苏省": 32,
	"浙江": 33, "浙江省": 33, "宁波": 3302, "宁波市": 3302,
	"安徽": 34, "安徽省": 34,
	"福建": 35, "福建省": 35, "厦门": 3502, "厦门市": 3502,
	"江西": 36, "江西省": 36,
	"山东": 37, "山东省": 37, "青岛": 3702, "青岛市": 3702,
	"河南": 41, "河南省": 41,
	"湖北": 42, "湖北省": 42,
	"湖南": 43, "湖南省": 43,
	"广东": 44, "广东省": 44, "深圳": 4403, "深圳市": 4403,
	"广西": 45, "广西壮族自治区": 45,
	"海南": 46, "海南省": 46,
	"重庆": 50, "重庆市": 50,
	"四川": 51, "四川省": 51,
	"贵州": 52, "贵州省": 52,
	"云南": 53, "云南省": 53,
	"西藏": 54, "西藏自治区": 54,
	"陕西": 61, "陕西省": 61,
	"甘肃": 62, "甘肃省": 62,
	"青海": 63, "青海省": 63,
	"宁夏": 64, "宁夏回族自治区": 64,
	"新疆": 65, "新疆维吾尔自治区": 65,
}

var cityNames = []string{"大连市", "大连", "宁波市", "宁波", "厦门市", "厦门", "青岛市", "青岛", "深圳市", "深圳"}

var provinceNames = []string{
	"北京市", "北京", "天津市", "天津", "河北省", "河北", "山西省", "山西",
	"内蒙古自治区", "内蒙古", "辽宁省", "辽宁", "吉林省", "吉林", "黑龙江省", "黑龙江",
	"上海市", "上海", "江苏省", "江苏", "浙江省", "浙江", "安徽省", "安徽",
	"福建省", "福建", "江西省", "江西", "山东省", "山东", "河南省", "河南",
	"湖北省", "湖北", "湖南省", "湖南", "广东省", "广东", "广西壮族自治区", "广西",
	"海南省", "海南", "重庆市", "重庆", "四川省", "四川", "贵州省", "贵州",
	"云南省", "云南", "西藏自治区", "西藏", "陕西省", "陕西", "甘肃省", "甘肃",
	"青海省", "青海", "宁夏回族自治区", "宁夏", "新疆维吾尔自治区", "新疆",
}

func ResolveAreaCode(value any) (int, error) {
	switch v := value.(type) {
	case bool:
		return 0, fmt.Errorf("areaCode cannot be boolean")
	case int:
		return v, nil
	case int64:
		return int(v), nil
	case float64:
		if v == float64(int(v)) {
			return int(v), nil
		}
		return 0, fmt.Errorf("cannot resolve areaCode: %v", value)
	}

	text := strings.Join(strings.Fields(StringValue(value)), "")
	if text == "" {
		return 0, fmt.Errorf("areaCode cannot be empty")
	}
	if onlyDigits(text) {
		code, _ := strconv.Atoi(text)
		return code, nil
	}
	if code, ok := areaCodeByName[text]; ok {
		return code, nil
	}
	for _, name := range cityNames {
		if strings.Contains(text, name) {
			return areaCodeByName[name], nil
		}
	}
	for _, name := range provinceNames {
		if strings.Contains(text, name) {
			return areaCodeByName[name], nil
		}
	}
	return 0, fmt.Errorf("cannot resolve areaCode from area name: %v", value)
}

func NormalizeStandardAreaCode(value any) (int, error) {
	code, err := ResolveAreaCode(value)
	if err != nil {
		return 0, err
	}
	if isStandardAreaCode(code) {
		return code, nil
	}
	text := strconv.Itoa(code)
	for end := len(text) - 1; end > 1; end-- {
		prefix := text[:end]
		suffix := text[end:]
		if strings.Trim(suffix, "0") != "" {
			continue
		}
		candidate, _ := strconv.Atoi(prefix)
		if isStandardAreaCode(candidate) {
			return candidate, nil
		}
	}
	return 0, fmt.Errorf("areaCode %v is not standard and cannot be normalized", value)
}

func isStandardAreaCode(code int) bool {
	for _, value := range areaCodeByName {
		if value == code {
			return true
		}
	}
	return false
}

func onlyDigits(text string) bool {
	for _, ch := range text {
		if ch < '0' || ch > '9' {
			return false
		}
	}
	return text != ""
}
