package reportpdf

import (
	"fmt"

	"github.com/gpdf-dev/gpdf/document"
	"github.com/gpdf-dev/gpdf/pdf"
	"github.com/gpdf-dev/gpdf/template"
)

const (
	regularFontFamily = "NotoSansSC"
	boldFontFamily    = "NotoSansSC-Bold"
)

func renderPDFBytes(reportText string, fonts bundledFonts) ([]byte, error) {
	doc := template.New(
		template.WithPageSize(document.A4),
		template.WithMargins(document.UniformEdges(document.Mm(18))),
		template.WithFont(regularFontFamily, fonts.Regular),
		template.WithFont(boldFontFamily, fonts.Bold),
		template.WithDefaultFont(regularFontFamily, 10.5),
		template.WithMetadata(document.DocumentMetadata{
			Title:   "企业发票核验分析报告",
			Creator: "invoice-verify-analysis report-pdf",
		}),
	)

	page := doc.AddPage()
	renderBlocks(page, parseMarkdownBlocks(reportText))
	payload, err := doc.Generate()
	if err != nil {
		return nil, fmt.Errorf("PDF生成失败：%w", err)
	}
	return payload, nil
}

func renderBlocks(page *template.PageBuilder, blocks []pdfBlock) {
	for _, block := range blocks {
		current := block
		page.AutoRow(func(row *template.RowBuilder) {
			row.Col(12, func(col *template.ColBuilder) {
				renderBlock(col, current)
			})
		})
	}
}

func renderBlock(col *template.ColBuilder, block pdfBlock) {
	switch block.Kind {
	case pdfBlockHeading:
		col.Text(block.Text, headingOptions(block.Level)...)
		col.Spacer(document.Mm(2.2))
	case pdfBlockParagraph:
		col.Text(block.Text, template.FontSize(10.5))
		col.Spacer(document.Mm(1.7))
	case pdfBlockList:
		if block.Ordered {
			col.OrderedList(block.Items, template.ListIndent(document.Mm(5)))
		} else {
			col.List(block.Items, template.ListIndent(document.Mm(5)))
		}
		col.Spacer(document.Mm(1.7))
	case pdfBlockTable:
		tableOptions := []template.TableOption{
			template.ColumnWidths(tableColumnWidths(block.Header, block.Rows)...),
			template.ColumnAlign(block.Aligns...),
			template.TableHeaderStyle(
				template.TextColor(pdf.White),
				template.BgColor(pdf.RGBHex(0x244C7A)),
			),
			template.TableStripe(pdf.RGBHex(0xF4F7FB)),
			template.WithTableBorderCollapse(true),
		}
		col.Table(block.Header, block.Rows, tableOptions...)
		col.Spacer(document.Mm(2.4))
	case pdfBlockRule:
		col.Line(template.LineColor(pdf.RGBHex(0xD0D7E2)), template.LineThickness(document.Pt(0.8)))
		col.Spacer(document.Mm(2))
	case pdfBlockCode:
		col.Text(block.Text, template.FontSize(9), template.BgColor(pdf.RGBHex(0xF5F5F5)))
		col.Spacer(document.Mm(1.7))
	}
}

func headingOptions(level int) []template.TextOption {
	size := 12.0
	switch level {
	case 1:
		size = 20
	case 2:
		size = 16
	case 3:
		size = 13.5
	case 4:
		size = 12
	}
	return []template.TextOption{
		template.FontSize(size),
		template.Bold(),
		template.TextColor(pdf.RGBHex(0x1A237E)),
	}
}
