package report

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/signintech/gopdf"
)

const (
	pdfPageWidth               = 595.0
	pdfPageHeight              = 842.0
	pdfMargin                  = 40.0
	pdfContentTop              = 72.0
	pdfContentEnd              = 790.0
	pdfRegularFontFamily       = "TAXANA+NotoSansSC"
	pdfBoldFontFamily          = "TAXBOL+NotoSansSCBold"
	pdfSubsectionHeadingHeight = 42.0
	pdfBlockTitleHeight        = 22.0
	pdfParagraphOpeningHeight  = 20.0
	pdfParagraphLineHeight     = 15.0
	pdfParagraphBottomSpacing  = 5.0
	pdfChartOpeningHeight      = 224.0
	pdfTableHeaderHeight       = 22.0
	pdfTableLineHeight         = 13.0
	pdfTableCellPadding        = 10.0
	pdfRiskBandHeight          = 22.0
	pdfRiskBandSpacing         = 6.0
	pdfTOCChapterEntryHeight   = 40.0
	pdfTOCSectionEntryHeight   = 27.0
	pdfTOCFirstEntryY          = 84.0
)

type pdfRenderer struct {
	pdf             *gopdf.GoPdf
	model           Model
	charts          map[string][]byte
	pageRefs        map[string]int
	sectionPages    map[string]int
	page            int
	y               float64
	withLinks       bool
	assets          ReportAssets
	companyFontSize float64
}

// WritePDF renders the same report model used by WriteDOCX. The first layout
// pass records section pages; the second pass writes exact TOC page numbers and
// internal links.
func WritePDF(path string, model Model, charts map[string][]byte, fonts FontFiles) error {
	sectionPages, err := layoutPDFSectionPages(model, charts, fonts)
	if err != nil {
		return err
	}
	return writePDFWithSectionPages(path, model, charts, fonts, sectionPages)
}

// layoutPDFSectionPages performs the first deterministic PDF layout pass.
// Parameters are the shared model, chart images, and packaged fonts. The
// return value maps stable bookmarks to absolute PDF pages.
func layoutPDFSectionPages(model Model, charts map[string][]byte, fonts FontFiles) (map[string]int, error) {
	assets, err := LoadReportAssets(fonts.Directory)
	if err != nil {
		return nil, err
	}
	first, err := newPDFRenderer(model, charts, fonts, assets, nil, false)
	if err != nil {
		return nil, err
	}
	if err := first.render(); err != nil {
		return nil, fmt.Errorf("PDF first layout pass: %w", err)
	}
	pages := make(map[string]int, len(first.sectionPages))
	for bookmark, page := range first.sectionPages {
		pages[bookmark] = page
	}
	return pages, nil
}

// writePDFWithSectionPages renders the linked second pass from a previously
// measured page map. Parameters match WritePDF plus absolute sectionPages. It
// returns an error if pagination changes or the final PDF is invalid.
func writePDFWithSectionPages(path string, model Model, charts map[string][]byte, fonts FontFiles, sectionPages map[string]int) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("PDF output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	assets, err := LoadReportAssets(fonts.Directory)
	if err != nil {
		return err
	}
	second, err := newPDFRenderer(model, charts, fonts, assets, sectionPages, true)
	if err != nil {
		return err
	}
	if err := second.render(); err != nil {
		return fmt.Errorf("PDF second layout pass: %w", err)
	}
	if len(sectionPages) != len(second.sectionPages) {
		return fmt.Errorf("PDF layout changed between passes")
	}
	for bookmark, page := range sectionPages {
		if second.sectionPages[bookmark] != page {
			return fmt.Errorf("PDF layout changed between passes at %s", bookmark)
		}
	}
	if err := second.pdf.WritePdf(path); err != nil {
		return err
	}
	return validatePDF(path)
}

// logicalSectionPages converts absolute PDF pages to body-page numbers shared
// with the Word section, whose visible pagination restarts at one. Parameter
// pages is keyed by bookmark; the returned map is an independent copy.
func logicalSectionPages(pages map[string]int) map[string]int {
	firstBodyPage := 0
	for _, page := range pages {
		if firstBodyPage == 0 || page < firstBodyPage {
			firstBodyPage = page
		}
	}
	result := make(map[string]int, len(pages))
	for bookmark, page := range pages {
		if firstBodyPage == 0 || page < firstBodyPage {
			result[bookmark] = page
			continue
		}
		result[bookmark] = page - firstBodyPage + 1
	}
	return result
}

func newPDFRenderer(model Model, charts map[string][]byte, fonts FontFiles, assets ReportAssets, pageRefs map[string]int, withLinks bool) (*pdfRenderer, error) {
	document := &gopdf.GoPdf{}
	document.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	document.SetInfo(gopdf.PdfInfo{
		Title:    model.CompanyName + "财税风险分析报告",
		Author:   "tax-analysis",
		Subject:  reportDocumentSubject(model),
		Creator:  "tax-analysis 0.1.1",
		Producer: "tax-analysis 0.1.1",
	})
	if err := document.AddTTFFontData(pdfRegularFontFamily, fonts.Regular); err != nil {
		return nil, fmt.Errorf("load PDF regular font: %w", err)
	}
	if err := document.AddTTFFontData(pdfBoldFontFamily, fonts.Bold); err != nil {
		return nil, fmt.Errorf("load PDF bold font: %w", err)
	}
	return &pdfRenderer{
		pdf:             document,
		model:           model,
		charts:          charts,
		pageRefs:        pageRefs,
		sectionPages:    map[string]int{},
		withLinks:       withLinks,
		assets:          assets,
		companyFontSize: fitCoverCompanyFontSize(fonts.Regular, model.CompanyName),
	}, nil
}

