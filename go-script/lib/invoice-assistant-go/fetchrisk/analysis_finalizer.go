package fetchrisk

import (
	"strings"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func outputRequirementForAnalysis(task map[string]any) string {
	intent := common.ObjectValue(task, "intent")
	params := common.ObjectValue(intent, "params")
	return strings.TrimSpace(common.StringValue(params["outputRequirement"]))
}

func finalizeLoadedAnalysisArtifacts(state *TaskState) error {
	outputs := common.ObjectValue(state.Data, "outputs")
	artifacts := common.ObjectValue(state.Data, "artifacts")
	paths := uniqueNonEmptyPaths(
		common.StringValue(outputs["summary_markdown_path"]),
		common.StringValue(artifacts["summary_markdown_path"]),
		common.StringValue(outputs["deduction_data_file"]),
		common.StringValue(artifacts["deduction_data_file"]),
	)
	roots, err := analysisFinalizerDeletionRoots(state)
	if err != nil {
		return err
	}
	if err := validateFinalizerDeletionPaths(paths, roots); err != nil {
		return err
	}
	for _, path := range paths {
		if err := removeFileIfExists(path); err != nil {
			return err
		}
	}
	deleteFinalizedAnalysisRefs(outputs)
	deleteFinalizedAnalysisRefs(artifacts)
	return nil
}

func analysisFinalizerDeletionRoots(state *TaskState) ([]finalizerDeletionRoot, error) {
	storage := common.ObjectValue(state.Data, "storage")
	if root := strings.TrimSpace(common.StringValue(storage["root_dir"])); root != "" {
		return canonicalFinalizerDeletionRoots([]string{root})
	}
	return canonicalFinalizerDeletionRoots([]string{common.StringValue(state.Data["workspace_dir"])})
}

func deleteFinalizedAnalysisRefs(taskMap map[string]any) {
	delete(taskMap, "summary_markdown_path")
	delete(taskMap, "summary_markdown")
	delete(taskMap, "deduction_data_file")
	delete(taskMap, "deduction_data_list")
	delete(taskMap, "deduction_markdown")
}
