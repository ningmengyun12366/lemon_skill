package common

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/signintech/gopdf"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	east "github.com/yuin/goldmark/extension/ast"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

const (
	pdfPageWidth  = 595.28
	pdfPageHeight = 841.89
	pdfMarginX    = 51.02
	pdfMarginTop  = 51.02
	pdfMarginBot  = 51.02
	pdfFontFamily = "invoice-assistant-regular"
	pdfFontBold   = "invoice-assistant-bold"

	cachedRegularFontName = "NotoSansSC-Regular.ttf"
	cachedBoldFontName    = "NotoSansSC-Bold.ttf"
	pdfFontAssetsDir      = "fonts"
)

func ToPDF(inputPath, outputPath string) error {
	raw, err := os.ReadFile(inputPath)
	if err != nil {
		return err
	}
	if outputDir := filepath.Dir(outputPath); outputDir != "." && outputDir != "" {
		if err := os.MkdirAll(outputDir, 0o755); err != nil {
			return err
		}
	}

	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
	)
	root := md.Parser().Parse(text.NewReader(raw))

	renderer, err := newPDFRenderer(outputPath)
	if err != nil {
		return err
	}
	if err := renderer.render(root, raw); err != nil {
		return err
	}
	return renderer.pdf.WritePdf(outputPath)
}

type pdfRenderer struct {
	pdf          *gopdf.GoPdf
	contentWidth float64
	lineHeight   float64
	fontSize     float64
	boldReady    bool
	bold         bool
}

type inlineSpan struct {
	text string
	bold bool
}

func newPDFRenderer(title string) (*pdfRenderer, error) {
	pdf := &gopdf.GoPdf{}
	pdf.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
	pdf.SetInfo(gopdf.PdfInfo{Title: filepath.Base(title)})
	pdf.AddPage()
	pdf.SetXY(pdfMarginX, pdfMarginTop)

	boldReady, err := addPDFFonts(pdf)
	if err != nil {
		return nil, err
	}
	if err := pdf.SetFont(pdfFontFamily, "", 10.5); err != nil {
		return nil, err
	}
	pdf.SetTextColor(17, 24, 39)

	return &pdfRenderer{
		pdf:          pdf,
		contentWidth: pdfPageWidth - pdfMarginX*2,
		lineHeight:   16,
		fontSize:     10.5,
		boldReady:    boldReady,
	}, nil
}

func addPDFFonts(pdf *gopdf.GoPdf) (bool, error) {
	if regular, bold, ok := firstLoadableFontPair(localNotoSansSCFontPairs()); ok {
		if err := addSelectedPDFFonts(pdf, regular, bold); err != nil {
			return false, err
		}
		return true, nil
	}
	return false, fmt.Errorf("no usable NotoSansSC font pair found for PDF generation; expected local files under %s or %s", filepath.Join(SkillInstallDir(), "assets", pdfFontAssetsDir), filepath.Join(SkillInstallDir(), "internal", "assets", pdfFontAssetsDir))
}

type pdfFontPair struct {
	regular string
	bold    string
}

func firstLoadableFontPair(pairs []pdfFontPair) (string, string, bool) {
	for _, pair := range pairs {
		if pair.regular == "" || pair.bold == "" {
			continue
		}
		if _, err := os.Stat(pair.regular); err != nil {
			continue
		}
		if _, err := os.Stat(pair.bold); err != nil {
			continue
		}
		probe := &gopdf.GoPdf{}
		probe.Start(gopdf.Config{PageSize: *gopdf.PageSizeA4})
		if err := probe.AddTTFFont(pdfFontFamily, pair.regular); err != nil {
			continue
		}
		if err := probe.AddTTFFont(pdfFontBold, pair.bold); err != nil {
			continue
		}
		return pair.regular, pair.bold, true
	}
	return "", "", false
}

func addSelectedPDFFonts(pdf *gopdf.GoPdf, regular, bold string) error {
	if err := pdf.AddTTFFont(pdfFontFamily, regular); err != nil {
		return err
	}
	if err := pdf.AddTTFFont(pdfFontBold, bold); err != nil {
		return err
	}
	return nil
}