func (renderer *pdfRenderer) render() error {
	if err := renderer.renderCover(); err != nil {
		return fmt.Errorf("render cover: %w", err)
	}
	if err := renderer.renderTOC(); err != nil {
		return fmt.Errorf("render TOC: %w", err)
	}
	for index, section := range renderer.model.Sections {
		if index == 0 {
			renderer.addContentPage()
		} else {
			openingHeight, err := renderer.subsectionOpeningHeight(section)
			if err != nil {
				return fmt.Errorf("measure section %s opening: %w", section.ID, err)
			}
			// Let short chapter endings share a page with the next chapter, while
			// keeping its title and the first subsection's opening together.
			if section.Level == 1 && index+1 < len(renderer.model.Sections) && renderer.model.Sections[index+1].Level > 1 {
				nextHeight, nextErr := renderer.subsectionOpeningHeight(renderer.model.Sections[index+1])
				if nextErr != nil {
					return nextErr
				}
				openingHeight += nextHeight
			}
			if err := renderer.ensureSpace(openingHeight); err != nil {
				return err
			}
		}
		if err := renderer.renderHeading(section); err != nil {
			return fmt.Errorf("render section %s heading: %w", section.ID, err)
		}
		for blockIndex := 0; blockIndex < len(section.Blocks); blockIndex++ {
			block := section.Blocks[blockIndex]
			if isAgentNarrativeBlock(block) {
				requiredHeight, measureErr := renderer.agentNarrativeBlockHeight(block)
				if measureErr != nil {
					return fmt.Errorf("measure section %s block %d: %w", section.ID, blockIndex, measureErr)
				}
				if requiredHeight <= pdfContentEnd-pdfContentTop {
					if err := renderer.ensureSpace(requiredHeight); err != nil {
						return err
					}
				}
			}
			if err := renderer.renderBlock(block); err != nil {
				return fmt.Errorf("render section %s block %d (%s): %w", section.ID, blockIndex, block.Kind, err)
			}
		}
	}
	return nil
}

// agentNarrativeBlockHeight measures one validated Agent analysis block.
// Parameter block contains one to three concise paragraphs. Return values are
// the required PDF height and any font-layout error.
func (renderer *pdfRenderer) agentNarrativeBlockHeight(block Block) (float64, error) {
	if err := renderer.setFont(false, 9, reportInkColor.R, reportInkColor.G, reportInkColor.B); err != nil {
		return 0, err
	}
	height := 0.0
	if block.Title != "" {
		height += pdfBlockTitleHeight
	}
	for _, paragraph := range block.Paragraphs {
		lines, err := renderer.splitParagraphText(paragraph)
		if err != nil {
			return 0, err
		}
		lineCount := len(lines)
		if lineCount == 0 {
			lineCount = 1
		}
		height += float64(lineCount)*pdfParagraphLineHeight + pdfParagraphBottomSpacing
	}
	return height, nil
}

func (renderer *pdfRenderer) renderCover() error {
	renderer.pdf.AddPage()
	renderer.page++
	holder, err := gopdf.ImageHolderByBytes(renderer.assets.CoverBackground)
	if err != nil {
		return err
	}
	if err := renderer.pdf.ImageByHolder(holder, 0, 0, &gopdf.Rect{W: pdfPageWidth, H: pdfPageHeight}); err != nil {
		return err
	}
	if err := renderer.setFont(true, 42.5, 255, 255, 255); err != nil {
		return err
	}
	renderer.pdf.SetXY(50, 204)
	if err := renderer.pdf.Cell(nil, "税务风险分析报告"); err != nil {
		return err
	}
	renderer.pdf.SetStrokeColor(85, 165, 255)
	renderer.pdf.SetLineWidth(2.5)
	renderer.pdf.Line(50, 268, 411, 268)
	if err := renderer.setFont(false, renderer.companyFontSize, 255, 255, 255); err != nil {
		return err
	}
	renderer.pdf.SetXY(50, 292)
	if err := renderer.pdf.MultiCellWithOption(&gopdf.Rect{W: 360, H: 70}, renderer.model.CompanyName, gopdf.CellOption{Align: gopdf.Left, CoefLineHeight: 1.35}); err != nil {
		return err
	}
	if err := renderer.setFont(false, 15, 242, 245, 250); err != nil {
		return err
	}
	period := fmt.Sprintf("报告期间：  %s 至 %s", renderer.model.StartTime, renderer.model.EndTime)
	renderer.pdf.SetXY(50, 378)
	if err := renderer.pdf.Cell(nil, period); err != nil {
		return err
	}
	renderer.pdf.SetXY(50, 405)
	if err := renderer.pdf.Cell(nil, "报告生成时间：  "+renderer.model.GeneratedAt.Format("2006-01-02")); err != nil {
		return err
	}
	if err := renderer.setFont(false, 11.5, 242, 245, 250); err != nil {
		return err
	}
	renderer.pdf.SetXY(148, 706)
	if err := renderer.pdf.MultiCellWithOption(&gopdf.Rect{W: 380, H: 82}, "报告声明：\n本报告依据企业授权取得的财税信息，列示主要事实及需要关注的事项。\n报告结论限于所列分析期间及已取得数据。", gopdf.CellOption{CoefLineHeight: 1.55}); err != nil {
		return err
	}
	return nil
}

