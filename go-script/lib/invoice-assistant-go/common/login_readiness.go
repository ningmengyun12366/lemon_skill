package common

import (
	"fmt"
	"strings"
)

type LoginReadinessRequest struct {
	Enterprise map[string]any
	Config     map[string]any
	Client     *APIClient
}

type LoginReadinessResult struct {
	LoginStatus map[string]any
}

func PrepareLoginReadiness(request LoginReadinessRequest) (LoginReadinessResult, error) {
	enterprise := normalizeEnterpriseRecord(request.Enterprise)
	config := request.Config
	if config == nil {
		config = LoadWorkspaceConfig()
	}
	client := request.Client
	if client == nil {
		client = NewAPIClient()
	}
	enterpriseName := pickFirstNonEmpty(enterprise["enterpriseName"])
	apikey := strings.TrimSpace(StringValue(config[APIKey]))
	loginOK, loginStatus, loginIssues := checkLoginStatus(enterpriseName, apikey, config, enterprise, client, quickAutoLoginEnabled)
	if !loginOK {
		return LoginReadinessResult{LoginStatus: loginStatus}, formatLoginReadinessError("login readiness", loginIssues)
	}
	return LoginReadinessResult{LoginStatus: loginStatus}, nil
}

func formatLoginReadinessError(stage string, issues []checkIssue) error {
	if len(issues) == 0 {
		return fmt.Errorf("%s failed", stage)
	}
	issue := issues[0]
	detail := strings.TrimSpace(issue.Detail)
	if detail == "" {
		detail = strings.TrimSpace(issue.Title)
	}
	suggestion := strings.TrimSpace(issue.Suggestion)
	if suggestion != "" && suggestion != detail {
		detail += "\n" + suggestion
	}
	return fmt.Errorf("%s failed (%s): %s", stage, issue.Code, detail)
}
