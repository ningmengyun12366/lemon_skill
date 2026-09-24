package report

const (
	// RuleVersion identifies the deterministic business-rule set used by the
	// prepared report model.
	RuleVersion = "2026.09.v36"
	// AIContractVersion identifies the strict Agent-to-renderer JSON contract.
	AIContractVersion = "tax-analysis-ai.v8"

	workspaceManifestFilename       = "manifest.json"
	workspaceSourceFilename         = "source-snapshot.json"
	workspaceModelFilename          = "report-model.json"
	workspaceAnalysisTaskFilename   = "analysis-task.md"
	workspaceAnalysisOutputFilename = "analysis-output.json"
	workspaceValidationFilename     = "validation-result.json"
	workspaceAnalysisDataFilename   = "analysis-data.json"
)

var preservedFactParagraphTitles = map[string]bool{
	"数据口径":     true,
	"数据说明":     true,
	"计算口径":     true,
	"口径说明":     true,
	"财务指标综合提示": true,
}
