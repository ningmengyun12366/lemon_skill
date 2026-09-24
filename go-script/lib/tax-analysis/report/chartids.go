package report

import "sort"

func chartIDs(model Model) []string {
	set := map[string]bool{}
	for _, section := range model.Sections {
		for _, block := range section.Blocks {
			if block.Chart != nil {
				set[block.Chart.ID] = true
			}
		}
	}
	ids := make([]string, 0, len(set))
	for id := range set {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}
