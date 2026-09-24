package report

// quarterlyRiskOccurrences aggregates source events, never cumulative period totals.
// A missing historical period is not interpreted as recovery or as zero risk.
func quarterlyRiskOccurrences(finding Finding, start, end string) map[string]RiskOccurrence {
	occurrences := finding.Occurrences
	if len(occurrences) == 0 {
		for _, period := range uniqueSorted(finding.Periods) {
			occurrences = append(occurrences, RiskOccurrence{Period: period, Count: 1})
		}
	}
	result := map[string]RiskOccurrence{}
	for _, occurrence := range occurrences {
		month := extractMonth(occurrence.Period)
		from, to := occurrence.Start, occurrence.End
		if from == "" || to == "" {
			from, to = periodBounds(occurrence.Period)
		}
		if from != "" && to != "" {
			if quarterlyPeriodLabel(extractMonth(from), start, end) != quarterlyPeriodLabel(extractMonth(to), start, end) {
				continue
			}
			month = extractMonth(to)
		}
		if month == "" || month < extractMonth(start) || month > extractMonth(end) {
			continue
		}
		quarter := quarterlyPeriodLabel(month, start, end)
		current := result[quarter]
		current.Period = quarter
		if occurrence.Level != "" {
			current.Level = occurrence.Level
		}
		current.Count += occurrence.Count
		current.Amount += occurrence.Amount
		if current.Count == occurrence.Count {
			current.HasAmount = occurrence.HasAmount
		} else {
			current.HasAmount = current.HasAmount && occurrence.HasAmount
		}
		result[quarter] = current
	}
	return result
}

func riskOccurrencesFromRows(rows []map[string]any, amountKeys ...string) []RiskOccurrence {
	result := make([]RiskOccurrence, 0, len(rows))
	for _, row := range rows {
		value, known := mapNumber(row, amountKeys...)
		period := recordPeriod(row)
		if sourcePeriod, ok := taxPeriodFromRecord(row, false); ok {
			period = sourcePeriod.Display
		}
		from, to := periodBounds(period)
		result = append(result, RiskOccurrence{Period: period, Start: from, End: to, Kind: "event", Count: 1, Amount: value, HasAmount: known})
	}
	return result
}