func (renderer *pdfRenderer) renderTOC() error {
	if err := renderer.addTOCPage(false); err != nil {
		return err
	}
	pageTarget, balancedBreaks := balancedTOCPageTarget(renderer.model.Sections)
	pageUsed := 0.0
	for _, section := range renderer.model.Sections {
		entryHeight := pdfTOCChapterEntryHeight
		fontSize := 14.0
		x := 50.0
		color := [3]uint8{reportInkColor.R, reportInkColor.G, reportInkColor.B}
		if section.Level > 1 {
			entryHeight = pdfTOCSectionEntryHeight
			fontSize = 12
			x = 75
			color = [3]uint8{reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B}
		}
		if balancedBreaks > 0 && pageUsed > 0 && pageUsed+entryHeight > pageTarget {
			if err := renderer.addTOCPage(true); err != nil {
				return err
			}
			balancedBreaks--
			pageUsed = 0
		}
		if renderer.y+entryHeight > 805 {
			if err := renderer.addTOCPage(true); err != nil {
				return err
			}
			pageUsed = 0
		}
		if err := renderer.setFont(section.Level == 1, fontSize, color[0], color[1], color[2]); err != nil {
			return err
		}
		pageText := ""
		if page, ok := renderer.pageRefs[section.Bookmark]; ok && section.Level > 1 {
			pageText = fmt.Sprintf("%d", renderer.logicalBodyPage(page))
		}
		title := section.Title
		pageWidth := 0.0
		if pageText != "" {
			pageWidth, _ = renderer.pdf.MeasureTextWidth(pageText)
		}
		maxTitleWidth := pdfPageWidth - x - 72 - pageWidth
		lines, err := renderer.pdf.SplitText(title, maxTitleWidth)
		if err != nil {
			return err
		}
		if len(lines) == 0 {
			lines = []string{title}
		}
		lineHeight := fontSize + 5
		minimumHeight := float64(len(lines))*lineHeight + 8
		if minimumHeight > entryHeight {
			entryHeight = minimumHeight
		}
		renderer.pdf.SetXY(x, renderer.y)
		if err := renderer.pdf.MultiCellWithOption(&gopdf.Rect{W: maxTitleWidth, H: entryHeight}, strings.Join(lines, "\n"), gopdf.CellOption{CoefLineHeight: 1.25}); err != nil {
			return err
		}
		if pageText != "" {
			renderer.pdf.SetXY(pdfPageWidth-48-pageWidth, renderer.y)
			if err := renderer.pdf.Cell(nil, pageText); err != nil {
				return err
			}
		}
		if section.Level > 1 {
			firstLineWidth, _ := renderer.pdf.MeasureTextWidth(lines[0])
			leaderStart := x + firstLineWidth + 12
			leaderEnd := pdfPageWidth - 58 - pageWidth
			renderer.pdf.SetStrokeColor(reportTOCLeaderColor.R, reportTOCLeaderColor.G, reportTOCLeaderColor.B)
			renderer.pdf.SetLineWidth(0.45)
			for dot := leaderStart; dot < leaderEnd; dot += 4.0 {
				renderer.pdf.Line(dot, renderer.y+fontSize+2, dot+0.9, renderer.y+fontSize+2)
			}
			renderer.pdf.SetStrokeColor(reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B)
			renderer.pdf.SetLineWidth(0.6)
			renderer.pdf.Line(x, renderer.y+fontSize+2, x+firstLineWidth, renderer.y+fontSize+2)
		}
		if renderer.withLinks {
			renderer.pdf.AddInternalLink(section.Bookmark, x, renderer.y-2, pdfPageWidth-48-x, entryHeight)
		}
		renderer.y += entryHeight
		pageUsed += entryHeight
	}
	return nil
}

// balancedTOCPageTarget calculates a stable target height for each TOC page.
// Parameter sections is the finalized dynamic section tree. Return values are
// the target used height and the number of planned page breaks; rendering still
// retains the hard bottom-boundary check for unusually long wrapped titles.
func balancedTOCPageTarget(sections []Section) (float64, int) {
	availableHeight := 805.0 - pdfTOCFirstEntryY
	totalHeight := 0.0
	for _, section := range sections {
		if section.Level > 1 {
			totalHeight += pdfTOCSectionEntryHeight
			continue
		}
		totalHeight += pdfTOCChapterEntryHeight
	}
	pageCount := 1
	for totalHeight > availableHeight*float64(pageCount) {
		pageCount++
	}
	if totalHeight == 0 {
		return availableHeight, 0
	}
	return totalHeight / float64(pageCount), pageCount - 1
}

func (renderer *pdfRenderer) addTOCPage(continuation bool) error {
	renderer.pdf.AddPage()
	renderer.page++
	renderer.y = 40
	if err := renderer.setFont(true, 21.5, 36, 42, 51); err != nil {
		return err
	}
	title := "目录"
	if continuation {
		title = "目录（续）"
	}
	renderer.pdf.SetXY(50, renderer.y)
	if err := renderer.pdf.Cell(nil, title); err != nil {
		return err
	}
	renderer.y = pdfTOCFirstEntryY
	return nil
}

