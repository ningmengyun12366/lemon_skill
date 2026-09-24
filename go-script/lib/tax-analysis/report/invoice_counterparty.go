package report

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

// invoicePartyName only folds width and whitespace; it never performs fuzzy matching.
func invoicePartyName(value string) string {
	return strings.ToLower(strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, norm.NFKC.String(value)))
}

func invoicePartyTaxID(value string) string {
	value = strings.ToUpper(invoicePartyName(value))
	switch value {
	case "", "-", "—", "无", "NULL", "N/A":
		return ""
	}
	return value
}

// invoicePartyKeys resolves a missing tax ID only through a unique exact normalized
// name within the complete period. Conflicting tax IDs remain distinct entities.
func invoicePartyKeys(groups []invoiceGroup) []string {
	names := map[string]map[string]bool{}
	for _, group := range groups {
		name, id := invoicePartyName(group.Counterparty), invoicePartyTaxID(group.CounterpartyID)
		if name == "" || id == "" {
			continue
		}
		if names[name] == nil {
			names[name] = map[string]bool{}
		}
		names[name][id] = true
	}
	keys := make([]string, len(groups))
	for i, group := range groups {
		name, id := invoicePartyName(group.Counterparty), invoicePartyTaxID(group.CounterpartyID)
		if id == "" && len(names[name]) == 1 {
			for candidate := range names[name] {
				id = candidate
			}
		}
		if id != "" {
			keys[i] = "id:" + id
		} else if name != "" {
			keys[i] = "name:" + name
		}
	}
	return keys
}

// invoiceCounterpartyItems is the same signed-net ranking contract used by OpenTax.
func invoiceCounterpartyItems(groups []invoiceGroup) []counterpartyItem {
	byKey := map[string]*counterpartyItem{}
	keys := invoicePartyKeys(groups)
	for i, group := range groups {
		key := keys[i]
		if !group.HasAmount || key == "" {
			continue
		}
		item := byKey[key]
		if item == nil {
			id := ""
			if strings.HasPrefix(key, "id:") {
				id = strings.TrimPrefix(key, "id:")
			}
			item = &counterpartyItem{Name: textOr(strings.TrimSpace(group.Counterparty), id), TaxID: id}
			byKey[key] = item
		}
		item.Amount += group.Amount
	}
	orderedKeys := make([]string, 0, len(byKey))
	for key, item := range byKey {
		if item.Amount > 0 {
			orderedKeys = append(orderedKeys, key)
		}
	}
	sort.Slice(orderedKeys, func(i, j int) bool {
		a, b := orderedKeys[i], orderedKeys[j]
		if byKey[a].Amount != byKey[b].Amount {
			return byKey[a].Amount > byKey[b].Amount
		}
		return a < b
	})
	items := make([]counterpartyItem, 0, len(orderedKeys))
	for _, key := range orderedKeys {
		items = append(items, *byKey[key])
	}
	return items
}

// invoiceNetAmount includes every signed amount, even an unidentified or net
// negative party. Filtering the ranking must never change the denominator.
func invoiceNetAmount(groups []invoiceGroup) (total float64) {
	for _, group := range groups {
		if group.HasAmount {
			total += group.Amount
		}
	}
	return
}
