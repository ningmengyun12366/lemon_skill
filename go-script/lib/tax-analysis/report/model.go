package report

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type RiskLevel string

const (
	RiskHigh   RiskLevel = "高风险"
	RiskMedium RiskLevel = "中风险"
	RiskLow    RiskLevel = "低风险"
)

type EvaluationOutcome string

const (
	OutcomeRisk             EvaluationOutcome = "risk"
	OutcomeClear            EvaluationOutcome = "clear"
	OutcomeNotApplicable    EvaluationOutcome = "not_applicable"
	OutcomeDataInsufficient EvaluationOutcome = "data_insufficient"
)

type OmissionReason string

const (
	OmittedNoRisk             OmissionReason = "no_risk"
	OmittedNotApplicable      OmissionReason = "not_applicable"
	OmittedDataInsufficient   OmissionReason = "data_insufficient"
	OmittedDependencyDeferred OmissionReason = "dependency_deferred"
)

type BlockKind string

const (
	BlockParagraph BlockKind = "paragraph"
	BlockTable     BlockKind = "table"
	BlockChart     BlockKind = "chart"
	BlockRiskCard  BlockKind = "risk_card"
	BlockRiskBand  BlockKind = "risk_band"
)

// NarrativeMode is the script-owned presentation boundary for Agent-authored
// text. The Agent may fill text only for sections whose mode is not "none";
// it cannot turn a fact, attention item, or data gap into a risk.
type NarrativeMode string

const (
	NarrativeNone          NarrativeMode = "none"
	NarrativeRisk          NarrativeMode = "RISK"
	NarrativeFact          NarrativeMode = "DESCRIPTION"
	NarrativeAttention     NarrativeMode = "ATTENTION"
	NarrativeDataGap       NarrativeMode = "DATA_INSUFFICIENT"
	NarrativeNotApplicable NarrativeMode = "NOT_APPLICABLE"
	NarrativeNoIssue       NarrativeMode = "NO_ISSUE"
)

// NarrativePolicy defines how many concise Agent analysis items a section may
// contain. Values are assigned by deterministic code after section data and
// rule outcomes are finalized.
type NarrativePolicy struct {
	Mode     NarrativeMode `json:"mode"`
	MinItems int           `json:"minItems"`
	MaxItems int           `json:"maxItems"`
}

type Model struct {
	FilingEvents      []FilingEvent              `json:"filingEvents,omitempty"`
	RiskAssessment    RiskAssessment             `json:"riskAssessment"`
	AnalysisSources   []SectionAnalysisData      `json:"analysisSources,omitempty"`
	InvoiceCoverage   map[string]InvoiceCoverage `json:"invoiceCoverage,omitempty"`
	Calculations      []TaxCalculation           `json:"calculations,omitempty"`
	TaxFields         []TaxFieldFact             `json:"taxFields,omitempty"`
	FixtureOnly       bool                       `json:"fixtureOnly,omitempty"`
	CompanyName       string                     `json:"companyName"`
	USCC              string                     `json:"uscc"`
	StartTime         string                     `json:"startTime"`
	EndTime           string                     `json:"endTime"`
	GeneratedAt       time.Time                  `json:"generatedAt"`
	RuleVersion       string                     `json:"ruleVersion"`
	TaskID            string                     `json:"taskId"`
	Sections          []Section                  `json:"sections"`
	Findings          []Finding                  `json:"findings"`
	Evaluations       []RuleEvaluation           `json:"evaluations"`
	ExecutedRuleCount int                        `json:"executedRuleCount"`
	Warnings          []string                   `json:"warnings"`
	OmittedReasons    map[string]OmissionReason  `json:"omittedReasons"`
}

