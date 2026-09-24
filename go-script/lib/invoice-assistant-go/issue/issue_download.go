package issue

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"invoice-assistant-skill/go-script/lib/invoice-assistant-go/common"
)

func downloadIssueFiles(resp map[string]any, taskState map[string]any) []any {
	items := extractStatusItems(resp)
	root := resolveIssueAttachmentRoot(taskState)
	results := []any{}
	for idx, item := range items {
		issueNo := extractItemIssueNumber(item, idx)
		targetDir := filepath.Join(root, common.SanitizeFilename(issueNo, fmt.Sprintf("issue-item-%d", idx+1)))
		fileInfo, _ := item["file"].(map[string]any)
		localFile := map[string]any{}
		errors := map[string]any{}
		for _, kind := range []string{"pdf", "xml", "ofd"} {
			remoteURL := issueFileURL(fileInfo, kind)
			if remoteURL == "" {
				continue
			}
			job := common.DownloadJob{URL: remoteURL, TargetFilename: issueNo + guessFileSuffix(kind, remoteURL), FallbackName: issueNo + "." + kind}
			result := common.DownloadMany([]common.DownloadJob{job}, targetDir, issueNo, nil, 120*time.Second, "url")
			successes, _ := asList(result["successes"])
			if len(successes) > 0 {
				success, _ := successes[0].(map[string]any)
				localFile[kind] = success["saved_path"]
			}
			failures, _ := asList(result["failures"])
			if len(failures) > 0 {
				failure, _ := failures[0].(map[string]any)
				errors[kind] = failure["error"]
			}
		}
		if len(localFile) > 0 {
			item["local_file"] = localFile
		}
		if len(errors) > 0 {
			item["local_file_errors"] = errors
		}
		results = append(results, map[string]any{"issue_number": issueNo, "destination_dir": targetDir, "local_file": localFile, "local_file_errors": errors})
	}
	return results
}

func resolveIssueAttachmentRoot(taskState map[string]any) string {
	if path := strings.TrimSpace(common.StringValue(GetPath(taskState, "storage.attachments_dir"))); path != "" {
		return path
	}
	workspace := common.StringValue(taskState["workspace_dir"])
	if workspace == "" {
		workspace, _ = ResolveWorkspaceDir()
	}
	return filepath.Join(workspace, issueRunsDirName, attachmentsDirName)
}

func resolveIssueResultMarkdownOutputPath(state *TaskState) string {
	if path := strings.TrimSpace(common.StringValue(GetPath(state.Data, "storage.preview_dir"))); path != "" {
		return filepath.Join(path, fileStem(state.Path)+"-issue-result.md")
	}
	return ""
}

func writeTextFile(path, content string) error {
	if err := ensureWritableDir(filepath.Dir(path)); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func extractItemIssueNumber(item map[string]any, index int) string {
	nested, _ := item["data"].(map[string]any)
	for _, candidate := range []any{nested["sdphm"], item["sdphm"], item["fplsh"], item["id"], item["invoiceNo"], item["fphm"], item["invoiceNumber"]} {
		if value := strings.TrimSpace(common.StringValue(candidate)); value != "" {
			return common.SanitizeFilename(value, fmt.Sprintf("invoice_%d", index+1))
		}
	}
	return fmt.Sprintf("issue-item-%d", index+1)
}

func extractItemIssueTotal(item map[string]any) any {
	if nested, ok := item["data"].(map[string]any); ok {
		return firstNonNil(nested["jshj"], nested["je"], item["hjje"])
	}
	return item["hjje"]
}

func extractItemTaxAmount(item map[string]any) any {
	if nested, ok := item["data"].(map[string]any); ok {
		return nested["se"]
	}
	return nil
}

func guessFileSuffix(kind, rawURL string) string {
	if parsed, err := url.Parse(rawURL); err == nil {
		if ext := filepath.Ext(parsed.Path); ext != "" {
			return ext
		}
	}
	return "." + kind
}
