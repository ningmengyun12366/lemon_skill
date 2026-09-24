package report

import (
	"archive/zip"
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type docxImage struct {
	ID       string
	RelID    string
	Filename string
	Payload  []byte
	Width    int64
	Height   int64
}

// WriteDOCX writes a standalone OOXML document with embedded fonts, stable
// section bookmarks, clickable TOC entries, and measured PAGEREF page fields.
// Parameters are the output path, finalized report model, rendered chart images,
// and packaged fonts. It returns a layout, generation, or package validation error.
func WriteDOCX(path string, model Model, charts map[string][]byte, fonts FontFiles) error {
	sectionPages, err := layoutPDFSectionPages(model, charts, fonts)
	if err != nil {
		return fmt.Errorf("measure DOCX table of contents pages: %w", err)
	}
	return writeDOCXWithPageRefs(path, model, charts, fonts, logicalSectionPages(sectionPages))
}

// writeDOCXWithPageRefs writes a DOCX with precomputed logical body-page
// values cached inside PAGEREF fields. Parameters match WriteDOCX; pageRefs is
// keyed by stable section bookmark. It returns a generation or validation error.
func writeDOCXWithPageRefs(path string, model Model, charts map[string][]byte, fonts FontFiles, pageRefs map[string]int) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("DOCX output path is required")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(file)
	defer func() {
		_ = writer.Close()
		_ = file.Close()
	}()

	assets, err := LoadReportAssets(fonts.Directory)
	if err != nil {
		return err
	}
	images := make([]docxImage, 0, len(charts)+1)
	images = append(images, docxImage{ID: "cover-background", RelID: "rIdImage1", Filename: coverBackgroundFilename, Payload: assets.CoverBackground, Width: 7_559_310, Height: 10_693_130})
	chartIDs := chartIDs(model)
	for index, id := range chartIDs {
		payload := charts[id]
		if len(payload) == 0 {
			return fmt.Errorf("missing chart payload %s", id)
		}
		images = append(images, docxImage{ID: id, RelID: fmt.Sprintf("rIdImage%d", index+2), Filename: fmt.Sprintf("chart-%02d.png", index+1), Payload: payload, Width: 5_943_600, Height: 2_476_500})
	}
	imageByID := map[string]docxImage{}
	for _, image := range images {
		imageByID[image.ID] = image
	}

	parts := map[string][]byte{
		"[Content_Types].xml":                 []byte(contentTypesXML()),
		"_rels/.rels":                         []byte(packageRelationshipsXML()),
		"docProps/app.xml":                    []byte(appPropertiesXML()),
		"docProps/core.xml":                   []byte(corePropertiesXML(model)),
		"word/document.xml":                   []byte(documentXML(model, imageByID, fonts, pageRefs)),
		"word/styles.xml":                     []byte(stylesXML()),
		"word/settings.xml":                   []byte(settingsXML()),
		"word/fontTable.xml":                  []byte(fontTableXML()),
		"word/_rels/fontTable.xml.rels":       []byte(fontTableRelationshipsXML()),
		"word/_rels/document.xml.rels":        []byte(documentRelationshipsXML(images)),
		"word/header1.xml":                    []byte(headerXML(model)),
		"word/footer1.xml":                    []byte(footerXML()),
		"word/theme/theme1.xml":               []byte(themeXML()),
		"word/fonts/NotoSansSC-Regular.odttf": obfuscateFont(fonts.Regular, regularFontKey),
		"word/fonts/NotoSansSC-Bold.odttf":    obfuscateFont(fonts.Bold, boldFontKey),
	}
	for _, image := range images {
		parts["word/media/"+image.Filename] = image.Payload
	}
	order := []string{
		"[Content_Types].xml", "_rels/.rels", "docProps/app.xml", "docProps/core.xml",
		"word/document.xml", "word/styles.xml", "word/settings.xml", "word/fontTable.xml",
		"word/_rels/fontTable.xml.rels", "word/_rels/document.xml.rels", "word/header1.xml",
		"word/footer1.xml", "word/theme/theme1.xml", "word/fonts/NotoSansSC-Regular.odttf",
		"word/fonts/NotoSansSC-Bold.odttf",
	}
	for _, image := range images {
		order = append(order, "word/media/"+image.Filename)
	}
	for _, name := range order {
		if err := writeZipPart(writer, name, parts[name]); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return validateDOCXPackage(path, len(chartIDs))
}

func writeZipPart(writer *zip.Writer, name string, payload []byte) error {
	header := &zip.FileHeader{Name: filepath.ToSlash(name), Method: zip.Deflate}
	header.SetModTime(time.Date(2026, 8, 25, 0, 0, 0, 0, time.UTC))
	part, err := writer.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = part.Write(payload)
	return err
}

func validateDOCXPackage(path string, expectedImages int) error {
	reader, err := zip.OpenReader(path)
	if err != nil {
		return err
	}
	defer reader.Close()
	required := map[string]bool{
		"[Content_Types].xml": false, "word/document.xml": false, "word/styles.xml": false,
		"word/fontTable.xml": false, "word/fonts/NotoSansSC-Regular.odttf": false,
		"word/fonts/NotoSansSC-Bold.odttf": false,
	}
	images := 0
	coverPresent := false
	for _, file := range reader.File {
		if _, ok := required[file.Name]; ok {
			required[file.Name] = true
		}
		if strings.HasPrefix(file.Name, "word/media/chart-") {
			images++
		}
		if file.Name == "word/media/"+coverBackgroundFilename {
			coverPresent = true
		}
	}
	for name, present := range required {
		if !present {
			return fmt.Errorf("invalid DOCX package: missing %s", name)
		}
	}
	if images != expectedImages {
		return fmt.Errorf("invalid DOCX package: expected %d chart images, got %d", expectedImages, images)
	}
	if !coverPresent {
		return errors.New("invalid DOCX package: Figma cover background is missing")
	}
	return nil
}

func contentTypesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="png" ContentType="image/png"/>
  <Default Extension="odttf" ContentType="application/vnd.openxmlformats-officedocument.obfuscatedFont"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
  <Override PartName="/word/styles.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.styles+xml"/>
  <Override PartName="/word/settings.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.settings+xml"/>
  <Override PartName="/word/fontTable.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.fontTable+xml"/>
  <Override PartName="/word/header1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.header+xml"/>
  <Override PartName="/word/footer1.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.footer+xml"/>
  <Override PartName="/word/theme/theme1.xml" ContentType="application/vnd.openxmlformats-officedocument.theme+xml"/>
  <Override PartName="/docProps/core.xml" ContentType="application/vnd.openxmlformats-package.core-properties+xml"/>
  <Override PartName="/docProps/app.xml" ContentType="application/vnd.openxmlformats-officedocument.extended-properties+xml"/>
</Types>`
}

func packageRelationshipsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
  <Relationship Id="rId2" Type="http://schemas.openxmlformats.org/package/2006/relationships/metadata/core-properties" Target="docProps/core.xml"/>
  <Relationship Id="rId3" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/extended-properties" Target="docProps/app.xml"/>
</Relationships>`
}

