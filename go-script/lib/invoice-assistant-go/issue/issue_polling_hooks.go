package issue

import (
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

var (
	sleepBeforeIssueStatusPoll = time.Sleep
	submitInvoiceIssueRequest  = submitInvoiceIssue
	submitIssueStatusRequest   = submitIssueStatus
	prepareIssueLoginReadiness = common.PrepareLoginReadiness
)
