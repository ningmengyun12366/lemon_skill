package fetchrisk

import "fmt"

func intFrom(value any) int {
	switch v := value.(type) {
	case int:
		return v
	case int64:
		return int(v)
	case float64:
		return int(v)
	case string:
		var out int
		_, _ = fmt.Sscan(v, &out)
		return out
	default:
		return 0
	}
}

func firstNonNil(values ...any) any {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func anySlice(value any) []any {
	if items, ok := value.([]any); ok {
		return items
	}
	return []any{}
}
