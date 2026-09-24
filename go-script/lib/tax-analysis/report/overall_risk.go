package report

import "fmt"

const (
	overallMediumHigh      = "中高风险"
	overallCoverageLimited = "综合判断受数据覆盖限制"
	minimumRiskCoverage    = 0.70
)

type RiskAssessment struct {
	Level         string            `json:"level"`
	Total         int               `json:"totalIndicators"`
	Completed     int               `json:"completedIndicators"`
	Missing       int               `json:"missingIndicators"`
	NotApplicable int               `json:"notApplicableIndicators"`
	High          int               `json:"highIndicators"`
	Medium        int               `json:"mediumIndicators"`
	Low           int               `json:"lowIndicators"`
	Clear         int               `json:"clearIndicators"`
	Coverage      float64           `json:"coverage"`
	ThemeCounts   map[RiskLevel]int `json:"themeCounts"`
}

// assessOverallRisk counts each indicator once; theme counts only determine the
// overall grade. Missing evidence never counts as a completed clear indicator.
func assessOverallRisk(findings []Finding, evaluations []RuleEvaluation) RiskAssessment {
	a := RiskAssessment{ThemeCounts: riskCounts(deduplicateFindings(findings))}
	type state struct {
		missing, applicable, completed bool
		level                          RiskLevel
	}
	states := map[string]state{}
	rank := map[RiskLevel]int{RiskLow: 1, RiskMedium: 2, RiskHigh: 3}
	for _, e := range evaluations {
		if e.RuleID == "" {
			continue
		}
		s := states[e.RuleID]
		if e.Outcome != OutcomeNotApplicable {
			s.applicable = true
		}
		if e.Outcome == OutcomeDataInsufficient {
			s.missing = true
		}
		if e.Outcome == OutcomeClear {
			s.completed = true
		}
		if e.Outcome == OutcomeRisk && e.Finding != nil {
			s.completed = true
			if rank[e.Finding.Level] > rank[s.level] {
				s.level = e.Finding.Level
			}
		}
		states[e.RuleID] = s
	}
	for _, s := range states {
		a.Total++
		switch {
		case !s.applicable:
			a.NotApplicable++
		case s.missing || !s.completed:
			a.Missing++
		default:
			a.Completed++
			switch s.level {
			case RiskHigh:
				a.High++
			case RiskMedium:
				a.Medium++
			case RiskLow:
				a.Low++
			default:
				a.Clear++
			}
		}
	}
	if applicable := a.Total - a.NotApplicable; applicable > 0 {
		a.Coverage = float64(a.Completed) / float64(applicable)
	}
	if a.Coverage < minimumRiskCoverage {
		a.Level = overallCoverageLimited
		return a
	}
	high, medium := a.ThemeCounts[RiskHigh], a.ThemeCounts[RiskMedium]
	strongHigh, major := 0, false
	for _, f := range deduplicateFindings(findings) {
		if f.Level == RiskHigh && f.Confidence != "低" {
			strongHigh++
			major = major || f.MajorImpactEvidence != ""
		}
	}
	switch {
	case strongHigh >= 2 || major:
		a.Level = string(RiskHigh)
	case high >= 1 || medium >= 4:
		a.Level = overallMediumHigh
	case medium >= 2:
		a.Level = string(RiskMedium)
	default:
		a.Level = string(RiskLow)
	}
	return a
}

func (a RiskAssessment) summary() string {
	return fmt.Sprintf("本期设置风险检测指标%d项，完成%d项，待补充%d项，不适用%d项。已完成指标中高风险%d项、中风险%d项、低风险%d项、未见明显异常%d项；数据覆盖率%.2f%%。风险主题按主题去重。", a.Total, a.Completed, a.Missing, a.NotApplicable, a.High, a.Medium, a.Low, a.Clear, a.Coverage*100)
}