func documentRelationshipsXML(images []docxImage) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">`)
	builder.WriteString(`<Relationship Id="rIdStyles" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/styles" Target="styles.xml"/>`)
	builder.WriteString(`<Relationship Id="rIdSettings" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/settings" Target="settings.xml"/>`)
	builder.WriteString(`<Relationship Id="rIdFonts" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/fontTable" Target="fontTable.xml"/>`)
	builder.WriteString(`<Relationship Id="rIdTheme" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/theme" Target="theme/theme1.xml"/>`)
	builder.WriteString(`<Relationship Id="rIdHeader" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/header" Target="header1.xml"/>`)
	builder.WriteString(`<Relationship Id="rIdFooter" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/footer" Target="footer1.xml"/>`)
	for _, image := range images {
		builder.WriteString(fmt.Sprintf(`<Relationship Id="%s" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/%s"/>`, image.RelID, image.Filename))
	}
	builder.WriteString(`</Relationships>`)
	return builder.String()
}

func fontTableRelationshipsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rIdRegular" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/font" Target="fonts/NotoSansSC-Regular.odttf"/>
  <Relationship Id="rIdBold" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/font" Target="fonts/NotoSansSC-Bold.odttf"/>
</Relationships>`
}

func fontTableXML() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:fonts xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">
  <w:font w:name="Noto Sans SC">%s<w:embedRegular r:id="rIdRegular" w:subsetted="0" w:fontKey="%s"/><w:embedBold r:id="rIdBold" w:subsetted="0" w:fontKey="%s"/></w:font>
</w:fonts>`, notoSansSCFontMetadataXML, regularFontKey, boldFontKey)
}

func settingsXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:embedTrueTypeFonts/><w:embedSystemFonts/><w:saveSubsetFonts w:val="false"/>
  <w:updateFields w:val="true"/>
  <w:themeFontLang w:val="zh-CN" w:eastAsia="zh-CN"/>
  <w:compat><w:compatSetting w:name="compatibilityMode" w:uri="http://schemas.microsoft.com/office/word" w:val="15"/></w:compat>
</w:settings>`
}

func stylesXML() string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:styles xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:docDefaults><w:rPrDefault><w:rPr><w:rFonts w:ascii="Noto Sans SC" w:hAnsi="Noto Sans SC" w:eastAsia="Noto Sans SC"/><w:color w:val="%s"/><w:sz w:val="18"/></w:rPr></w:rPrDefault><w:pPrDefault><w:pPr><w:spacing w:after="90" w:line="276" w:lineRule="auto"/></w:pPr></w:pPrDefault></w:docDefaults>
  <w:style w:type="paragraph" w:default="1" w:styleId="Normal"><w:name w:val="Normal"/><w:qFormat/><w:pPr><w:wordWrap w:val="1"/></w:pPr><w:rPr><w:rFonts w:ascii="Noto Sans SC" w:hAnsi="Noto Sans SC" w:eastAsia="Noto Sans SC"/><w:sz w:val="18"/></w:rPr></w:style>
	  <w:style w:type="paragraph" w:styleId="Heading1"><w:name w:val="heading 1"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:pageBreakBefore/><w:pBdr><w:left w:val="single" w:sz="%d" w:space="8" w:color="%s"/></w:pBdr><w:shd w:val="clear" w:fill="%s"/><w:spacing w:before="240" w:after="120"/><w:ind w:left="120"/><w:outlineLvl w:val="0"/></w:pPr><w:rPr><w:rFonts w:ascii="Noto Sans SC" w:hAnsi="Noto Sans SC" w:eastAsia="Noto Sans SC"/><w:b/><w:color w:val="%s"/><w:sz w:val="28"/></w:rPr></w:style>
	  <w:style w:type="paragraph" w:styleId="Heading2"><w:name w:val="heading 2"/><w:basedOn w:val="Normal"/><w:next w:val="Normal"/><w:qFormat/><w:pPr><w:keepNext/><w:keepLines/><w:pBdr><w:bottom w:val="single" w:sz="8" w:space="4" w:color="%s"/></w:pBdr><w:tabs><w:tab w:val="right" w:pos="9800"/></w:tabs><w:spacing w:before="180" w:after="120"/><w:outlineLvl w:val="1"/></w:pPr><w:rPr><w:rFonts w:ascii="Noto Sans SC" w:hAnsi="Noto Sans SC" w:eastAsia="Noto Sans SC"/><w:b/><w:color w:val="%s"/><w:sz w:val="24"/></w:rPr></w:style>
  <w:style w:type="paragraph" w:styleId="TOC1"><w:name w:val="toc 1"/><w:basedOn w:val="Normal"/><w:pPr><w:keepNext/><w:keepLines/><w:spacing w:before="120" w:after="100" w:line="380" w:lineRule="exact"/></w:pPr><w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="28"/></w:rPr></w:style>
  <w:style w:type="paragraph" w:styleId="TOC2"><w:name w:val="toc 2"/><w:basedOn w:val="Normal"/><w:pPr><w:tabs><w:tab w:val="right" w:leader="dot" w:pos="9800"/></w:tabs><w:keepLines/><w:spacing w:after="80" w:line="360" w:lineRule="exact"/><w:ind w:left="720"/></w:pPr><w:rPr><w:color w:val="%s"/><w:u w:val="single"/><w:sz w:val="24"/></w:rPr></w:style>
</w:styles>`, reportInkColor.Hex, docxChapterBorderSize, reportPrimaryBlueColor.Hex, reportChapterFillColor.Hex, reportPrimaryBlueColor.Hex, reportSubheadingLineColor.Hex, reportPrimaryBlueColor.Hex, reportInkColor.Hex, reportPrimaryBlueColor.Hex)
}