func (renderer *pdfRenderer) renderHeading(section Section) error {
	if section.Level == 1 {
		renderer.pdf.SetFillColor(reportChapterFillColor.R, reportChapterFillColor.G, reportChapterFillColor.B)
		renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin, renderer.y-4, pdfPageWidth-2*pdfMargin, 26, "F")
		renderer.pdf.SetFillColor(reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B)
		renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin, renderer.y-4, 4, 26, "F")
		if err := renderer.setFont(true, 14, reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B); err != nil {
			return err
		}
		renderer.pdf.SetXY(pdfMargin+10, renderer.y)
		// SetAnchor reads gopdf's current text position and font, not renderer.y.
		// Record both destinations only after the heading's layout is final.
		renderer.sectionPages[section.Bookmark] = renderer.page
		renderer.pdf.SetAnchor(section.Bookmark)
		if err := renderer.pdf.MultiCell(&gopdf.Rect{W: pdfPageWidth - 2*pdfMargin, H: 44}, section.Title); err != nil {
			return err
		}
		renderer.y += 48
		return nil
	}
	if err := renderer.ensureSpace(45); err != nil {
		return err
	}
	headingY := renderer.y
	// Figma subsection component: an offset pale square, a solid blue square,
	// a thin underline, and a pale terminal square. It intentionally has no
	// full-width background fill.
	renderer.pdf.SetStrokeColor(reportSubheadingLineColor.R, reportSubheadingLineColor.G, reportSubheadingLineColor.B)
	renderer.pdf.SetLineWidth(1)
	renderer.pdf.Line(pdfMargin, headingY+23, pdfPageWidth-pdfMargin, headingY+23)
	renderer.pdf.SetFillColor(reportPaleBlueColor.R, reportPaleBlueColor.G, reportPaleBlueColor.B)
	renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin+4, headingY, 13, 13, "F")
	renderer.pdf.SetFillColor(reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B)
	renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin, headingY+5, 14, 14, "F")
	renderer.pdf.SetFillColor(reportPaleBlueColor.R, reportPaleBlueColor.G, reportPaleBlueColor.B)
	renderer.pdf.RectFromUpperLeftWithStyle(pdfPageWidth-pdfMargin-9, headingY+4.5, 9, 8.5, "F")
	if err := renderer.setFont(true, 12, reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B); err != nil {
		return err
	}
	renderer.pdf.SetXY(pdfMargin+25, headingY+3.5)
	renderer.sectionPages[section.Bookmark] = renderer.page
	renderer.pdf.SetAnchor(section.Bookmark)
	if err := renderer.pdf.Cell(nil, section.Title); err != nil {
		return err
	}
	renderer.y += pdfSubsectionHeadingHeight
	return nil
}

// subsectionOpeningHeight calculates the minimum vertical space required to
// keep a subsection heading with the first visible part of its content.
// Parameter section is the finalized report subsection. The return values are
// the required height in PDF points and any text-layout error.
func (renderer *pdfRenderer) subsectionOpeningHeight(section Section) (float64, error) {
	height := pdfSubsectionHeadingHeight
	if section.Level == 1 {
		height = 48
	}
	if len(section.Blocks) == 0 {
		return height, nil
	}

	// A grade strip is a prefix to the content, not the content opening.
	// Reserve the first chart/table opening too so neither title nor grade is
	// left at the foot of the preceding page.
	firstContent := 0
	for firstContent < len(section.Blocks) && section.Blocks[firstContent].Kind == BlockRiskBand {
		height += pdfRiskBandHeight + pdfRiskBandSpacing
		firstContent++
	}
	if firstContent == len(section.Blocks) {
		return height, nil
	}
	block := section.Blocks[firstContent]
	if section.Key == "chapter.conclusion" && isAgentNarrativeBlock(block) {
		bodyHeight, err := renderer.agentNarrativeBlockHeight(block)
		if err != nil {
			return 0, err
		}
		if height+bodyHeight <= pdfContentEnd-pdfContentTop {
			return height + bodyHeight, nil
		}
	}
	if block.Title != "" && block.Kind != BlockRiskCard && block.Kind != BlockChart {
		height += pdfBlockTitleHeight
	}
	if len(block.Paragraphs) > 0 && block.Kind != BlockRiskCard {
		if block.Kind == BlockChart {
			return height + pdfParagraphOpeningHeight + pdfChartOpeningHeight, nil
		}
		return height + pdfParagraphOpeningHeight, nil
	}

	switch block.Kind {
	case BlockTable:
		if len(block.Headers) == 0 {
			return height, nil
		}
		if len(block.Rows) == 0 {
			return height + pdfTableHeaderHeight, nil
		}
		openingHeight, err := renderer.tableOpeningHeight(block.Headers, block.Rows, block.TableVariant, false)
		return height + openingHeight, err
	case BlockRiskCard:
		if len(block.Paragraphs) == 0 {
			return height, nil
		}
		firstRow := []string{block.Title, block.Paragraphs[0]}
		openingHeight, err := renderer.tableOpeningHeight([]string{"风险评价", "结果"}, [][]string{firstRow}, TableVariantDefault, false)
		return height + openingHeight, err
	case BlockChart:
		return height + pdfChartOpeningHeight, nil
	default:
		return height, nil
	}
}

func (renderer *pdfRenderer) renderBlock(block Block) error {
	if block.Title != "" && block.Kind != BlockRiskCard && block.Kind != BlockRiskBand && block.Kind != BlockChart {
		requiredHeight := 28.0
		if block.Kind == BlockTable && len(block.Headers) > 0 && len(block.Rows) > 0 {
			openingHeight, err := renderer.tableOpeningHeight(block.Headers, block.Rows, block.TableVariant, true)
			if err != nil {
				return err
			}
			requiredHeight = openingHeight
		} else if len(block.Paragraphs) > 0 {
			// Keep generic titles, including script-owned recommendations, with
			// the first line without forcing the whole paragraph onto one page.
			requiredHeight = pdfBlockTitleHeight + pdfParagraphOpeningHeight
		}
		if err := renderer.ensureSpace(requiredHeight); err != nil {
			return err
		}
		r, g, b := uint8(49), uint8(92), uint8(140)
		if block.Title == "数据说明" {
			r, g, b = 102, 112, 133
		}
		if err := renderer.setFont(true, 10, r, g, b); err != nil {
			return err
		}
		renderer.pdf.SetXY(pdfMargin, renderer.y)
		if err := renderer.pdf.Cell(nil, block.Title); err != nil {
			return err
		}
		renderer.y += pdfBlockTitleHeight
	}
	if block.Kind != BlockRiskBand && block.Kind != BlockRiskCard {
		for _, paragraph := range block.Paragraphs {
			var err error
			if block.Title == "数据说明" {
				err = renderer.renderParagraphColor(paragraph, 102, 112, 133)
			} else {
				err = renderer.renderParagraph(paragraph)
			}
			if err != nil {
				return err
			}
		}
	}
	switch block.Kind {
	case BlockTable:
		return renderer.renderTable(block)
	case BlockRiskCard:
		return renderer.renderRiskSummaryCard(block)
	case BlockRiskBand:
		return renderer.renderRiskBand(block)
	case BlockChart:
		return renderer.renderChart(block)
	default:
		return nil
	}
}

