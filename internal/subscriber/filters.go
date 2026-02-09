package subscriber

import (
	"encoding/json"
	"fmt"
	"strings"

	"mysql_changelog_publisher/internal/event"
)

type strset map[string]struct{}

func toSet(list []string, normalizeLower bool) strset {
	if len(list) == 0 {
		return nil
	}
	out := make(strset, len(list))
	for _, v := range list {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if normalizeLower {
			v = strings.ToLower(v)
		}
		out[v] = struct{}{}
	}
	return out
}

func inSet(s strset, v string) bool {
	if s == nil {
		return true // no filter
	}
	_, ok := s[v]
	return ok
}

func rowKeyToString(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		return fmt.Sprintf("%v", t)
	}
}

func hasAnyColumns(changes []event.ColumnChange, cols strset) bool {
	if cols == nil {
		return true
	}
	for _, c := range changes {
		if _, ok := cols[c.Column]; ok {
			return true
		}
	}
	return false
}

func hasAllColumns(changes []event.ColumnChange, cols strset) bool {
	if cols == nil || len(cols) == 0 {
		return true
	}
	seen := map[string]bool{}
	for _, c := range changes {
		seen[c.Column] = true
	}
	for col := range cols {
		if !seen[col] {
			return false
		}
	}
	return true
}