func documentXML(model Model, images map[string]docxImage, fonts FontFiles, pageRefs map[string]int) string {
	var builder strings.Builder
	builder.WriteString(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture" xmlns:v="urn:schemas-microsoft-com:vml"><w:body>`)
	builder.WriteString(coverBlockXML(model, images["cover-background"], fonts))
	builder.WriteString(pageBreakXML())
	builder.WriteString(paragraphXML("目录", "", "", `<w:rPr><w:b/><w:color w:val="242A33"/><w:sz w:val="43"/></w:rPr>`))
	for _, section := range model.Sections {
		builder.WriteString(tocEntryWithPageXML(section, pageRefs[section.Bookmark]))
	}
	builder.WriteString(bodySectionBreakXML())
	bookmarkID := 100
	drawingID := 2
	for sectionIndex, section := range model.Sections {
		keepChapterOpening := section.Level == 1 && sectionIndex+1 < len(model.Sections) && model.Sections[sectionIndex+1].Level > section.Level
		bookmarkID++
		builder.WriteString(headingXML(section, bookmarkID))
		for blockIndex, block := range section.Blocks {
			agentNarrative := isAgentNarrativeBlock(block)
			keepParagraphsWithChart := shouldKeepParagraphsWithChart(section.Blocks, blockIndex)
			if block.Title != "" && block.Kind != BlockRiskCard && block.Kind != BlockRiskBand && block.Kind != BlockChart {
				titleXML := blockTitleParagraphXML(block.Title)
				if block.Title == "数据说明" {
					titleXML = strings.ReplaceAll(titleXML, "315C8C", "667085")
				}
				builder.WriteString(titleXML)
			}
			if block.Kind != BlockRiskBand && block.Kind != BlockRiskCard {
				for paragraphIndex, paragraph := range block.Paragraphs {
					if block.Title == "数据说明" {
						builder.WriteString(paragraphXML(paragraph, "Normal", "", `<w:rPr><w:color w:val="667085"/></w:rPr>`))
						continue
					}
					if agentNarrative {
						keepNext := paragraphIndex < len(block.Paragraphs)-1
						builder.WriteString(paginatedBodyParagraphXML(paragraph, keepNext))
						continue
					}
					if keepParagraphsWithChart || keepChapterOpening {
						builder.WriteString(paginatedBodyParagraphXML(paragraph, true))
						continue
					}
					builder.WriteString(paragraphXML(paragraph, "Normal", "", ""))
				}
			}
			switch block.Kind {
			case BlockTable:
				builder.WriteString(tableBlockXML(block))
			case BlockRiskCard:
				builder.WriteString(riskSummaryCardXML(block))
			case BlockRiskBand:
				builder.WriteString(riskBandXML(block))
			case BlockChart:
				if block.Chart != nil {
					if image, ok := images[block.Chart.ID]; ok {
						imageXML := imageParagraphXML(image, drawingID, block.Chart.Title)
						if blockIndex+1 < len(section.Blocks) && section.Blocks[blockIndex+1].Kind == BlockTable {
							imageXML = strings.Replace(imageXML, "<w:pPr>", "<w:pPr><w:keepNext/>", 1)
						}
						builder.WriteString(imageXML)
						drawingID++
					}
				}
			}
		}
	}
	// Word requires a paragraph after a final table, but adding one after an
	// existing final text paragraph can itself create an empty last page.
	if strings.HasSuffix(builder.String(), "</w:tbl>") {
		builder.WriteString(`<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="20" w:lineRule="exact"/><w:rPr><w:sz w:val="2"/></w:rPr></w:pPr></w:p>`)
	}
	builder.WriteString(`<w:sectPr><w:headerReference w:type="default" r:id="rIdHeader"/><w:footerReference w:type="default" r:id="rIdFooter"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="800" w:right="800" w:bottom="800" w:left="800" w:header="240" w:footer="300" w:gutter="0"/><w:pgNumType w:start="1"/><w:cols w:space="720"/><w:docGrid w:linePitch="312"/></w:sectPr></w:body></w:document>`)
	return builder.String()
}

// riskBandXML renders the bordered pastel 1040-by-44 Figma warning strip. Parameter
// block supplies a confirmed level in Title and trusted copy in Paragraphs.
// The return value is one keep-together OOXML table plus bottom spacing.
func riskBandXML(block Block) string {
	palette, ok := riskBandStyle(RiskLevel(block.Title))
	if !ok || len(block.Paragraphs) == 0 {
		return ""
	}
	return fmt.Sprintf(`<w:tbl><w:tblPr><w:tblW w:w="9800" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="%s"/><w:left w:val="single" w:sz="4" w:color="%s"/><w:bottom w:val="single" w:sz="4" w:color="%s"/><w:right w:val="single" w:sz="4" w:color="%s"/></w:tblBorders></w:tblPr><w:tblGrid><w:gridCol w:w="9800"/></w:tblGrid><w:tr><w:trPr><w:trHeight w:val="%d" w:hRule="exact"/><w:cantSplit/></w:trPr><w:tc><w:tcPr><w:tcW w:w="9800" w:type="dxa"/><w:shd w:val="clear" w:fill="%s"/><w:vAlign w:val="center"/></w:tcPr><w:p><w:pPr><w:keepNext/><w:spacing w:before="0" w:after="0"/><w:ind w:left="140"/></w:pPr><w:r><w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="18"/></w:rPr><w:t>%s</w:t></w:r></w:p></w:tc></w:tr></w:tbl><w:p><w:pPr><w:keepNext/><w:spacing w:after="%d"/></w:pPr></w:p>`, palette.Border.Hex, palette.Border.Hex, palette.Border.Hex, palette.Border.Hex, docxRiskBandHeightTwips, palette.Fill.Hex, palette.Text.Hex, xmlEscape(block.Paragraphs[0]), docxRiskBandSpacingTwips)
}

// blockTitleParagraphXML creates a table/content block title that stays with
// the following content. Parameters: text is the user-visible block title.
// Return value: an OOXML paragraph with keep-next pagination semantics.
func blockTitleParagraphXML(text string) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:keepNext/></w:pPr><w:r><w:rPr><w:b/><w:color w:val="315C8C"/><w:sz w:val="19"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, xmlEscape(text))
}