type Section struct {
	DataNotice    string          `json:"dataNotice,omitempty"`
	SampleOnly    bool            `json:"sampleOnly,omitempty"`
	FindingIDs    []string        `json:"findingIds,omitempty"`
	Key           string          `json:"semanticId"`
	ID            string          `json:"sourceSectionId"`
	DisplayNumber string          `json:"displayNumber"`
	Level         int             `json:"level"`
	BaseTitle     string          `json:"baseTitle"`
	Title         string          `json:"title"`
	Bookmark      string          `json:"bookmark"`
	ModuleIDs     []string        `json:"moduleIds"`
	RuleIDs       []string        `json:"ruleIds"`
	Narrative     NarrativePolicy `json:"narrative"`
	Blocks        []Block         `json:"blocks"`
}

type Block struct {
	Kind         BlockKind    `json:"kind"`
	TableVariant TableVariant `json:"tableVariant,omitempty"`
	Title        string       `json:"title,omitempty"`
	Paragraphs   []string     `json:"paragraphs,omitempty"`
	Headers      []string     `json:"headers,omitempty"`
	Rows         [][]string   `json:"rows,omitempty"`
	Chart        *Chart       `json:"chart,omitempty"`
	Finding      *Finding     `json:"finding,omitempty"`
}

type Chart struct {
	ID             string       `json:"id"`
	Kind           string       `json:"kind"`
	Title          string       `json:"title"`
	Labels         []string     `json:"labels"`
	Series         []Series     `json:"series"`
	Points         []ChartPoint `json:"points,omitempty"`
	Unit           string       `json:"unit,omitempty"`
	SecondaryUnit  string       `json:"secondaryUnit,omitempty"`
	CenterLabel    string       `json:"centerLabel,omitempty"`
	CenterValue    string       `json:"centerValue,omitempty"`
	EmptyTitle     string       `json:"emptyTitle,omitempty"`
	EmptyMessage   string       `json:"emptyMessage,omitempty"`
	ReferenceLabel string       `json:"referenceLabel,omitempty"`
	Summary        string       `json:"summary"`
}

type Series struct {
	Name      string    `json:"name"`
	Values    []float64 `json:"values"`
	Available []bool    `json:"available,omitempty"`
	Color     string    `json:"color"`
	Colors    []string  `json:"colors,omitempty"`
	Axis      string    `json:"axis,omitempty"`
	RenderAs  string    `json:"renderAs,omitempty"`
}

// ChartPoint is one explicitly positioned item used by scatter and priority
// matrix charts. X and Y are normalized business values; the renderer performs
// all pixel positioning.
type ChartPoint struct {
	Label     string  `json:"label"`
	X         float64 `json:"x"`
	Y         float64 `json:"y"`
	Level     string  `json:"level,omitempty"`
	Color     string  `json:"color,omitempty"`
	Available bool    `json:"available"`
}

type Finding struct {
	MajorImpactEvidence  string           `json:"majorImpactEvidence,omitempty"`
	ID                   string           `json:"id"`
	RiskThemeID          string           `json:"riskThemeId"`
	Title                string           `json:"title"`
	Level                RiskLevel        `json:"level"`
	ModuleCode           string           `json:"moduleCode"`
	Period               string           `json:"period"`
	Periods              []string         `json:"periods,omitempty"`
	Occurrences          []RiskOccurrence `json:"occurrences,omitempty"`
	Evidence             string           `json:"evidence"`
	Recommendation       string           `json:"recommendation"`
	Confidence           string           `json:"confidence"`
	VerificationRequired bool             `json:"verificationRequired"`
	Amount               float64          `json:"amount"`
	SignalTypes          []string         `json:"signalTypes,omitempty"`
}

