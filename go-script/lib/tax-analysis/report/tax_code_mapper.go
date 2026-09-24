package report

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"unicode"
)

//go:embed dictionaries/tax-analysis-tax-codes.json
var taxAnalysisTaxCodesJSON []byte

type taxProjectCode struct {
	Code string `json:"code"`
	Name string `json:"name"`
}
type taxItemCode struct {
	ProjectCode string `json:"projectCode"`
	Code        string `json:"code"`
	Name        string `json:"name"`
}
type taxCodeDictionary struct {
	Version            string           `json:"version"`
	SourceSHA256       string           `json:"sourceSha256"`
	Projects           []taxProjectCode `json:"projects"`
	HistoricalProjects []taxProjectCode `json:"historicalProjects"`
	Items              []taxItemCode    `json:"items"`
}

var analysisTaxDictionary = loadTaxAnalysisDictionary()

var taxBusinessNames = func() map[string]bool {
	names := map[string]bool{}
	for _, project := range analysisTaxDictionary.Projects {
		names[canonicalFormLabel(project.Name)] = true
	}
	for _, item := range analysisTaxDictionary.Items {
		names[canonicalFormLabel(item.Name)] = true
	}
	return names
}()
var taxProjectNames = func() map[string]bool {
	names := map[string]bool{}
	for _, project := range analysisTaxDictionary.Projects {
		names[canonicalFormLabel(project.Name)] = true
	}
	return names
}()

func loadTaxAnalysisDictionary() taxCodeDictionary {
	var dictionary taxCodeDictionary
	if err := json.Unmarshal(taxAnalysisTaxCodesJSON, &dictionary); err != nil {
		panic("税务分析专用征收字典无效: " + err.Error())
	}
	return dictionary
}

// taxProjectCodes reads only original collection-project codes. Form codes and
// the legacy TaxType display value do not participate in classification.
func taxProjectCodes(record map[string]any) []string {
	value, _ := mapValue(record, "zsxmDmArray", "zsxmDm", "projectCode")
	var texts []string
	if list, ok := value.([]any); ok {
		for _, item := range list {
			texts = append(texts, scalarText(item))
		}
	} else {
		texts = append(texts, scalarText(value))
	}
	seen := map[string]bool{}
	for _, text := range texts {
		for _, code := range strings.FieldsFunc(text, func(r rune) bool { return unicode.IsSpace(r) || strings.ContainsRune(",，;；、|", r) }) {
			if code != "" {
				seen[code] = true
			}
		}
	}
	codes := make([]string, 0, len(seen))
	for code := range seen {
		codes = append(codes, code)
	}
	sort.Strings(codes)
	return codes
}

func taxProjectName(code string) string {
	for _, item := range analysisTaxDictionary.Projects {
		if item.Code == code {
			return item.Name
		}
	}
	for _, item := range analysisTaxDictionary.HistoricalProjects {
		if item.Code == code {
			return item.Name
		}
	}
	return "未识别税种（" + code + "）"
}

// taxItemName requires an exact project/item relationship. Letter-containing
// and historical item codes are preserved without numeric conversion.
func taxItemName(project, code string) (string, bool) {
	for _, item := range analysisTaxDictionary.Items {
		if item.Code == code && item.ProjectCode == project {
			return item.Name, true
		}
	}
	return "", false
}

func taxCodeName(record map[string]any) (string, bool) {
	codes := taxProjectCodes(record)
	if len(codes) == 0 {
		return "", false
	}
	names := make([]string, 0, len(codes))
	for _, code := range codes {
		name := taxProjectName(code)
		// The broad other-income project must retain the official union item.
		if code == "39900" {
			if item, ok := taxItemName(code, mapText(record, "zspmDm", "itemCode")); ok {
				name = item
			}
		}
		names = append(names, name)
	}
	return strings.Join(names, "、"), true
}
