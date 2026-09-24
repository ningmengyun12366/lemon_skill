package report

// isAgentNarrativeBlock reports whether a paragraph block contains validated
// Agent analysis. Parameters contain one finalized block. Return value excludes
// script-owned handling advice and data-gap notices.
func isAgentNarrativeBlock(block Block) bool {
	if block.Kind != BlockParagraph {
		return false
	}
	switch block.Title {
	case sectionNarrativeTitle(NarrativeRisk), sectionNarrativeTitle(NarrativeAttention), sectionNarrativeTitle(NarrativeFact), "综合结论":
		return true
	default:
		return false
	}
}

// shouldKeepParagraphsWithChart identifies narrative copy that must stay with
// its immediately following chart. Parameters: blocks is one finalized section
// and index is the current block. Return value is true for chart descriptions
// stored on the chart block or in the paragraph block directly before it.
func shouldKeepParagraphsWithChart(blocks []Block, index int) bool {
	if index < 0 || index >= len(blocks) {
		return false
	}
	if blocks[index].Kind == BlockChart && len(blocks[index].Paragraphs) > 0 {
		return true
	}
	return index+1 < len(blocks) && len(blocks[index].Paragraphs) > 0 && blocks[index+1].Kind == BlockChart
}