func localNotoSansSCFontPairs() []pdfFontPair {
	return []pdfFontPair{
		{
			regular: localPDFFontPath(false),
			bold:    localPDFFontPath(true),
		},
		{
			regular: filepath.Join(SkillInstallDir(), "internal", "assets", pdfFontAssetsDir, cachedRegularFontName),
			bold:    filepath.Join(SkillInstallDir(), "internal", "assets", pdfFontAssetsDir, cachedBoldFontName),
		},
	}
}

func localPDFFontPath(bold bool) string {
	name := cachedRegularFontName
	if bold {
		name = cachedBoldFontName
	}
	return filepath.Join(SkillInstallDir(), "assets", pdfFontAssetsDir, name)
}

func (r *pdfRenderer) render(root ast.Node, source []byte) error {
	for child := root.FirstChild(); child != nil; child = child.NextSibling() {
		if err := r.renderBlock(child, source, 0); err != nil {
			return err
		}
	}
	return nil
}

func (r *pdfRenderer) renderBlock(node ast.Node, source []byte, listDepth int) error {
	switch n := node.(type) {
	case *ast.Heading:
		return r.renderHeading(n, source)
	case *ast.Paragraph:
		return r.renderParagraph(n, source, listDepth)
	case *ast.List:
		return r.renderList(n, source, listDepth)
	case *ast.Blockquote:
		return r.renderQuote(n, source)
	case *ast.FencedCodeBlock:
		return r.renderCodeBlock(n, source)
	case *ast.CodeBlock:
		return r.renderCodeBlock(n, source)
	case *ast.ThematicBreak:
		return r.renderThematicBreak()
	case *east.Table:
		return r.renderTable(n, source)
	default:
		for child := node.FirstChild(); child != nil; child = child.NextSibling() {
			if err := r.renderBlock(child, source, listDepth); err != nil {
				return err
			}
		}
	}
	return nil
}

func (r *pdfRenderer) renderHeading(n *ast.Heading, source []byte) error {
	specs := map[int]struct {
		size    float64
		leading float64
	}{
		1: {18, 24},
		2: {16, 22},
		3: {14, 20},
		4: {12.5, 18},
		5: {11.5, 16},
		6: {10.5, 15},
	}
	spec := specs[n.Level]
	if spec.size == 0 {
		spec = specs[6]
	}
	if n.Level <= 3 {
		r.addSpace(12)
	} else {
		r.addSpace(8)
	}
	if err := r.setFont(spec.size, true); err != nil {
		return err
	}
	r.pdf.SetTextColor(17, 24, 39)
	text := inlineText(n, source)
	if err := r.writeWrapped(text, pdfMarginX, r.contentWidth, spec.leading); err != nil {
		return err
	}
	r.addSpace(6)
	r.pdf.SetTextColor(17, 24, 39)
	return r.setFont(10.5, false)
}

func (r *pdfRenderer) renderParagraph(n ast.Node, source []byte, listDepth int) error {
	spans := trimInlineSpans(inlineSpans(n, source))
	text := strings.TrimSpace(spansPlainText(spans))
	if text == "" {
		r.addSpace(4)
		return nil
	}
	if text == `\f` {
		r.pdf.AddPage()
		r.pdf.SetXY(pdfMarginX, pdfMarginTop)
		return r.setFont(10.5, false)
	}
	x := pdfMarginX + float64(listDepth)
	if err := r.writeInlineWrapped(spans, x, pdfPageWidth-pdfMarginX-x, r.lineHeight); err != nil {
		return err
	}
	r.addSpace(6)
	return nil
}

func (r *pdfRenderer) renderList(n *ast.List, source []byte, depth int) error {
	index := n.Start
	for item := n.FirstChild(); item != nil; item = item.NextSibling() {
		prefix := "\u2022 "
		if n.IsOrdered() {
			prefix = fmt.Sprintf("%d. ", index)
			index++
		}
		if err := r.renderListItem(item, source, depth, prefix); err != nil {
			return err
		}
	}
	return nil
}

