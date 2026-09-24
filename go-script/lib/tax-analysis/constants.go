package taxanalysis

import "time"

const (
	DefaultCapabilityCode  = "tax_analysis"
	BoundEnterprisePath    = "/share/enterprise-list"
	CreateTaskPath         = "/tax-analysis/create-task"
	QueryTaskPath          = "/tax-analysis/query-task"
	PublicCreateTaskPath   = "/api/v1/skill" + CreateTaskPath
	PublicQueryTaskPath    = "/api/v1/skill" + QueryTaskPath
	DefaultTimeout         = 30 * time.Second
	DefaultPollInterval    = 5 * time.Second
	MinimumRequestInterval = 5 * time.Second
	DefaultPollTimeout     = 30 * time.Minute
	MaximumRangeMonths     = 36
	MinimumStartYear       = 2017
	MonthLayout            = "2006-01"
	BusinessSuccessCode    = 0
	BoundEnterpriseCode    = 200
	BoundEnterpriseTimeout = 15 * time.Second
)

var boundEnterpriseContextFields = []string{
	"enterpriseName",
	"uscc",
	"areaCode",
	"agencyUSCC",
	"loginType",
	"identityType",
	"personalAccount",
	"nsrzg",
}

const (
	StatusPending        TaskStatus = "pending"
	StatusRunning        TaskStatus = "running"
	StatusSuccess        TaskStatus = "success"
	StatusPartialSuccess TaskStatus = "partial_success"
	StatusFailed         TaskStatus = "failed"
	StatusInterrupted    TaskStatus = "interrupted"
)

const (
	ModuleStatusPending      ModuleStatus = "pending"
	ModuleStatusRunning      ModuleStatus = "running"
	ModuleStatusSuccess      ModuleStatus = "success"
	ModuleStatusFailed       ModuleStatus = "failed"
	ModuleStatusNotSupported ModuleStatus = "not_supported"
)
