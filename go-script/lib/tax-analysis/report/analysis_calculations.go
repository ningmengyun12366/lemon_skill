package report

import (
	"crypto/sha256"
	"fmt"
)

// CalculationInput identifies the exact source and presence of a derived value.
// TaxFieldID links a Chinese authority cell when one is available; other inputs
// retain their module/record path and business label.
type CalculationInput struct {
	TaxFieldID string  `json:"taxFieldId,omitempty"`
	Source     string  `json:"source"`
	Label      string  `json:"label"`
	Value      float64 `json:"value"`
	Present    bool    `json:"present"`
}

type TaxCalculation struct {
	ID        string             `json:"id"`
	Section   string             `json:"section"`
	Period    string             `json:"period"`
	Name      string             `json:"name"`
	Algorithm string             `json:"algorithm"`
	Inputs    []CalculationInput `json:"inputs"`
	Value     float64            `json:"value"`
	Status    string             `json:"status"`
}

// recordCalculation stores the same result that the business table consumes.
// Missing inputs stay missing, even when their zero-valued Go fields exist.
func (ctx *reportContext) recordCalculation(section, period, name, algorithm string, inputs []CalculationInput, value float64, present bool) {
	status := "missing"
	if present {
		status = "present"
	}
	id := fmt.Sprintf("calculation-%x", sha256.Sum256([]byte(section+"|"+period+"|"+name)))
	for _, old := range ctx.calculations {
		if old.ID == id {
			return
		}
	}
	ctx.calculations = append(ctx.calculations, TaxCalculation{ID: id, Section: section, Period: period, Name: name, Algorithm: algorithm, Inputs: inputs, Value: value, Status: status})
}

func taxCalculationInput(fact TaxFieldFact) CalculationInput {
	return CalculationInput{TaxFieldID: fact.ID, Source: fact.Path, Label: fact.RowLabel + " · " + fact.ColumnLabel, Value: fact.Value, Present: fact.Status == "present"}
}