func (r *pdfRenderer) renderListItem(item ast.Node, source []byte, depth int, prefix string) error {
	x := pdfMarginX + 14 + float64(depth)*12
	textX := x + 14
	first := true
	for child := item.FirstChild(); child != nil; child = child.NextSibling() {
		switch child.(type) {
		case *ast.Paragraph:
			spans := trimInlineSpans(inlineSpans(child, source))
			if taskPrefix := taskCheckboxPrefix(child); taskPrefix != "" {
				prefix = taskPrefix
			}
			if first {
				if err := r.writeListInline(prefix, spans, x, pdfPageWidth-pdfMarginX-x, r.lineHeight); err != nil {
					return err
				}
			} else {
				if err := r.writeInlineWrapped(spans, x, pdfPageWidth-pdfMarginX-x, r.lineHeight); err != nil {
					return err
				}
			}
			r.addSpace(6)
			first = false
		case *ast.List:
			if err := r.renderBlock(child, source, depth+1); err != nil {
				return err
			}
		default:
			if !first {
				if err := r.renderBlock(child, source, depth+1); err != nil {
					return err
				}
				continue
			}
			spans := trimInlineSpans(inlineSpans(child, source))
			if spansPlainText(spans) != "" {
				if err := r.writeListInline(prefix, spans, x, pdfPageWidth-pdfMarginX-x, r.lineHeight); err != nil {
					return err
				}
				r.addSpace(6)
				first = false
			}
		}
	}
	if first {
		return r.writeWrapped(prefix, textX, pdfPageWidth-pdfMarginX-textX, r.lineHeight)
	}
	return nil
}

func (r *pdfRenderer) renderCodeBlock(n ast.Node, source []byte) error {
	if err := r.setFont(9, false); err != nil {
		return err
	}
	code := stripUnsupportedSymbols(blockText(n, source))
	lines := strings.Split(strings.TrimRight(code, "\n"), "\n")
	if len(lines) == 0 {
		lines = []string{""}
	}
	lineHeight := 13.0
	height := lineHeight*float64(len(lines)) + 16
	r.ensureSpace(height)
	x, y := pdfMarginX, r.pdf.GetY()
	r.pdf.SetFillColor(245, 247, 250)
	r.pdf.SetStrokeColor(217, 224, 232)
	r.pdf.RectFromUpperLeftWithStyle(x, y, r.contentWidth, height, "DF")
	r.pdf.SetTextColor(17, 24, 39)
	r.pdf.SetXY(x+8, y+8)
	for _, line := range lines {
		if err := r.writeWrapped(line, x+8, r.contentWidth-16, lineHeight); err != nil {
			return err
		}
	}
	r.pdf.SetY(y + height + 8)
	return r.setFont(10.5, false)
}

func (r *pdfRenderer) renderThematicBreak() error {
	r.ensureSpace(14)
	y := r.pdf.GetY() + 6
	r.pdf.SetStrokeColor(208, 215, 226)
	r.pdf.SetLineWidth(0.8)
	r.pdf.Line(pdfMarginX, y, pdfPageWidth-pdfMarginX, y)
	r.pdf.SetY(y + 8)
	return nil
}

func (r *pdfRenderer) renderQuote(n *ast.Blockquote, source []byte) error {
	text := strings.TrimSpace(inlineText(n, source))
	if text == "" {
		return nil
	}
	x := pdfMarginX + 16
	padding := 8.0
	width := r.contentWidth - 16
	if err := r.setFont(10.5, false); err != nil {
		return err
	}
	lines := r.splitText(text, width-padding*2)
	height := float64(len(lines))*r.lineHeight + padding*2
	r.ensureSpace(height)
	y := r.pdf.GetY()
	r.pdf.SetFillColor(247, 249, 252)
	r.pdf.SetStrokeColor(213, 219, 227)
	r.pdf.RectFromUpperLeftWithStyle(x, y, width, height, "DF")
	r.pdf.SetTextColor(68, 80, 102)
	r.pdf.SetXY(x+padding, y+padding)
	for _, line := range lines {
		if err := r.pdf.Cell(&gopdf.Rect{W: width - padding*2, H: r.lineHeight}, line); err != nil {
			return err
		}
		r.pdf.Br(r.lineHeight)
		r.pdf.SetX(x + padding)
	}
	r.pdf.SetTextColor(17, 24, 39)
	r.pdf.SetY(y + height + 6)
	return nil
}