// renderRiskBand draws the bordered pastel 1040-by-44 Figma warning strip. Parameter
// block supplies a confirmed level in Title and trusted copy in Paragraphs.
// The return value is a layout or font error.
func (renderer *pdfRenderer) renderRiskBand(block Block) error {
	palette, ok := riskBandStyle(RiskLevel(block.Title))
	if !ok || len(block.Paragraphs) == 0 {
		return nil
	}
	if err := renderer.ensureSpace(pdfRiskBandHeight + pdfRiskBandSpacing); err != nil {
		return err
	}
	renderer.pdf.SetFillColor(palette.Fill.R, palette.Fill.G, palette.Fill.B)
	renderer.pdf.SetStrokeColor(palette.Border.R, palette.Border.G, palette.Border.B)
	renderer.pdf.SetLineWidth(0.5)
	renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin, renderer.y, pdfPageWidth-2*pdfMargin, pdfRiskBandHeight, "DF")
	if err := renderer.setFont(true, 9, palette.Text.R, palette.Text.G, palette.Text.B); err != nil {
		return err
	}
	renderer.pdf.SetXY(pdfMargin+7, renderer.y+7)
	if err := renderer.pdf.Cell(&gopdf.Rect{W: pdfPageWidth - 2*pdfMargin - 14, H: 12}, block.Paragraphs[0]); err != nil {
		return err
	}
	renderer.y += pdfRiskBandHeight + pdfRiskBandSpacing
	return nil
}

// renderRiskSummaryCard draws the same three-column summary used by DOCX.
// Parameter block supplies high/medium/low counts and trusted summary text.
// Return value is a layout or font error.
func (renderer *pdfRenderer) renderRiskSummaryCard(block Block) error {
	const cardHeight = 124.0
	if err := renderer.ensureSpace(cardHeight); err != nil {
		return err
	}
	labels := []string{string(RiskHigh), string(RiskMedium), string(RiskLow)}
	counts := []string{"0", "0", "0"}
	if len(block.Headers) == len(labels) {
		copy(labels, block.Headers)
	}
	if len(block.Rows) > 0 {
		copy(counts, block.Rows[0])
	}
	styles := make([]riskTablePalette, len(labels))
	for index, label := range labels {
		styles[index], _ = riskTableStyle(label)
	}
	cardWidth := (pdfPageWidth - 2*pdfMargin) / 3
	for index := range labels {
		x := pdfMargin + float64(index)*cardWidth
		renderer.pdf.SetFillColor(styles[index].Fill.R, styles[index].Fill.G, styles[index].Fill.B)
		renderer.pdf.SetStrokeColor(reportTableGridColor.R, reportTableGridColor.G, reportTableGridColor.B)
		renderer.pdf.SetLineWidth(0.5)
		renderer.pdf.RectFromUpperLeftWithStyle(x, renderer.y, cardWidth, 70, "DF")
		if err := renderer.setFont(true, 16, styles[index].Text.R, styles[index].Text.G, styles[index].Text.B); err != nil {
			return err
		}
		text := labels[index] + "  " + counts[index]
		width, _ := renderer.pdf.MeasureTextWidth(text)
		renderer.pdf.SetXY(x+(cardWidth-width)/2, renderer.y+25)
		if err := renderer.pdf.Cell(nil, text); err != nil {
			return err
		}
	}
	overall := "未见明显异常"
	summary := ""
	if len(block.Paragraphs) > 0 {
		overall = block.Paragraphs[0]
	}
	if len(block.Paragraphs) > 1 {
		summary = block.Paragraphs[1]
	}
	renderer.pdf.SetFillColor(reportOverallRiskFill.R, reportOverallRiskFill.G, reportOverallRiskFill.B)
	renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin, renderer.y+79, pdfPageWidth-2*pdfMargin, 35, "F")
	renderer.pdf.SetFillColor(248, 102, 2)
	renderer.pdf.RectFromUpperLeftWithStyle(pdfMargin, renderer.y+79, 3, 35, "F")
	if err := renderer.setFont(false, 8.5, reportOverallRiskText.R, reportOverallRiskText.G, reportOverallRiskText.B); err != nil {
		return err
	}
	text := "综合风险评价：" + overall + "；" + summary
	renderer.pdf.SetXY(pdfMargin+10, renderer.y+89)
	if err := renderer.pdf.MultiCellWithOption(&gopdf.Rect{W: pdfPageWidth - 2*pdfMargin - 20, H: 28}, text, gopdf.CellOption{CoefLineHeight: 1.25}); err != nil {
		return err
	}
	renderer.y += cardHeight
	return nil
}

func (renderer *pdfRenderer) renderParagraph(value string) error {
	return renderer.renderParagraphColor(value, 36, 42, 51)
}

