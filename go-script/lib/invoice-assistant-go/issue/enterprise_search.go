package issue

import (
	"fmt"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func SearchEnterprise(nameOrUSCC string) (map[string]any, error) {
	query := strings.TrimSpace(nameOrUSCC)
	if query == "" {
		return nil, fmt.Errorf("missing required enterprise query")
	}
	candidates, err := fetchEnterpriseSearchCandidates(query)
	if err != nil {
		return nil, err
	}
	result := map[string]any{
		"query":                query,
		"candidate_count":      len(candidates),
		"candidates":           candidates,
		"needs_user_selection": len(candidates) > 1,
		"selected_candidate":   nil,
	}
	switch len(candidates) {
	case 0:
		result["message"] = "未查询到企业信息"
	case 1:
		result["selected_candidate"] = candidates[0]
		result["message"] = "查询到唯一企业，已可默认选择"
	default:
		result["message"] = "查询到多个企业，仅作为可选参考；用户可选择其中一个写回，也可不选并继续普通发票草稿"
	}
	return result, nil
}

var fetchEnterpriseSearchCandidates = func(nameOrUSCC string) ([]map[string]any, error) {
	payload, err := enterpriseSearchRequestPayload(nameOrUSCC)
	if err != nil {
		return nil, err
	}
	client := newAPIClient()
	client.HeaderProvider = func() map[string]string {
		return map[string]string{"Content-Type": "application/json"}
	}
	resp, err := client.PostJSON("/share/search-enterprise", payload, 15*time.Second)
	if err != nil {
		return nil, err
	}
	if responseCode(resp) != 200 {
		return nil, fmt.Errorf("%s", common.StringValue(resp["message"]))
	}
	items, _ := asList(resp["data"])
	candidates := make([]map[string]any, 0, len(items))
	for _, item := range items {
		raw, _ := item.(map[string]any)
		if raw == nil {
			continue
		}
		name := strings.TrimSpace(common.StringValue(raw["name"]))
		uscc := strings.TrimSpace(common.StringValue(raw["uscc"]))
		if name == "" && uscc == "" {
			continue
		}
		candidates = append(candidates, map[string]any{"name": name, "uscc": uscc})
	}
	return candidates, nil
}

func enterpriseSearchRequestPayload(nameOrUSCC string) (map[string]any, error) {
	config := common.LoadWorkspaceConfig()
	apiKey := strings.TrimSpace(common.StringValue(config[common.APIKey]))
	if apiKey == "" {
		return nil, fmt.Errorf("workspace-config.json 中缺少 apikey，无法查询企业信息")
	}
	return map[string]any{
		"apiKey":     apiKey,
		"nameOrUscc": nameOrUSCC,
	}, nil
}
