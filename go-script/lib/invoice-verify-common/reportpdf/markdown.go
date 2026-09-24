package reportpdf

import (
	"strings"
	"unicode"

	"github.com/yuin/goldmark"
	gast "github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	extast "github.com/yuin/goldmark/extension/ast"
	gmtext "github.com/yuin/goldmark/text"

	"github.com/gpdf-dev/gpdf/document"
)

type pdfBlockKind int

const (
	pdfBlockParagraph pdfBlockKind = iota
	pdfBlockHeading
	pdfBlockList
	pdfBlockTable
	pdfBlockRule
	pdfBlockCode
)

type pdfBlock struct {
	Kind    pdfBlockKind
	Level   int
	Text    string
	Ordered bool
	Items   []string
	Header  []string
	Rows    [][]string
	Aligns  []document.TextAlign
}

func parseMarkdownBlocks(reportText string) []pdfBlock {
	source := []byte(reportText)
	markdown := goldmark.New(goldmark.WithExtensions(extension.Table))
	documentNode := markdown.Parser().Parse(gmtext.NewReader(source))
	var blocks []pdfBlock
	for child := documentNode.FirstChild(); child != nil; child = child.NextSibling() {
		blocks = appendNodeBlocks(blocks, child, source)
	}
	if len(blocks) == 0 {
		blocks = append(blocks, pdfBlock{Kind: pdfBlockParagraph, Text: strings.TrimSpace(reportText)})
	}
	return blocks
}

func appendNodeBlocks(blocks []pdfBlock, node gast.Node, source []byte) []pdfBlock {
	switch n := node.(type) {
	case *gast.Heading:
		text := cleanBlockText(inlineText(n, source))
		if text != "" {
			blocks = append(blocks, pdfBlock{Kind: pdfBlockHeading, Level: n.Level, Text: text})
		}
	case *gast.Paragraph:
		text := cleanBlockText(inlineText(n, source))
		if text != "" {
			blocks = append(blocks, pdfBlock{Kind: pdfBlockParagraph, Text: text})
		}
	case *gast.List:
		items := collectListItems(n, source)
		if len(items) > 0 {
			blocks = append(blocks, pdfBlock{Kind: pdfBlockList, Ordered: n.IsOrdered(), Items: items})
		}
	case *extast.Table:
		if block, ok := collectTable(n, source); ok {
			blocks = append(blocks, block)
		}
	case *gast.ThematicBreak:
		blocks = append(blocks, pdfBlock{Kind: pdfBlockRule})
	case *gast.CodeBlock:
		text := strings.TrimRight(string(n.Lines().Value(source)), "\r\n")
		if text != "" {
			blocks = append(blocks, pdfBlock{Kind: pdfBlockCode, Text: text})
		}
	case *gast.FencedCodeBlock:
		text := strings.TrimRight(string(n.Lines().Value(source)), "\r\n")
		if text != "" {
			blocks = append(blocks, pdfBlock{Kind: pdfBlockCode, Text: text})
		}
	default:
		if node.HasChildren() {
			for child := node.FirstChild(); child != nil; child = child.NextSibling() {
				blocks = appendNodeBlocks(blocks, child, source)
			}
			return blocks
		}
		text := cleanBlockText(string(node.Text(source)))
		if text != "" {
			blocks = append(blocks, pdfBlock{Kind: pdfBlockParagraph, Text: text})
		}
	}
	return blocks
}

func collectListItems(list *gast.List, source []byte) []string {
	var items []string
	for child := list.FirstChild(); child != nil; child = child.NextSibling() {
		item, ok := child.(*gast.ListItem)
		if !ok {
			continue
		}
		text := cleanBlockText(inlineText(item, source))
		if text != "" {
			items = append(items, text)
		}
	}
	return items
}