func (r *pdfRenderer) renderTable(n *east.Table, source []byte) error {
	rows := tableRows(n, source)
	if len(rows) == 0 {
		return nil
	}
	colCount := 0
	for _, row := range rows {
		if len(row) > colCount {
			colCount = len(row)
		}
	}
	if colCount == 0 {
		return nil
	}
	if err := r.setFont(10.5, false); err != nil {
		return err
	}
	paddingX := 6.0
	paddingY := 5.0
	colW := r.contentWidth / float64(colCount)
	for rowIndex, row := range rows {
		lineGroups := make([][]string, colCount)
		maxLines := 1
		for i := 0; i < colCount; i++ {
			cell := ""
			if i < len(row) {
				cell = row[i]
			}
			lines := r.splitText(cell, colW-paddingX*2)
			lineGroups[i] = lines
			if len(lines) > maxLines {
				maxLines = len(lines)
			}
		}
		rowH := float64(maxLines)*16 + paddingY*2
		r.ensureSpace(rowH)
		x, y := pdfMarginX, r.pdf.GetY()
		currentX := x
		r.pdf.SetLineWidth(0.5)
		r.pdf.SetStrokeColor(202, 213, 226)
		for i := 0; i < colCount; i++ {
			if rowIndex == 0 {
				r.pdf.SetFillColor(234, 242, 255)
				r.pdf.RectFromUpperLeftWithStyle(currentX, y, colW, rowH, "DF")
			} else {
				if rowIndex%2 == 0 {
					r.pdf.SetFillColor(249, 251, 253)
				} else {
					r.pdf.SetFillColor(255, 255, 255)
				}
				r.pdf.RectFromUpperLeftWithStyle(currentX, y, colW, rowH, "DF")
			}
			r.pdf.RectFromUpperLeftWithStyle(currentX, y, colW, rowH, "D")
			textX := currentX + paddingX
			textW := colW - paddingX*2
			r.pdf.SetTextColor(17, 24, 39)
			if rowIndex == 0 {
				_ = r.setFont(10.5, true)
				r.pdf.SetTextColor(15, 23, 42)
			} else {
				_ = r.setFont(10.5, false)
			}
			r.pdf.SetY(y + paddingY)
			for _, line := range lineGroups[i] {
				r.pdf.SetXY(textX, r.pdf.GetY())
				if err := r.pdf.Cell(&gopdf.Rect{W: textW, H: 16}, line); err != nil {
					return err
				}
				r.pdf.Br(16)
			}
			currentX += colW
		}
		r.pdf.SetLineWidth(0.75)
		r.pdf.RectFromUpperLeftWithStyle(x, y, r.contentWidth, rowH, "D")
		r.pdf.SetY(y + rowH)
	}
	r.addSpace(8)
	r.pdf.SetTextColor(17, 24, 39)
	return r.setFont(10.5, false)
}

func tableRows(table *east.Table, source []byte) [][]string {
	rows := [][]string{}
	for rowNode := table.FirstChild(); rowNode != nil; rowNode = rowNode.NextSibling() {
		switch rowNode.(type) {
		case *east.TableHeader, *east.TableRow:
			row := []string{}
			for cell := rowNode.FirstChild(); cell != nil; cell = cell.NextSibling() {
				row = append(row, strings.TrimSpace(inlineText(cell, source)))
			}
			rows = append(rows, row)
		}
	}
	return rows
}

func taskCheckboxPrefix(node ast.Node) string {
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		if checkbox, ok := child.(*east.TaskCheckBox); ok {
			if checkbox.IsChecked {
				return "\u2611 "
			}
			return "\u2610 "
		}
	}
	return ""
}

func inlineText(node ast.Node, source []byte) string {
	var buf bytes.Buffer
	_ = ast.Walk(node, func(n ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch t := n.(type) {
		case *ast.Text:
			buf.Write(t.Value(source))
			if t.HardLineBreak() {
				buf.WriteByte('\n')
			} else if t.SoftLineBreak() {
				buf.WriteByte(' ')
			}
		case *ast.CodeSpan:
			buf.Write(t.Text(source))
		case *ast.AutoLink:
			buf.Write(t.URL(source))
		case *east.TaskCheckBox:
			return ast.WalkSkipChildren, nil
		case *ast.RawHTML:
			return ast.WalkSkipChildren, nil
		}
		return ast.WalkContinue, nil
	})
	return stripUnsupportedSymbols(normalizeInlineText(buf.String()))
}

