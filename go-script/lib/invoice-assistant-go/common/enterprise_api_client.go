package common

import (
	"fmt"
	"strings"
)

type EnterpriseRequestContext struct {
	USCC            string
	PersonalAccount string
	LoginType       string
	CapabilityCode  string
}

func NewEnterpriseAPIClient(base *APIClient, config map[string]any, requestContext EnterpriseRequestContext) *APIClient {
	if base == nil {
		base = NewAPIClient()
	}
	client := &APIClient{
		BaseURL:        base.BaseURL,
		Client:         base.Client,
		VersionChecker: base.VersionChecker,
		RequestSender:  base.RequestSender,
	}
	client.HeaderProvider = func() map[string]string {
		return BuildEnterpriseRequestHeaders(config, requestContext)
	}
	return client
}

func BuildEnterpriseRequestHeaders(config map[string]any, requestContext EnterpriseRequestContext) map[string]string {
	headers := map[string]string{"Content-Type": "application/json"}
	apikey := strings.TrimSpace(StringValue(config[APIKey]))
	uscc := strings.TrimSpace(requestContext.USCC)
	capabilityCode := strings.TrimSpace(requestContext.CapabilityCode)
	if capabilityCode == "" {
		capabilityCode = CapabilityCode
	}
	if apikey != "" {
		headers["Authorization"] = fmt.Sprintf("apikey=%s; uscc=%s; capabilityCode=%s", apikey, uscc, capabilityCode)
	}
	return headers
}
