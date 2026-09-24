package report

import (
	"encoding/json"
	"fmt"
)

const taxTableFormat = "tax-table.v1"

// decodeTaxTableRows consumes only the server's text dictionary and coordinates.
// It never parses HTML or manufactures a page from an older response.
func decodeTaxTableRows(value any) ([]formTableRow, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	var table struct {
		Format string              `json:"format"`
		Texts  []string            `json:"texts"`
		Rows   [][]json.RawMessage `json:"rows"`
	}
	if err = json.Unmarshal(data, &table); err != nil || table.Format != taxTableFormat {
		return nil, fmt.Errorf("缺少服务端结构化税表")
	}
	result := make([]formTableRow, 0, len(table.Rows))
	for _, row := range table.Rows {
		if len(row) != 4 {
			return nil, fmt.Errorf("结构化税表行坐标不完整")
		}
		var out formTableRow
		if err := json.Unmarshal(row[0], &out.Table); err != nil {
			return nil, err
		}
		if err := json.Unmarshal(row[1], &out.Index); err != nil {
			return nil, err
		}
		for index, target := range []*[]string{&out.Cells, &out.Grid} {
			var ids []int
			if err := json.Unmarshal(row[index+2], &ids); err != nil {
				return nil, err
			}
			for _, id := range ids {
				if id < 0 || id >= len(table.Texts) {
					return nil, fmt.Errorf("结构化税表文本引用不存在")
				}
				*target = append(*target, table.Texts[id])
			}
		}
		result = append(result, out)
	}
	return result, nil
}

func isTaxTable(value any) bool {
	object, ok := value.(map[string]any)
	return ok && mapText(object, "format") == taxTableFormat
}

func taxTableHint(value any, name string) bool {
	object, ok := value.(map[string]any)
	if !ok {
		return false
	}
	hints, ok := object["hints"].(map[string]any)
	return ok && hints[name] == true
}