func (renderer *pdfRenderer) renderParagraphColor(value string, r, g, b uint8) error {
	if err := renderer.setFont(false, 9, r, g, b); err != nil {
		return err
	}
	value = textOr(value, "—")
	lines, err := renderer.splitParagraphText(value)
	if err != nil {
		return err
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	for _, line := range lines {
		if err := renderer.ensureSpace(16); err != nil {
			return err
		}
		// A new page draws the header in a larger blue font. Restore the
		// paragraph style so the already wrapped line keeps its measured width.
		if err := renderer.setFont(false, 9, r, g, b); err != nil {
			return err
		}
		renderer.pdf.SetXY(pdfMargin, renderer.y)
		if err := renderer.pdf.Cell(nil, textOr(line, "—")); err != nil {
			return err
		}
		renderer.y += pdfParagraphLineHeight
	}
	renderer.y += pdfParagraphBottomSpacing
	return nil
}

// splitParagraphText keeps a trailing Chinese stop with preceding text when
// strict font-width wrapping would put only punctuation on the final line.
// The same split is used for block measurement and drawing.
func (renderer *pdfRenderer) splitParagraphText(value string) ([]string, error) {
	width := pdfPageWidth - 2*pdfMargin
	lines, err := splitNumericParagraph(value, width, renderer.pdf.MeasureTextWidth)
	if err != nil || len(lines) < 2 {
		return lines, err
	}
	last := len(lines) - 1
	tail := strings.TrimSpace(lines[last])
	if tail == "" || strings.Trim(tail, "。；！？…，、：）》】」』") != "" {
		return lines, nil
	}
	previous := []rune(lines[last-1])
	cut := len(previous)
	for cut > 1 && len(previous)-cut < 2 && unicode.Is(unicode.Han, previous[cut-1]) {
		cut--
	}
	if cut == len(previous) {
		return lines, nil
	}
	joined := string(previous[cut:]) + lines[last]
	measured, err := renderer.pdf.MeasureTextWidth(joined)
	if err != nil {
		return nil, err
	}
	if measured <= width {
		lines[last-1], lines[last] = string(previous[:cut]), joined
	}
	return lines, nil
}

// renderTable draws one table using the semantic Figma variant carried by the
// report model. Parameter block supplies headers, rows, and variant. It returns
// a layout or font error.
func (renderer *pdfRenderer) renderTable(block Block) error {
	headers, rows, variant := block.Headers, block.Rows, block.TableVariant
	if len(headers) == 0 {
		return nil
	}
	columnWidths := tableColumnWidths(headers, rows, pdfPageWidth-2*pdfMargin, variant)
	headerLines, headerHeight, err := renderer.tableHeaderLayout(headers, columnWidths)
	if err != nil {
		return err
	}
	riskColumn := riskLevelColumn(headers)
	columnX := make([]float64, len(headers))
	currentX := pdfMargin
	for index, width := range columnWidths {
		columnX[index] = currentX
		currentX += width
	}
	renderHeader := func() error {
		if err := renderer.ensureSpace(headerHeight + 2); err != nil {
			return err
		}
		if err := renderer.setFont(true, 8, reportTableTextColor.R, reportTableTextColor.G, reportTableTextColor.B); err != nil {
			return err
		}
		for index, lines := range headerLines {
			x, columnWidth := columnX[index], columnWidths[index]
			renderer.pdf.SetFillColor(reportTableHeaderColor.R, reportTableHeaderColor.G, reportTableHeaderColor.B)
			renderer.pdf.SetStrokeColor(reportTableBorderColor.R, reportTableBorderColor.G, reportTableBorderColor.B)
			renderer.pdf.SetLineWidth(0.5)
			renderer.pdf.RectFromUpperLeftWithStyle(x, renderer.y, columnWidth, headerHeight, "DF")
			for lineIndex, line := range lines {
				renderer.pdf.SetXY(x+5, renderer.y+6+float64(lineIndex)*pdfTableLineHeight)
				if err := renderer.pdf.Cell(&gopdf.Rect{W: columnWidth - 10, H: 14}, line); err != nil {
					return err
				}
			}
		}
		renderer.y += headerHeight
		return nil
	}
	measureRow := func(row []string) ([][]string, float64, error) {
		lineSets := make([][]string, len(headers))
		maxLines := 1
		if err := renderer.setFont(false, 8, reportTableTextColor.R, reportTableTextColor.G, reportTableTextColor.B); err != nil {
			return nil, 0, err
		}
		for column := range headers {
			value := ""
			if column < len(row) {
				value = row[column]
			}
			lines, err := renderer.splitTextWithBreaks(value, columnWidths[column]-10)
			if err != nil {
				return nil, 0, err
			}
			if len(lines) == 0 {
				lines = []string{""}
			}
			lineSets[column] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		return lineSets, float64(maxLines)*pdfTableLineHeight + pdfTableCellPadding, nil
	}
	if len(rows) > 0 {
		_, firstRowHeight, err := measureRow(rows[0])
		if err != nil {
			return err
		}
		if err := renderer.ensureSpace(headerHeight + firstRowHeight); err != nil {
			return err
		}
	}
	if err := renderHeader(); err != nil {
		return err
	}
	for rowIndex, row := range rows {
		lineSets, rowHeight, err := measureRow(row)
		if err != nil {
			return err
		}
		if renderer.y+rowHeight > pdfContentEnd {
			renderer.addContentPage()
			if err := renderHeader(); err != nil {
				return err
			}
		}
		// renderHeader switches to the header font. A body row that starts
		// on the next page must restore the normal body font and text color.
		if err := renderer.setFont(false, 8, reportTableTextColor.R, reportTableTextColor.G, reportTableTextColor.B); err != nil {
			return err
		}
		fill := [3]uint8{reportTableRowColor.R, reportTableRowColor.G, reportTableRowColor.B}
		if rowIndex%2 == 1 {
			fill = [3]uint8{reportTableAlternateColor.R, reportTableAlternateColor.G, reportTableAlternateColor.B}
		}
		if variant == TableVariantAttention {
			fill = [3]uint8{reportRiskAttentionColor.R, reportRiskAttentionColor.G, reportRiskAttentionColor.B}
		}
		for column, lines := range lineSets {
			x, columnWidth := columnX[column], columnWidths[column]
			cellFill := reportColor{R: fill[0], G: fill[1], B: fill[2]}
			textColor := reportTableTextColor
			bold := false
			centered := false
			var leftBorder *reportColor
			value := ""
			if column < len(row) {
				value = row[column]
			}
			if tableHasRiskColumnStyle(variant) && column == riskColumn {
				if palette, ok := riskTableStyle(value); ok {
					cellFill, textColor, bold = palette.Fill, palette.Text, true
					border := palette.LeftBorder
					leftBorder = &border
				}
			}
			renderer.pdf.SetFillColor(cellFill.R, cellFill.G, cellFill.B)
			renderer.pdf.SetStrokeColor(reportTableGridColor.R, reportTableGridColor.G, reportTableGridColor.B)
			renderer.pdf.RectFromUpperLeftWithStyle(x, renderer.y, columnWidth, rowHeight, "DF")
			if leftBorder != nil {
				renderer.pdf.SetFillColor(leftBorder.R, leftBorder.G, leftBorder.B)
				renderer.pdf.RectFromUpperLeftWithStyle(x, renderer.y, 3, rowHeight, "F")
			}
			if err := renderer.setFont(bold, 8, textColor.R, textColor.G, textColor.B); err != nil {
				return err
			}
			for lineIndex, line := range lines {
				textX := x + 5
				if centered {
					textWidth, _ := renderer.pdf.MeasureTextWidth(line)
					textX = x + (columnWidth-textWidth)/2
				}
				renderer.pdf.SetXY(textX, renderer.y+6+float64(lineIndex)*pdfTableLineHeight)
				if err := renderer.pdf.Cell(&gopdf.Rect{W: columnWidth - 10, H: 12}, textOr(line, "—")); err != nil {
					return err
				}
			}
		}
		renderer.y += rowHeight
	}
	renderer.y += 14
	return nil
}

// tableOpeningHeight calculates the minimum space required to keep a table
// title, header, and first row on one page. Parameters: headers and firstRow are
// the rendered table values; withTitle indicates whether a block title is
// present. Return values: required height and any font/text-layout error.
func (renderer *pdfRenderer) tableOpeningHeight(headers []string, rows [][]string, variant TableVariant, withTitle bool) (float64, error) {
	var firstRow []string
	if len(rows) > 0 {
		firstRow = rows[0]
	}
	columnWidths := tableColumnWidths(headers, rows, pdfPageWidth-2*pdfMargin, variant)
	_, headerHeight, err := renderer.tableHeaderLayout(headers, columnWidths)
	if err != nil {
		return 0, err
	}
	if err := renderer.setFont(false, 8, reportInkColor.R, reportInkColor.G, reportInkColor.B); err != nil {
		return 0, err
	}
	maxLines := 1
	for column := range headers {
		value := ""
		if column < len(firstRow) {
			value = firstRow[column]
		}
		lines, err := renderer.splitTextWithBreaks(value, columnWidths[column]-pdfTableCellPadding)
		if err != nil {
			return 0, err
		}
		if len(lines) > maxLines {
			maxLines = len(lines)
		}
	}
	height := headerHeight + float64(maxLines)*pdfTableLineHeight + pdfTableCellPadding
	if withTitle {
		height += pdfBlockTitleHeight
	}
	return height, nil
}

// tableHeaderLayout wraps long period/metric labels inside their own columns.
// Rendering and pagination use the same measured height, including repeated headers.
func (renderer *pdfRenderer) tableHeaderLayout(headers []string, widths []float64) ([][]string, float64, error) {
	if err := renderer.setFont(true, 8, reportTableTextColor.R, reportTableTextColor.G, reportTableTextColor.B); err != nil {
		return nil, 0, err
	}
	lineSets := make([][]string, len(headers))
	maxLines := 1
	for i, header := range headers {
		lines, err := renderer.splitTextWithBreaks(header, widths[i]-pdfTableCellPadding)
		if err != nil {
			return nil, 0, err
		}
		lineSets[i] = lines
		maxLines = max(maxLines, len(lines))
	}
	return lineSets, pdfTableHeaderHeight + float64(maxLines-1)*pdfTableLineHeight, nil
}

// splitTextWithBreaks wraps text while preserving explicit model line breaks.
// Parameters are visible text and maximum line width. Return values are the
// complete ordered lines and any PDF font-layout error.
func (renderer *pdfRenderer) splitTextWithBreaks(value string, width float64) ([]string, error) {
	result := make([]string, 0)
	for _, explicitLine := range strings.Split(value, "\n") {
		if explicitLine == "" {
			result = append(result, "")
			continue
		}
		lines, err := renderer.splitTableTokens(explicitLine, width)
		if err != nil {
			return nil, err
		}
		if len(lines) == 0 {
			lines = []string{""}
		}
		result = append(result, lines...)
	}
	if len(result) == 0 {
		result = []string{""}
	}
	return result, nil
}

// splitTableTokens wraps CJK text freely but keeps a numeric/date token intact
// whenever it fits the column; an overlong identifier uses the ordinary splitter.
func (renderer *pdfRenderer) splitTableTokens(value string, width float64) ([]string, error) {
	var tokens []string
	cursor := 0
	for _, match := range tableAtomicValuePattern.FindAllStringIndex(value, -1) {
		for _, r := range value[cursor:match[0]] {
			tokens = append(tokens, string(r))
		}
		tokens = append(tokens, value[match[0]:match[1]])
		cursor = match[1]
	}
	for _, r := range value[cursor:] {
		tokens = append(tokens, string(r))
	}
	var lines []string
	line := ""
	for _, token := range tokens {
		measure, err := renderer.pdf.MeasureTextWidth(line + token)
		if err != nil {
			return nil, err
		}
		if line != "" && measure > width {
			lines = append(lines, line)
			line = ""
		}
		tokenWidth, err := renderer.pdf.MeasureTextWidth(token)
		if err != nil {
			return nil, err
		}
		if tokenWidth > width {
			parts, err := renderer.pdf.SplitText(token, width)
			if err != nil {
				return nil, err
			}
			if len(parts) > 0 {
				lines = append(lines, parts[:len(parts)-1]...)
				line = parts[len(parts)-1]
			}
			continue
		}
		line += token
	}
	if line != "" {
		lines = append(lines, line)
	}
	return lines, nil
}

func (renderer *pdfRenderer) renderChart(block Block) error {
	if block.Chart == nil {
		return nil
	}
	payload := renderer.charts[block.Chart.ID]
	if len(payload) == 0 {
		return fmt.Errorf("missing chart payload %s", block.Chart.ID)
	}
	if err := renderer.ensureSpace(pdfChartOpeningHeight); err != nil {
		return err
	}
	holder, err := gopdf.ImageHolderByBytes(payload)
	if err != nil {
		return fmt.Errorf("decode chart %s image: %w", block.Chart.ID, err)
	}
	if err := renderer.pdf.ImageByHolder(holder, pdfMargin, renderer.y, &gopdf.Rect{W: pdfPageWidth - 2*pdfMargin, H: 210}); err != nil {
		return fmt.Errorf("draw chart %s image: %w", block.Chart.ID, err)
	}
	renderer.y += 218
	return nil
}

func (renderer *pdfRenderer) ensureSpace(height float64) error {
	if renderer.y+height <= pdfContentEnd {
		return nil
	}
	renderer.addContentPage()
	return nil
}

func (renderer *pdfRenderer) addContentPage() {
	renderer.pdf.AddPage()
	renderer.page++
	renderer.y = pdfContentTop
	renderer.pdf.SetFillColor(reportHeaderFillColor.R, reportHeaderFillColor.G, reportHeaderFillColor.B)
	renderer.pdf.RectFromUpperLeftWithStyle(20, 20, pdfPageWidth-40, 30, "F")
	renderer.pdf.SetStrokeColor(reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B)
	renderer.pdf.SetLineWidth(1.2)
	renderer.pdf.Line(20, 50, pdfPageWidth-20, 50)
	_ = renderer.setFont(false, 12, reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B)
	renderer.pdf.SetXY(40, 27.5)
	_ = renderer.pdf.Cell(nil, renderer.model.CompanyName)
	rightTitle := "财税风险分析报告"
	_ = renderer.setFont(false, 12, reportPrimaryBlueColor.R, reportPrimaryBlueColor.G, reportPrimaryBlueColor.B)
	rightWidth, _ := renderer.pdf.MeasureTextWidth(rightTitle)
	renderer.pdf.SetXY(pdfPageWidth-40-rightWidth, 27.5)
	_ = renderer.pdf.Cell(nil, rightTitle)
	pageText := fmt.Sprintf("%d", renderer.logicalBodyPage(renderer.page))
	width, _ := renderer.pdf.MeasureTextWidth(pageText)
	renderer.pdf.SetXY((pdfPageWidth-width)/2, 818)
	_ = renderer.pdf.Cell(nil, pageText)
}

// logicalBodyPage converts a physical PDF page to the Figma-visible body page
// number. Parameter physicalPage is the absolute PDF page. Return value starts
// at one for the first bookmarked report section.
func (renderer *pdfRenderer) logicalBodyPage(physicalPage int) int {
	firstBodyPage := 0
	for _, page := range renderer.pageRefs {
		if firstBodyPage == 0 || page < firstBodyPage {
			firstBodyPage = page
		}
	}
	if firstBodyPage == 0 {
		for _, page := range renderer.sectionPages {
			if firstBodyPage == 0 || page < firstBodyPage {
				firstBodyPage = page
			}
		}
	}
	if firstBodyPage == 0 || physicalPage < firstBodyPage {
		return physicalPage
	}
	return physicalPage - firstBodyPage + 1
}

func (renderer *pdfRenderer) setFont(bold bool, size float64, red, green, blue uint8) error {
	family := pdfRegularFontFamily
	if bold {
		family = pdfBoldFontFamily
	}
	if err := renderer.pdf.SetFont(family, "", size); err != nil {
		return err
	}
	renderer.pdf.SetTextColor(red, green, blue)
	return nil
}

func validatePDF(path string) error {
	payload, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if len(payload) < 1024 || !bytes.HasPrefix(payload, []byte("%PDF-")) {
		return fmt.Errorf("invalid PDF output")
	}
	if !bytes.Contains(payload, []byte("/FontFile2")) && !bytes.Contains(payload, []byte("/FontFile3")) {
		return fmt.Errorf("invalid PDF output: embedded font program is missing")
	}
	return nil
}
