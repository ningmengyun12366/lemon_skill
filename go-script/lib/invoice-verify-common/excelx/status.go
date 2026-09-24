package excelx

import "strings"

const (
	UnsupportedVerifyStatus = "该文件不支持查验"
)

func IsUnsupportedVerifyStatus(status string) bool {
	return strings.TrimSpace(status) == UnsupportedVerifyStatus
}
