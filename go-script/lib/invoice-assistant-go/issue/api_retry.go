package issue

import (
	"time"

	"invoice-assistant-skill/go-script/lib/skillupdate"
)

const issueAPIMaxAttempts = 3

func postIssueJSONWithRetry(requestURL string, payload map[string]any, timeout time.Duration) (map[string]any, error) {
	var lastResp map[string]any
	var lastErr error
	for attempt := 0; attempt < issueAPIMaxAttempts; attempt++ {
		resp, err := newAPIClient().PostJSON(requestURL, payload, timeout)
		if err != nil {
			if outdated, ok := skillupdate.FindOutdatedError(err); ok {
				return nil, outdated
			}
			lastResp = nil
			lastErr = err
			continue
		}
		if responseCode(resp) == 0 {
			return resp, nil
		}
		lastResp = resp
		lastErr = nil
	}
	if lastResp != nil {
		return lastResp, nil
	}
	return nil, lastErr
}