// paginatedBodyParagraphXML keeps related narrative under its heading and
// prevents a single line from being orphaned. Parameter text is resolved
// trusted content and keepNext links it to the following paragraph or chart.
// The return value is one paginated OOXML paragraph.
func paginatedBodyParagraphXML(text string, keepNext bool) string {
	keepNextXML := ""
	if keepNext {
		keepNextXML = `<w:keepNext/>`
	}
	return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="Normal"/>%s<w:keepLines/></w:pPr><w:r><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, keepNextXML, xmlEscape(text))
}

func coverBlockXML(model Model, background docxImage, fonts FontFiles) string {
	companySize := int(fitCoverCompanyFontSize(fonts.Regular, model.CompanyName) * 2)
	var builder strings.Builder
	builder.WriteString(backgroundImageXML(background, 1))
	builder.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="10306" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblCellMar><w:left w:w="200" w:type="dxa"/><w:right w:w="200" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid><w:gridCol w:w="7200"/><w:gridCol w:w="3106"/></w:tblGrid>`)
	builder.WriteString(coverTableRowXML("", 3380, 16, false, "FFFFFF", ""))
	builder.WriteString(coverTableRowXML("税务风险分析报告", 1180, 85, true, "FFFFFF", "bottom"))
	builder.WriteString(coverTableRowXML(model.CompanyName, 1900, companySize, false, "FFFFFF", ""))
	dateText := "报告期间：  " + model.StartTime + " 至 " + model.EndTime + "\n报告生成时间：  " + model.GeneratedAt.Format("2006-01-02")
	builder.WriteString(coverTableRowXML(dateText, 3200, 32, false, "F2F5FA", ""))
	builder.WriteString(coverTableRowXML("", 3400, 16, false, "FFFFFF", ""))
	disclaimer := "报告声明：\n本报告依据企业授权取得的财税信息，列示主要事实及需要关注的事项。\n报告结论限于所列分析期间及已取得数据。"
	builder.WriteString(coverDisclaimerRowXML(disclaimer))
	builder.WriteString(`</w:tbl>`)
	return builder.String()
}

func backgroundImageXML(image docxImage, drawingID int) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:spacing w:before="0" w:after="0" w:line="1" w:lineRule="exact"/></w:pPr><w:r><w:drawing><wp:anchor distT="0" distB="0" distL="0" distR="0" simplePos="0" relativeHeight="0" behindDoc="1" locked="1" layoutInCell="1" allowOverlap="1"><wp:simplePos x="0" y="0"/><wp:positionH relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionH><wp:positionV relativeFrom="page"><wp:posOffset>0</wp:posOffset></wp:positionV><wp:extent cx="%d" cy="%d"/><wp:effectExtent l="0" t="0" r="0" b="0"/><wp:wrapNone/><wp:docPr id="%d" name="Figma Cover Background" descr="Figma cover background"/><wp:cNvGraphicFramePr><a:graphicFrameLocks noChangeAspect="1"/></wp:cNvGraphicFramePr><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:anchor></w:drawing></w:r></w:p>`, image.Width, image.Height, drawingID, xmlEscape(image.Filename), image.RelID, image.Width, image.Height)
}