func inlineSpans(node ast.Node, source []byte) []inlineSpan {
	var spans []inlineSpan
	collectInlineSpans(node, source, false, &spans)
	return trimInlineSpans(spans)
}

func collectInlineSpans(node ast.Node, source []byte, bold bool, spans *[]inlineSpan) {
	switch n := node.(type) {
	case *ast.Text:
		value := string(n.Value(source))
		if n.HardLineBreak() {
			value += "\n"
		} else if n.SoftLineBreak() {
			value += " "
		}
		appendInlineSpan(spans, stripUnsupportedSymbols(value), bold)
		return
	case *ast.CodeSpan:
		appendInlineSpan(spans, stripUnsupportedSymbols(string(n.Text(source))), bold)
		return
	case *ast.AutoLink:
		appendInlineSpan(spans, stripUnsupportedSymbols(string(n.URL(source))), bold)
		return
	case *east.TaskCheckBox, *ast.RawHTML:
		return
	case *ast.Emphasis:
		bold = bold || n.Level >= 2
	}
	for child := node.FirstChild(); child != nil; child = child.NextSibling() {
		collectInlineSpans(child, source, bold, spans)
	}
}

func appendInlineSpan(spans *[]inlineSpan, text string, bold bool) {
	if text == "" {
		return
	}
	if len(*spans) > 0 && (*spans)[len(*spans)-1].bold == bold {
		(*spans)[len(*spans)-1].text += text
		return
	}
	*spans = append(*spans, inlineSpan{text: text, bold: bold})
}

func trimInlineSpans(spans []inlineSpan) []inlineSpan {
	for len(spans) > 0 {
		spans[0].text = strings.TrimLeft(spans[0].text, " \t\r\n")
		if spans[0].text != "" {
			break
		}
		spans = spans[1:]
	}
	for len(spans) > 0 {
		last := len(spans) - 1
		spans[last].text = strings.TrimRight(spans[last].text, " \t\r\n")
		if spans[last].text != "" {
			break
		}
		spans = spans[:last]
	}
	return spans
}

func spansPlainText(spans []inlineSpan) string {
	var b strings.Builder
	for _, span := range spans {
		b.WriteString(span.text)
	}
	return b.String()
}