// RiskOccurrence retains one source event for additive quarterly risk statistics.
type RiskOccurrence struct {
	Level     RiskLevel `json:"level,omitempty"`
	Start     string    `json:"start,omitempty"`
	End       string    `json:"end,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Period    string    `json:"period"`
	Count     int       `json:"count"`
	Amount    float64   `json:"amount,omitempty"`
	HasAmount bool      `json:"hasAmount,omitempty"`
}

type RuleEvaluation struct {
	RuleID   string            `json:"ruleId"`
	Outcome  EvaluationOutcome `json:"outcome"`
	Finding  *Finding          `json:"finding,omitempty"`
	Evidence string            `json:"evidence"`
}

type ModuleSnapshot struct {
	Code        string
	Name        string
	UpdatedAt   string
	RecordCount int
	Facts       []Fact
	Raw         any
	Complete    bool
}

type Fact struct {
	Path  string
	Label string
	Value string
}

type Result struct {
	Reused           bool             `json:"reused"`
	ResultRecordPath string           `json:"resultRecordPath,omitempty"`
	Timings          map[string]int64 `json:"timingsMilliseconds,omitempty"`
	Status           string           `json:"status"`
	TaskID           string           `json:"taskId"`
	DataStatus       string           `json:"dataStatus"`
	RuleVersion      string           `json:"ruleVersion"`
	DOCXPath         string           `json:"docxPath"`
	PDFPath          string           `json:"pdfPath"`
	IncludedSections []string         `json:"includedSections"`
	OmittedSections  []string         `json:"omittedSections"`
	Warnings         []string         `json:"warnings"`
}

func (m Model) SectionIDs() []string {
	ids := make([]string, 0, len(m.Sections))
	for _, section := range m.Sections {
		ids = append(ids, section.ID)
	}
	return ids
}

func (m Model) CanonicalText() string {
	var builder strings.Builder
	writeLine := func(values ...any) {
		for index, value := range values {
			if index > 0 {
				builder.WriteString("\t")
			}
			builder.WriteString(strings.TrimSpace(fmt.Sprint(value)))
		}
		builder.WriteByte('\n')
	}
	writeLine(m.CompanyName, m.USCC, m.StartTime, m.EndTime, m.RuleVersion)
	for _, section := range m.Sections {
		writeLine(section.Key, section.ID, section.DisplayNumber, section.Title, strings.Join(section.ModuleIDs, "|"), strings.Join(section.RuleIDs, "|"))
		for _, block := range section.Blocks {
			writeLine(block.Kind, block.TableVariant, block.Title)
			for _, paragraph := range block.Paragraphs {
				writeLine(paragraph)
			}
			if len(block.Headers) > 0 {
				values := make([]any, len(block.Headers))
				for index := range block.Headers {
					values[index] = block.Headers[index]
				}
				writeLine(values...)
			}
			for _, row := range block.Rows {
				values := make([]any, len(row))
				for index := range row {
					values[index] = row[index]
				}
				writeLine(values...)
			}
			if block.Chart != nil {
				writeLine(block.Chart.ID, block.Chart.Kind, block.Chart.Title, block.Chart.Unit, block.Chart.SecondaryUnit, block.Chart.CenterLabel, block.Chart.CenterValue, block.Chart.EmptyTitle, block.Chart.EmptyMessage, block.Chart.ReferenceLabel, block.Chart.Summary)
				writeLine(strings.Join(block.Chart.Labels, "|"))
				for _, series := range block.Chart.Series {
					writeLine(series.Name, series.Axis, series.RenderAs, series.Color, strings.Join(series.Colors, "|"), floatsText(series.Values, series.Available))
				}
				for _, point := range block.Chart.Points {
					writeLine(point.Label, point.X, point.Y, point.Level, point.Color, point.Available)
				}
			}
		}
	}
	return builder.String()
}

func floatsText(values []float64, available []bool) string {
	parts := make([]string, len(values))
	for index, value := range values {
		if len(available) == len(values) && !available[index] {
			parts[index] = "—"
		} else {
			parts[index] = fmt.Sprintf("%.4f", value)
		}
	}
	return strings.Join(parts, "|")
}

func sortedFindings(findings []Finding) []Finding {
	copyOf := append([]Finding(nil), findings...)
	priority := map[RiskLevel]int{RiskHigh: 0, RiskMedium: 1, RiskLow: 2}
	sort.SliceStable(copyOf, func(i, j int) bool {
		left, right := priority[copyOf[i].Level], priority[copyOf[j].Level]
		if left != right {
			return left < right
		}
		if copyOf[i].Amount != copyOf[j].Amount {
			return copyOf[i].Amount > copyOf[j].Amount
		}
		return copyOf[i].ID < copyOf[j].ID
	})
	return copyOf
}