// coverTableRowXML creates one fixed-height cover row. Parameters: text is the
// visible content, height and size use OOXML twips/half-points, bold and color
// control text styling, and border selects the Figma title-divider variant.
// Return value is a complete OOXML table row.
func coverTableRowXML(text string, height, size int, bold bool, color, border string) string {
	boldXML := ""
	if bold {
		boldXML = `<w:b/>`
	}
	paragraphs := strings.Split(text, "\n")
	var content strings.Builder
	lineSpacing := 360
	lineRule, gridXML := "auto", ""
	if border == "bottom" {
		// Word's CJK document grid can push the large title below the fixed
		// divider. Use a 50-point line box without grid snapping for this row.
		lineSpacing, lineRule, gridXML = 1000, "exact", `<w:snapToGrid w:val="0"/>`
	}
	for _, paragraph := range paragraphs {
		content.WriteString(fmt.Sprintf(`<w:p><w:pPr>%s<w:spacing w:before="0" w:after="90" w:line="%d" w:lineRule="%s"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Noto Sans SC" w:hAnsi="Noto Sans SC" w:eastAsia="Noto Sans SC"/>%s<w:color w:val="%s"/><w:sz w:val="%d"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, gridXML, lineSpacing, lineRule, boldXML, color, size, xmlEscape(paragraph)))
	}
	if border == "bottom" {
		// The title needs more width than the divider. Keep their combined height
		// unchanged, but span both columns for text so CJK font rounding cannot
		// wrap the final character in Word/WPS's table layout.
		const dividerHeight = 80 // twips, included in the original title row height
		title := fmt.Sprintf(`<w:tr><w:trPr><w:trHeight w:val="%d" w:hRule="exact"/><w:cantSplit/></w:trPr><w:tc><w:tcPr><w:tcW w:w="10306" w:type="dxa"/><w:gridSpan w:val="2"/><w:vAlign w:val="top"/></w:tcPr>%s</w:tc></w:tr>`, height-dividerHeight, content.String())
		divider := fmt.Sprintf(`<w:tr><w:trPr><w:trHeight w:val="%d" w:hRule="exact"/><w:cantSplit/></w:trPr><w:tc><w:tcPr><w:tcW w:w="7200" w:type="dxa"/><w:tcBorders><w:bottom w:val="single" w:sz="28" w:space="0" w:color="55A5FF"/></w:tcBorders></w:tcPr><w:p><w:pPr><w:spacing w:after="0" w:line="20" w:lineRule="exact"/></w:pPr></w:p></w:tc><w:tc><w:tcPr><w:tcW w:w="3106" w:type="dxa"/></w:tcPr><w:p><w:pPr><w:spacing w:after="0" w:line="20" w:lineRule="exact"/></w:pPr></w:p></w:tc></w:tr>`, dividerHeight)
		return title + divider
	}
	return fmt.Sprintf(`<w:tr><w:trPr><w:trHeight w:val="%d" w:hRule="exact"/><w:cantSplit/></w:trPr><w:tc><w:tcPr><w:tcW w:w="10306" w:type="dxa"/><w:gridSpan w:val="2"/><w:vAlign w:val="center"/></w:tcPr>%s</w:tc></w:tr>`, height, content.String())
}

// coverDisclaimerRowXML places the three-line declaration inside the Figma
// shield card instead of over the decorative icon. Parameter text is the
// trusted fixed declaration. Return value is one exact-height OOXML row.
func coverDisclaimerRowXML(text string) string {
	paragraphs := strings.Split(text, "\n")
	var content strings.Builder
	for _, paragraph := range paragraphs {
		content.WriteString(fmt.Sprintf(`<w:p><w:pPr><w:spacing w:before="0" w:after="40" w:line="300" w:lineRule="auto"/><w:ind w:left="1900"/></w:pPr><w:r><w:rPr><w:rFonts w:ascii="Noto Sans SC" w:hAnsi="Noto Sans SC" w:eastAsia="Noto Sans SC"/><w:color w:val="F2F5FA"/><w:sz w:val="23"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p>`, xmlEscape(paragraph)))
	}
	return fmt.Sprintf(`<w:tr><w:trPr><w:trHeight w:val="1600" w:hRule="exact"/><w:cantSplit/></w:trPr><w:tc><w:tcPr><w:tcW w:w="10306" w:type="dxa"/><w:gridSpan w:val="2"/><w:vAlign w:val="center"/></w:tcPr>%s</w:tc></w:tr>`, content.String())
}

func tocEntryXML(section Section) string {
	return tocEntryWithPageXML(section, 1)
}

// tocEntryWithPageXML renders one clickable Figma TOC item with a PAGEREF
// field and a useful cached page number. Parameters are the finalized section
// and its logical body page. The return value is one OOXML paragraph.
func tocEntryWithPageXML(section Section, pageNumber int) string {
	if pageNumber < 1 {
		pageNumber = 1
	}
	style := "TOC1"
	if section.Level > 1 {
		style = "TOC2"
	}
	linkStyle := `<w:color w:val="242A33"/><w:b/><w:u w:val="none"/>`
	if section.Level > 1 {
		linkStyle = `<w:color w:val="3D7FFF"/><w:u w:val="single"/>`
	}
	if section.Level == 1 {
		return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr><w:hyperlink w:anchor="%s" w:history="1"><w:r><w:rPr>%s</w:rPr><w:t>%s</w:t></w:r></w:hyperlink></w:p>`, style, section.Bookmark, linkStyle, xmlEscape(section.Title))
	}
	return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr><w:hyperlink w:anchor="%s" w:history="1"><w:r><w:rPr>%s</w:rPr><w:t>%s</w:t></w:r></w:hyperlink><w:r><w:rPr><w:color w:val="%s"/></w:rPr><w:tab/></w:r><w:fldSimple w:instr=" PAGEREF %s \h "><w:r><w:rPr><w:color w:val="%s"/></w:rPr><w:t>%d</w:t></w:r></w:fldSimple></w:p>`, style, section.Bookmark, linkStyle, xmlEscape(section.Title), reportTOCLeaderColor.Hex, section.Bookmark, reportTOCPageColor.Hex, pageNumber)
}

func headingXML(section Section, bookmarkID int) string {
	style := "Heading1"
	if section.Level == 1 {
		pageBreak := `<w:pageBreakBefore w:val="0"/>`
		return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="%s"/>%s</w:pPr><w:bookmarkStart w:id="%d" w:name="%s"/><w:r><w:t>%s</w:t></w:r><w:bookmarkEnd w:id="%d"/></w:p>`, style, pageBreak, bookmarkID, section.Bookmark, xmlEscape(section.Title), bookmarkID)
	}
	style = "Heading2"
	return fmt.Sprintf(`<w:p><w:pPr><w:pStyle w:val="%s"/></w:pPr><w:bookmarkStart w:id="%d" w:name="%s"/>%s<w:r><w:t xml:space="preserve">  %s</w:t></w:r><w:r><w:tab/></w:r>%s<w:bookmarkEnd w:id="%d"/></w:p>`, style, bookmarkID, section.Bookmark, subheadingMarkerXML(bookmarkID), xmlEscape(section.Title), subheadingTerminalXML(bookmarkID), bookmarkID)
}

// subheadingMarkerXML renders the two offset Figma squares as VML geometry.
// Parameter bookmarkID keeps shape identifiers unique. Return value is one
// inline marker run that does not depend on a font glyph.
func subheadingMarkerXML(bookmarkID int) string {
	return fmt.Sprintf(`<w:r><w:rPr><w:noProof/></w:rPr><w:pict><v:group id="subheading-marker-%d" style="width:%.2fpt;height:%.2fpt" coordorigin="0,0" coordsize="%d,%d"><v:rect style="position:absolute;left:%d;top:0;width:%d;height:%d" fillcolor="#%s" stroked="f"/><v:rect style="position:absolute;left:0;top:%d;width:%d;height:%d" fillcolor="#%s" stroked="f"/></v:group></w:pict></w:r>`, bookmarkID, docxSubheadingMarkerSizePt, docxSubheadingMarkerSizePt, docxSubheadingCoordinate, docxSubheadingCoordinate, docxSubheadingPaleOffsetX, docxSubheadingPaleSquare, docxSubheadingPaleSquare, reportPaleBlueColor.Hex, docxSubheadingBlueOffsetY, docxSubheadingBlueSquare, docxSubheadingBlueSquare, reportPrimaryBlueColor.Hex)
}

// subheadingTerminalXML renders the pale square at the end of a subsection
// divider. Parameter bookmarkID keeps shape identifiers unique. Return value
// is one inline VML marker run.
func subheadingTerminalXML(bookmarkID int) string {
	return fmt.Sprintf(`<w:r><w:rPr><w:noProof/></w:rPr><w:pict><v:rect id="subheading-terminal-%d" style="width:%.2fpt;height:%.2fpt" fillcolor="#%s" stroked="f"/></w:pict></w:r>`, bookmarkID, docxSubheadingTerminalWidth, docxSubheadingTerminalHeight, reportPaleBlueColor.Hex)
}

func paragraphXML(text, style, fill, extraRunProperties string) string {
	styleXML := ""
	if style != "" {
		styleXML = fmt.Sprintf(`<w:pStyle w:val="%s"/>`, style)
	}
	fillXML := ""
	if fill != "" {
		fillXML = fmt.Sprintf(`<w:shd w:val="clear" w:fill="%s"/>`, fill)
	}
	return fmt.Sprintf(`<w:p><w:pPr>%s%s</w:pPr><w:r>%s<w:t xml:space="preserve">%s</w:t></w:r></w:p>`, styleXML, fillXML, extraRunProperties, xmlEscape(text))
}