func collectTable(table *extast.Table, source []byte) (pdfBlock, bool) {
	block := pdfBlock{Kind: pdfBlockTable, Aligns: tableAlignments(table.Alignments)}
	for child := table.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.(type) {
		case *extast.TableHeader:
			block.Header = collectTableCells(child, source)
		case *extast.TableRow:
			row := collectTableCells(child, source)
			if len(row) > 0 {
				block.Rows = append(block.Rows, row)
			}
		}
	}
	columnCount := maxTableColumns(block.Header, block.Rows)
	if columnCount == 0 {
		return pdfBlock{}, false
	}
	if len(block.Header) == 0 && len(block.Rows) > 0 {
		block.Header = block.Rows[0]
		block.Rows = block.Rows[1:]
	}
	block.Header = normalizeTableRow(block.Header, columnCount)
	for i := range block.Rows {
		block.Rows[i] = normalizeTableRow(block.Rows[i], columnCount)
	}
	return block, true
}

func collectTableCells(row gast.Node, source []byte) []string {
	var cells []string
	for cell := row.FirstChild(); cell != nil; cell = cell.NextSibling() {
		cells = append(cells, cleanBlockText(inlineText(cell, source)))
	}
	return cells
}

func tableAlignments(alignments []extast.Alignment) []document.TextAlign {
	result := make([]document.TextAlign, 0, len(alignments))
	for _, alignment := range alignments {
		switch alignment {
		case extast.AlignCenter:
			result = append(result, document.AlignCenter)
		case extast.AlignRight:
			result = append(result, document.AlignRight)
		default:
			result = append(result, document.AlignLeft)
		}
	}
	return result
}

func maxTableColumns(header []string, rows [][]string) int {
	max := len(header)
	for _, row := range rows {
		if len(row) > max {
			max = len(row)
		}
	}
	return max
}

func normalizeTableRow(row []string, columns int) []string {
	if len(row) == columns {
		return row
	}
	normalized := make([]string, columns)
	copy(normalized, row)
	return normalized
}

func inlineText(node gast.Node, source []byte) string {
	var builder strings.Builder
	appendInlineText(&builder, node, source)
	return builder.String()
}

func appendInlineText(builder *strings.Builder, node gast.Node, source []byte) {
	switch n := node.(type) {
	case *gast.Text:
		builder.Write(n.Value(source))
		if n.HardLineBreak() {
			builder.WriteByte('\n')
		} else if n.SoftLineBreak() {
			builder.WriteByte(' ')
		}
	case *gast.String:
		builder.Write(n.Value)
	case *gast.AutoLink:
		builder.Write(n.Label(source))
	default:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			appendInlineText(builder, child, source)
		}
	}
}

func cleanBlockText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		lines[i] = strings.TrimSpace(line)
	}
	return strings.TrimSpace(strings.Join(lines, " "))
}

func tableColumnWidths(header []string, rows [][]string) []float64 {
	columns := maxTableColumns(header, rows)
	if columns == 0 {
		return nil
	}
	weights := make([]float64, columns)
	for i := range weights {
		weights[i] = 1
	}
	accumulateWidths(weights, header)
	for _, row := range rows {
		accumulateWidths(weights, row)
	}
	total := 0.0
	for _, weight := range weights {
		total += weight
	}
	widths := make([]float64, columns)
	for i, weight := range weights {
		widths[i] = weight / total * 100
		if widths[i] < 8 {
			widths[i] = 8
		}
	}
	return normalizePercentages(widths)
}

func accumulateWidths(weights []float64, row []string) {
	for i, cell := range row {
		if i >= len(weights) {
			return
		}
		width := float64(displayWidth(cell))
		if width > weights[i] {
			weights[i] = width
		}
	}
}

func normalizePercentages(values []float64) []float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	if total == 0 {
		return values
	}
	for i := range values {
		values[i] = values[i] / total * 100
	}
	return values
}

func displayWidth(text string) int {
	width := 0
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) || unicode.Is(unicode.Hangul, r) {
			width += 2
			continue
		}
		width++
	}
	return width
}