func normalizeInlineText(value string) string {
	lines := strings.Split(value, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func blockText(node ast.Node, source []byte) string {
	if lines := node.Lines(); lines != nil && lines.Len() > 0 {
		return string(lines.Value(source))
	}
	return inlineText(node, source)
}

func (r *pdfRenderer) writeWrapped(value string, x, width, lineHeight float64) error {
	if strings.TrimSpace(value) == "" {
		r.addSpace(lineHeight / 2)
		return nil
	}
	lines := r.splitText(value, width)
	r.ensureSpace(float64(len(lines)) * lineHeight)
	for _, line := range lines {
		r.ensureSpace(lineHeight)
		r.pdf.SetXY(x, r.pdf.GetY())
		if err := r.pdf.Cell(&gopdf.Rect{W: width, H: lineHeight}, line); err != nil {
			return err
		}
		r.pdf.Br(lineHeight)
	}
	return nil
}

func (r *pdfRenderer) writeInlineWrapped(spans []inlineSpan, x, width, lineHeight float64) error {
	lines := r.splitInlineSpans(spans, width)
	r.ensureSpace(float64(len(lines)) * lineHeight)
	for _, line := range lines {
		r.ensureSpace(lineHeight)
		r.pdf.SetXY(x, r.pdf.GetY())
		if err := r.drawInlineLine(line, width, lineHeight); err != nil {
			return err
		}
		r.pdf.Br(lineHeight)
	}
	_ = r.setFont(r.fontSize, false)
	return nil
}

func (r *pdfRenderer) writeListText(marker, value string, x, width, lineHeight float64) error {
	markerWidth, err := r.pdf.MeasureTextWidth(marker)
	if err != nil || markerWidth <= 0 {
		markerWidth = 12
	}
	firstWidth := width
	nextWidth := width - markerWidth
	lines := r.splitTextCJKHanging(marker+value, firstWidth, nextWidth)
	r.ensureSpace(float64(len(lines)) * lineHeight)
	for i, line := range lines {
		r.ensureSpace(lineHeight)
		lineX := x
		if i > 0 {
			lineX = x + markerWidth
		}
		r.pdf.SetXY(lineX, r.pdf.GetY())
		cellWidth := firstWidth
		if i > 0 {
			cellWidth = nextWidth
		}
		if err := r.pdf.Cell(&gopdf.Rect{W: cellWidth, H: lineHeight}, line); err != nil {
			return err
		}
		r.pdf.Br(lineHeight)
	}
	return nil
}

func (r *pdfRenderer) writeListInline(marker string, spans []inlineSpan, x, width, lineHeight float64) error {
	markerWidth, err := r.measureInlineSpans([]inlineSpan{{text: marker}})
	if err != nil || markerWidth <= 0 {
		markerWidth = 12
	}
	all := append([]inlineSpan{{text: marker}}, spans...)
	lines := r.splitInlineSpansHanging(all, width, width-markerWidth)
	r.ensureSpace(float64(len(lines)) * lineHeight)
	for i, line := range lines {
		r.ensureSpace(lineHeight)
		lineX := x
		lineWidth := width
		if i > 0 {
			lineX = x + markerWidth
			lineWidth = width - markerWidth
		}
		r.pdf.SetXY(lineX, r.pdf.GetY())
		if err := r.drawInlineLine(line, lineWidth, lineHeight); err != nil {
			return err
		}
		r.pdf.Br(lineHeight)
	}
	_ = r.setFont(r.fontSize, false)
	return nil
}

func (r *pdfRenderer) drawInlineLine(spans []inlineSpan, width, lineHeight float64) error {
	for _, span := range spans {
		if span.text == "" {
			continue
		}
		if err := r.setFont(r.fontSize, span.bold); err != nil {
			return err
		}
		w, err := r.pdf.MeasureTextWidth(span.text)
		if err != nil || w <= 0 {
			w = width
		}
		if err := r.pdf.Cell(&gopdf.Rect{W: w, H: lineHeight}, span.text); err != nil {
			return err
		}
	}
	return nil
}

func (r *pdfRenderer) splitInlineSpans(spans []inlineSpan, width float64) [][]inlineSpan {
	return r.splitInlineSpansHanging(spans, width, width)
}

func (r *pdfRenderer) splitInlineSpansHanging(spans []inlineSpan, firstWidth, nextWidth float64) [][]inlineSpan {
	var lines [][]inlineSpan
	var current []inlineSpan
	lineWidth := firstWidth
	for _, span := range spans {
		for _, ch := range span.text {
			if ch == '\n' {
				lines = append(lines, trimInlineSpans(current))
				current = nil
				lineWidth = nextWidth
				continue
			}
			next := appendSpanCopy(current, inlineSpan{text: string(ch), bold: span.bold})
			w, err := r.measureInlineSpans(next)
			if err == nil && w > lineWidth && len(current) > 0 {
				lines = append(lines, trimInlineSpans(current))
				current = []inlineSpan{{text: string(ch), bold: span.bold}}
				lineWidth = nextWidth
				continue
			}
			current = next
		}
	}
	if len(current) > 0 {
		lines = append(lines, trimInlineSpans(current))
	}
	if len(lines) == 0 {
		return [][]inlineSpan{{}}
	}
	return lines
}

func appendSpanCopy(spans []inlineSpan, span inlineSpan) []inlineSpan {
	next := append([]inlineSpan{}, spans...)
	appendInlineSpan(&next, span.text, span.bold)
	return next
}

func (r *pdfRenderer) measureInlineSpans(spans []inlineSpan) (float64, error) {
	total := 0.0
	for _, span := range spans {
		if span.text == "" {
			continue
		}
		if err := r.setFont(r.fontSize, span.bold); err != nil {
			return total, err
		}
		w, err := r.pdf.MeasureTextWidth(span.text)
		if err != nil {
			return total, err
		}
		total += w
	}
	return total, nil
}

func (r *pdfRenderer) splitTextCJKHanging(value string, firstWidth, nextWidth float64) []string {
	runes := []rune(value)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	var current []rune
	lineWidth := firstWidth
	for _, ch := range runes {
		current = append(current, ch)
		w, err := r.pdf.MeasureTextWidth(string(current))
		if err != nil || w <= lineWidth {
			continue
		}
		if len(current) == 1 {
			lines = append(lines, string(current))
			current = nil
		} else {
			lines = append(lines, strings.TrimRight(string(current[:len(current)-1]), " \t"))
			current = append([]rune{}, current[len(current)-1])
		}
		lineWidth = nextWidth
	}
	if len(current) > 0 {
		lines = append(lines, strings.TrimRight(string(current), " \t"))
	}
	if len(lines) == 0 {
		return []string{value}
	}
	return lines
}

func (r *pdfRenderer) splitText(value string, width float64) []string {
	lines := []string{}
	for _, part := range strings.Split(value, "\n") {
		split := r.splitTextCJK(part, width)
		lines = append(lines, split...)
	}
	if len(lines) == 0 {
		return []string{""}
	}
	return lines
}

func (r *pdfRenderer) splitTextCJK(value string, width float64) []string {
	runes := []rune(value)
	if len(runes) == 0 {
		return []string{""}
	}
	var lines []string
	var current []rune
	lastBreak := -1
	for _, ch := range runes {
		current = append(current, ch)
		if ch == ' ' || ch == '\t' || ch == '-' || ch == '/' || ch == '，' || ch == '。' || ch == '、' || ch == '；' {
			lastBreak = len(current)
		}
		w, err := r.pdf.MeasureTextWidth(string(current))
		if err != nil || w <= width {
			continue
		}
		if len(current) == 1 {
			lines = append(lines, string(current))
			current = nil
			lastBreak = -1
			continue
		}
		breakAt := len(current) - 1
		if lastBreak > 0 && lastBreak < len(current)-1 && !containsCJK(current[:len(current)-1]) {
			breakAt = lastBreak
		}
		lines = append(lines, strings.TrimRight(string(current[:breakAt]), " \t"))
		remaining := append([]rune{}, current[breakAt:]...)
		current = remaining
		lastBreak = lastBreakIndex(current)
	}
	if len(current) > 0 {
		lines = append(lines, strings.TrimRight(string(current), " \t"))
	}
	if len(lines) == 0 {
		return []string{value}
	}
	return lines
}

func containsCJK(value []rune) bool {
	for _, r := range value {
		if (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0x3400 && r <= 0x4DBF) || (r >= 0xF900 && r <= 0xFAFF) {
			return true
		}
	}
	return false
}

func lastBreakIndex(value []rune) int {
	last := -1
	for i, ch := range value {
		if ch == ' ' || ch == '\t' || ch == '-' || ch == '/' || ch == '，' || ch == '。' || ch == '、' || ch == '；' {
			last = i + 1
		}
	}
	return last
}

func stripUnsupportedSymbols(value string) string {
	var out strings.Builder
	for _, r := range value {
		switch {
		case r == '\ufe0f' || r == '\ufe0e':
			continue
		case r == '★' || r == '☆' || r == '☐' || r == '☑':
			out.WriteRune(r)
		case isEmojiRune(r):
			continue
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

func isEmojiRune(r rune) bool {
	return (r >= 0x2600 && r <= 0x27BF) ||
		(r >= 0x1F300 && r <= 0x1FAFF) ||
		(r >= 0x1F1E6 && r <= 0x1F1FF) ||
		(r >= 0x1F3FB && r <= 0x1F3FF)
}

func (r *pdfRenderer) setFont(size float64, bold bool) error {
	r.fontSize = size
	r.bold = bold && r.boldReady
	family := pdfFontFamily
	if r.bold {
		family = pdfFontBold
	}
	return r.pdf.SetFont(family, "", size)
}

func (r *pdfRenderer) addSpace(height float64) {
	r.ensureSpace(height)
	r.pdf.SetY(r.pdf.GetY() + height)
}

func (r *pdfRenderer) ensureSpace(height float64) {
	if r.pdf.GetY()+height <= pdfPageHeight-pdfMarginBot {
		return
	}
	r.pdf.AddPage()
	r.pdf.SetXY(pdfMarginX, pdfMarginTop)
	_ = r.setFont(r.fontSize, r.bold)
}