func pageBreakXML() string {
	return `<w:p><w:r><w:br w:type="page"/></w:r></w:p>`
}

// bodySectionBreakXML ends the cover-and-TOC section without a header/footer
// and starts the body on a new page. It keeps the Figma TOC visually plain
// while allowing the body to restart its visible page numbers at one.
func bodySectionBreakXML() string {
	return `<w:p><w:pPr><w:sectPr><w:type w:val="nextPage"/><w:pgSz w:w="11906" w:h="16838"/><w:pgMar w:top="800" w:right="800" w:bottom="800" w:left="800" w:header="240" w:footer="300" w:gutter="0"/><w:cols w:space="720"/><w:docGrid w:linePitch="312"/></w:sectPr></w:pPr></w:p>`
}

func tableXML(headers []string, rows [][]string, risk bool) string {
	variant := TableVariantDefault
	if risk {
		variant = TableVariantAttention
	}
	return tableBlockXML(Block{Kind: BlockTable, TableVariant: variant, Headers: headers, Rows: rows})
}

// tableBlockXML renders a table with the semantic Figma variant stored in the
// report model. Parameter block supplies headers, rows, and styling identity.
// The return value is standalone OOXML for the table.
func tableBlockXML(block Block) string {
	headers, rows, variant := block.Headers, block.Rows, block.TableVariant
	columns := len(headers)
	if columns == 0 && len(rows) > 0 {
		columns = len(rows[0])
	}
	if columns == 0 {
		return ""
	}
	totalWidth := 9800
	widths := tableColumnWidths(headers, rows, float64(totalWidth), variant)
	riskColumn := riskLevelColumn(headers)
	var builder strings.Builder
	builder.WriteString(fmt.Sprintf(`<w:tbl><w:tblPr><w:tblW w:w="%d" w:type="dxa"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="%s"/><w:left w:val="single" w:sz="4" w:color="%s"/><w:bottom w:val="single" w:sz="4" w:color="%s"/><w:right w:val="single" w:sz="4" w:color="%s"/><w:insideH w:val="single" w:sz="4" w:color="%s"/><w:insideV w:val="single" w:sz="4" w:color="%s"/></w:tblBorders><w:tblLook w:val="04A0" w:firstRow="1" w:lastRow="0" w:firstColumn="1" w:lastColumn="0" w:noHBand="0" w:noVBand="1"/><w:tblLayout w:type="fixed"/><w:tblCellMar><w:top w:w="80" w:type="dxa"/><w:left w:w="90" w:type="dxa"/><w:bottom w:w="80" w:type="dxa"/><w:right w:w="90" w:type="dxa"/></w:tblCellMar></w:tblPr><w:tblGrid>`, totalWidth, reportTableBorderColor.Hex, reportTableBorderColor.Hex, reportTableBorderColor.Hex, reportTableBorderColor.Hex, reportTableGridColor.Hex, reportTableGridColor.Hex))
	for index := 0; index < columns; index++ {
		builder.WriteString(fmt.Sprintf(`<w:gridCol w:w="%d"/>`, int(widths[index])))
	}
	builder.WriteString(`</w:tblGrid>`)
	if len(headers) > 0 {
		builder.WriteString(`<w:tr><w:trPr><w:tblHeader w:val="1"/><w:cantSplit/></w:trPr>`)
		for index, header := range headers {
			cellXML := tableCellXMLStyled(header, int(widths[index]), reportTableHeaderColor, reportTableTextColor, true, nil, "left")
			builder.WriteString(strings.Replace(cellXML, "<w:pPr>", "<w:pPr><w:keepNext/>", 1))
		}
		builder.WriteString(`</w:tr>`)
	}
	for rowIndex, row := range rows {
		fill := reportTableRowColor.Hex
		if rowIndex%2 == 1 {
			fill = reportTableAlternateColor.Hex
		}
		if variant == TableVariantAttention {
			fill = reportRiskAttentionColor.Hex
		}
		builder.WriteString(`<w:tr><w:trPr><w:cantSplit/></w:trPr>`)
		for columnIndex := 0; columnIndex < columns; columnIndex++ {
			value := ""
			if columnIndex < len(row) {
				value = row[columnIndex]
			}
			cellFill := reportColor{Hex: fill}
			textColor := reportTableTextColor
			bold := false
			alignment := "left"
			var leftBorder *reportColor
			if tableHasRiskColumnStyle(variant) && columnIndex == riskColumn {
				if palette, ok := riskTableStyle(value); ok {
					cellFill, textColor, bold = palette.Fill, palette.Text, true
					border := palette.LeftBorder
					leftBorder = &border
				}
			}
			builder.WriteString(tableCellXMLStyled(value, int(widths[columnIndex]), cellFill, textColor, bold, leftBorder, alignment))
		}
		builder.WriteString(`</w:tr>`)
	}
	builder.WriteString(`</w:tbl>`)
	return builder.String()
}

