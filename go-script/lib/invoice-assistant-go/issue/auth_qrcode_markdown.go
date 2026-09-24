package issue

import (
	"path/filepath"
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func buildQRCodeMarkdown(_ map[string]any, entries []map[string]string) string {
	lines := []string{
		"# 开票权限认证二维码",
		"",
		"- 当前状态：待扫码",
		"- 二维码链接有效期为 5 分钟，请及时扫码完成认证。",
		"",
		"## 二维码列表",
	}
	for _, entry := range entries {
		imagePath := filepath.ToSlash(strings.TrimSpace(entry[qrcodeImagePathKey]))
		label := firstString(entry["qrcodeTypeLabel"], entry["label"])
		if label == "" {
			label = "未知认证平台"
		}
		lines = append(lines,
			"",
			"### "+label,
			"- 二维码图片路径："+imagePath,
		)
	}
	if len(entries) == 0 {
		lines = append(lines, "", "接口未返回有效二维码。")
	}
	lines = append(lines,
		"",
		"## 下一步",
		"- 请任选一种对应 APP 完成扫码和人脸认证，无需两个二维码都扫码。",
		"- 用户确认扫码完成后，再执行 verify 子命令校验扫码状态与认证状态。",
	)
	return strings.Join(lines, "\n")
}

func buildVerifyMarkdown(_ map[string]any, entries []map[string]string, authResult map[string]any) string {
	currentStatus := buildVerifyStatus(entries, authResult)
	lines := []string{
		"# 开票权限认证状态",
		"",
		"- 当前状态：" + verifyStatusText(currentStatus),
		"",
		"## 二维码扫码状态",
	}
	for _, entry := range entries {
		label := firstString(entry["qrcodeTypeLabel"], entry["label"])
		if label == "" {
			label = "未知认证平台"
		}
		lines = append(lines,
			"",
			"- "+label+"："+entry["statusLabel"],
		)
	}
	lines = append(lines, "", "## 人脸认证状态")
	if authResult == nil {
		lines = append(lines, "- 尚未执行认证状态查询")
	} else {
		status := common.StringValue(GetPath(authResult, "data.status"))
		label := authStatusLabels[status]
		if label == "" {
			label = "未知状态(" + status + ")"
		}
		lines = append(lines,
			"- 认证任务状态："+label,
			"- 是否完成认证："+authFinishedText(GetPath(authResult, "data.isAuth")),
		)
	}
	return strings.Join(append(lines, verifyNextStepLines(currentStatus)...), "\n")
}

func verifyStatusText(status string) string {
	labels := map[string]string{
		"auth_passed":    "认证通过",
		"auth_pending":   "扫码成功，认证处理中",
		"auth_failed":    "扫码成功，认证失败",
		"scan_success":   "扫码成功，待查询认证状态",
		"login_required": "扫码成功，需重新登录",
		"qrcode_expired": "二维码失效",
		"awaiting_scan":  "等待扫描",
	}
	if label := labels[status]; label != "" {
		return label
	}
	return status
}

func verifyNextStepLines(status string) []string {
	switch status {
	case "awaiting_scan":
		return []string{"", "## 下一步", "- 继续完成扫码后，再次执行 verify 子命令。"}
	case "qrcode_expired":
		return []string{"", "## 下一步", "- 请重新执行 start 子命令获取新的二维码。"}
	case "auth_pending":
		return []string{"", "## 下一步", "- 扫码已完成，但认证结果仍在处理中，可稍后再次执行 verify 子命令。"}
	case "auth_failed":
		return []string{"", "## 下一步", "- 人脸认证失败，请重新扫码发起一次认证流程。"}
	case "auth_passed":
		return []string{"", "## 结论", "- 开票权限认证已通过，可返回开票流程继续提交。"}
	case "login_required":
		return []string{"", "## 下一步", "- 扫码结果已保留。请按命令返回的安全登录入口重新登录，再次执行 verify 子命令。"}
	default:
		return nil
	}
}

func authFinishedText(isAuth any) string {
	if legacyPythonTruthy(isAuth) {
		return "已完成"
	}
	return "未完成"
}