// riskSummaryCardXML renders the Figma three-column risk summary component.
// Parameter block contains high/medium/low counts plus the trusted overall
// outcome and deterministic execution summary. Return value is OOXML for one
// keep-together card.
func riskSummaryCardXML(block Block) string {
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
	var builder strings.Builder
	builder.WriteString(`<w:tbl><w:tblPr><w:tblW w:w="9800" w:type="dxa"/><w:tblLayout w:type="fixed"/><w:tblBorders><w:top w:val="single" w:sz="4" w:color="DCE2EB"/><w:left w:val="single" w:sz="4" w:color="DCE2EB"/><w:bottom w:val="single" w:sz="4" w:color="DCE2EB"/><w:right w:val="single" w:sz="4" w:color="DCE2EB"/><w:insideV w:val="single" w:sz="4" w:color="DCE2EB"/></w:tblBorders></w:tblPr><w:tblGrid><w:gridCol w:w="3266"/><w:gridCol w:w="3267"/><w:gridCol w:w="3267"/></w:tblGrid><w:tr><w:trPr><w:trHeight w:val="1400" w:hRule="exact"/><w:cantSplit/></w:trPr>`)
	for index := range labels {
		builder.WriteString(fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="3266" w:type="dxa"/><w:shd w:val="clear" w:fill="%s"/><w:vAlign w:val="center"/></w:tcPr><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:b/><w:color w:val="%s"/><w:sz w:val="32"/></w:rPr><w:t>%s  %s</w:t></w:r></w:p></w:tc>`, styles[index].Fill.Hex, styles[index].Text.Hex, xmlEscape(labels[index]), xmlEscape(counts[index])))
	}
	builder.WriteString(`</w:tr>`)
	overall := "未见明显异常"
	summary := ""
	if len(block.Paragraphs) > 0 {
		overall = block.Paragraphs[0]
	}
	if len(block.Paragraphs) > 1 {
		summary = block.Paragraphs[1]
	}
	builder.WriteString(fmt.Sprintf(`<w:tr><w:trPr><w:cantSplit/></w:trPr><w:tc><w:tcPr><w:gridSpan w:val="3"/><w:tcW w:w="9800" w:type="dxa"/><w:shd w:val="clear" w:fill="%s"/><w:tcBorders><w:left w:val="single" w:sz="20" w:color="F86602"/></w:tcBorders></w:tcPr><w:p><w:pPr><w:spacing w:before="90" w:after="90"/><w:ind w:left="140"/></w:pPr><w:r><w:rPr><w:b/><w:color w:val="%s"/></w:rPr><w:t>综合风险评价：%s</w:t></w:r><w:r><w:t xml:space="preserve">；%s</w:t></w:r></w:p></w:tc></w:tr></w:tbl>`, reportOverallRiskFill.Hex, reportOverallRiskText.Hex, xmlEscape(overall), xmlEscape(summary)))
	return builder.String()
}

func tableCellXML(text string, width int, fill string, bold bool) string {
	return tableCellXMLStyled(text, width, reportColor{Hex: fill}, reportInkColor, bold, nil, "left")
}

// tableCellXMLStyled renders one DOCX cell using shared Figma table tokens.
// Parameters define visible text, width, fill/text colors, emphasis, optional
// left status border, and paragraph alignment. The return value is OOXML.
func tableCellXMLStyled(text string, width int, fill, textColor reportColor, bold bool, leftBorder *reportColor, alignment string) string {
	boldXML := ""
	if bold {
		boldXML = "<w:b/>"
	}
	borderXML := ""
	if leftBorder != nil {
		borderXML = fmt.Sprintf(`<w:tcBorders><w:left w:val="single" w:sz="24" w:space="0" w:color="%s"/></w:tcBorders>`, leftBorder.Hex)
	}
	justificationXML := ""
	if alignment == "center" {
		justificationXML = `<w:jc w:val="center"/>`
	}
	return fmt.Sprintf(`<w:tc><w:tcPr><w:tcW w:w="%d" w:type="dxa"/><w:shd w:val="clear" w:fill="%s"/>%s<w:vAlign w:val="center"/></w:tcPr><w:p><w:pPr>%s<w:spacing w:before="0" w:after="0" w:line="240" w:lineRule="auto"/></w:pPr><w:r><w:rPr>%s<w:color w:val="%s"/><w:sz w:val="16"/></w:rPr><w:t xml:space="preserve">%s</w:t></w:r></w:p></w:tc>`, width, fill.Hex, borderXML, justificationXML, boldXML, textColor.Hex, tableCellTextXML(text))
}

// tableCellTextXML converts explicit model line breaks into Word line-break
// elements without changing any visible characters. Parameter text is trusted
// table content. The return value is safe for insertion inside one w:t element.
func tableCellTextXML(text string) string {
	lines := strings.Split(text, "\n")
	for index := range lines {
		lines[index] = xmlEscape(lines[index])
	}
	return strings.Join(lines, `</w:t><w:br/><w:t xml:space="preserve">`)
}

func imageParagraphXML(image docxImage, drawingID int, description string) string {
	return fmt.Sprintf(`<w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:drawing><wp:inline distT="0" distB="0" distL="0" distR="0"><wp:extent cx="%d" cy="%d"/><wp:docPr id="%d" name="%s" descr="%s"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="0" name="%s"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:embed="%s"/><a:stretch><a:fillRect/></a:stretch></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="%d" cy="%d"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p>`, image.Width, image.Height, drawingID, xmlEscape(image.ID), xmlEscape(description), xmlEscape(image.Filename), image.RelID, image.Width, image.Height)
}

func headerXML(model Model) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:hdr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:tbl><w:tblPr><w:tblW w:w="10306" w:type="dxa"/><w:tblBorders><w:bottom w:val="single" w:sz="12" w:color="%s"/></w:tblBorders><w:shd w:val="clear" w:fill="%s"/><w:tblLayout w:type="fixed"/></w:tblPr><w:tblGrid><w:gridCol w:w="6800"/><w:gridCol w:w="3506"/></w:tblGrid><w:tr><w:trPr><w:trHeight w:val="600" w:hRule="exact"/></w:trPr><w:tc><w:tcPr><w:tcW w:w="6800" w:type="dxa"/><w:vAlign w:val="center"/></w:tcPr><w:p><w:pPr><w:spacing w:before="0" w:after="0"/></w:pPr><w:r><w:rPr><w:color w:val="%s"/><w:sz w:val="24"/></w:rPr><w:t>%s</w:t></w:r></w:p></w:tc><w:tc><w:tcPr><w:tcW w:w="3506" w:type="dxa"/><w:vAlign w:val="center"/></w:tcPr><w:p><w:pPr><w:jc w:val="right"/><w:spacing w:before="0" w:after="0"/></w:pPr><w:r><w:rPr><w:color w:val="%s"/><w:sz w:val="24"/></w:rPr><w:t>财税风险分析报告</w:t></w:r></w:p></w:tc></w:tr></w:tbl></w:hdr>`, reportPrimaryBlueColor.Hex, reportHeaderFillColor.Hex, reportPrimaryBlueColor.Hex, xmlEscape(model.CompanyName), reportPrimaryBlueColor.Hex)
}

func footerXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><w:ftr xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main"><w:p><w:pPr><w:jc w:val="center"/></w:pPr><w:r><w:rPr><w:color w:val="6E7B8E"/><w:sz w:val="15"/></w:rPr><w:fldChar w:fldCharType="begin"/><w:instrText xml:space="preserve"> PAGE </w:instrText><w:fldChar w:fldCharType="separate"/><w:t>1</w:t><w:fldChar w:fldCharType="end"/></w:r></w:p></w:ftr>`
}

func appPropertiesXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?><Properties xmlns="http://schemas.openxmlformats.org/officeDocument/2006/extended-properties" xmlns:vt="http://schemas.openxmlformats.org/officeDocument/2006/docPropsVTypes"><Application>tax-analysis</Application><DocSecurity>0</DocSecurity><ScaleCrop>false</ScaleCrop><Company></Company><AppVersion>0.1.1</AppVersion></Properties>`
}

func corePropertiesXML(model Model) string {
	timestamp := model.GeneratedAt.UTC().Format(time.RFC3339)
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?><cp:coreProperties xmlns:cp="http://schemas.openxmlformats.org/package/2006/metadata/core-properties" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:dcterms="http://purl.org/dc/terms/" xmlns:dcmitype="http://purl.org/dc/dcmitype/" xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"><dc:title>%s财税风险分析报告</dc:title><dc:subject>%s</dc:subject><dc:creator>tax-analysis</dc:creator><cp:lastModifiedBy>tax-analysis</cp:lastModifiedBy><dcterms:created xsi:type="dcterms:W3CDTF">%s</dcterms:created><dcterms:modified xsi:type="dcterms:W3CDTF">%s</dcterms:modified></cp:coreProperties>`, xmlEscape(model.CompanyName), xmlEscape(reportDocumentSubject(model)), timestamp, timestamp)
}

func themeXML() string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<a:theme xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" name="Tax Analysis">
  <a:themeElements>
    <a:clrScheme name="Tax Analysis"><a:dk1><a:srgbClr val="242A33"/></a:dk1><a:lt1><a:srgbClr val="FFFFFF"/></a:lt1><a:dk2><a:srgbClr val="1F5FD1"/></a:dk2><a:lt2><a:srgbClr val="EAF2FF"/></a:lt2><a:accent1><a:srgbClr val="3D7FFF"/></a:accent1><a:accent2><a:srgbClr val="28B950"/></a:accent2><a:accent3><a:srgbClr val="F86602"/></a:accent3><a:accent4><a:srgbClr val="F7B500"/></a:accent4><a:accent5><a:srgbClr val="FF3B42"/></a:accent5><a:accent6><a:srgbClr val="8B5BDC"/></a:accent6><a:hlink><a:srgbClr val="1F5FD1"/></a:hlink><a:folHlink><a:srgbClr val="6574FF"/></a:folHlink></a:clrScheme>
    <a:fontScheme name="Noto Sans SC"><a:majorFont><a:latin typeface="Noto Sans SC"/><a:ea typeface="Noto Sans SC"/><a:cs typeface="Noto Sans SC"/></a:majorFont><a:minorFont><a:latin typeface="Noto Sans SC"/><a:ea typeface="Noto Sans SC"/><a:cs typeface="Noto Sans SC"/></a:minorFont></a:fontScheme>
    <a:fmtScheme name="Tax Analysis">
      <a:fillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:tint val="80000"/></a:schemeClr></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:shade val="80000"/></a:schemeClr></a:solidFill></a:fillStyleLst>
      <a:lnStyleLst><a:ln w="9525"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln><a:ln w="25400"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln><a:ln w="38100"><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:prstDash val="solid"/></a:ln></a:lnStyleLst>
      <a:effectStyleLst><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle><a:effectStyle><a:effectLst/></a:effectStyle></a:effectStyleLst>
      <a:bgFillStyleLst><a:solidFill><a:schemeClr val="phClr"/></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:tint val="95000"/></a:schemeClr></a:solidFill><a:solidFill><a:schemeClr val="phClr"><a:shade val="95000"/></a:schemeClr></a:solidFill></a:bgFillStyleLst>
    </a:fmtScheme>
  </a:themeElements>
</a:theme>`
}

// obfuscateFont returns a copy of font whose first 32 bytes are XOR-obfuscated
// with the reversed canonical GUID bytes from key, as required by ISO/IEC
// 29500 font embedding. The input font is never modified.
func obfuscateFont(font []byte, key string) []byte {
	result := append([]byte(nil), font...)
	keyBytes, err := guidBytes(key)
	if err != nil {
		return result
	}
	limit := fontObfuscationPrefixLength
	if len(result) < limit {
		limit = len(result)
	}
	for index := 0; index < limit; index++ {
		result[index] ^= keyBytes[fontObfuscationKeyLength-1-(index%fontObfuscationKeyLength)]
	}
	return result
}

// guidBytes decodes key into the 16-byte little-endian GUID layout used by
// OOXML font embedding. Parameter value is a canonical GUID with optional
// braces. Return values are the binary GUID bytes and a validation error.
func guidBytes(value string) ([]byte, error) {
	normalized := strings.Trim(strings.TrimSpace(value), "{}")
	parts := strings.Split(normalized, "-")
	if len(parts) != 5 || len(parts[0]) != 8 || len(parts[1]) != 4 || len(parts[2]) != 4 || len(parts[3]) != 4 || len(parts[4]) != 12 {
		return nil, fmt.Errorf("invalid GUID")
	}
	result := make([]byte, 0, fontObfuscationKeyLength)
	for index, part := range parts {
		decoded, err := hex.DecodeString(part)
		if err != nil {
			return nil, fmt.Errorf("invalid GUID: %w", err)
		}
		if index < 3 {
			for left, right := 0, len(decoded)-1; left < right; left, right = left+1, right-1 {
				decoded[left], decoded[right] = decoded[right], decoded[left]
			}
		}
		result = append(result, decoded...)
	}
	if len(result) != fontObfuscationKeyLength {
		return nil, fmt.Errorf("invalid GUID")
	}
	return result, nil
}

func xmlEscape(value string) string {
	var builder bytes.Buffer
	for _, char := range value {
		switch char {
		case '&':
			builder.WriteString("&amp;")
		case '<':
			builder.WriteString("&lt;")
		case '>':
			builder.WriteString("&gt;")
		case '"':
			builder.WriteString("&quot;")
		case '\'':
			builder.WriteString("&apos;")
		default:
			_, _ = io.WriteString(&builder, string(char))
		}
	}
	return builder.String()
}
