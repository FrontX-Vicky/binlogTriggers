package subscriber

import (
	"mysql_changelog_publisher/internal/event"
)

type Filter struct {
	dbSet     map[string]struct{}
	tableSet  map[string]struct{}
	idSet     map[interface{}]struct{}
	opSet     map[string]struct{}
	changeAny map[string]struct{}
	changeAll map[string]struct{}
}

func NewFilter(cfg *Config) *Filter {
	return &Filter{
		dbSet:     toSet(cfg.FilterDBs),
		tableSet:  toSet(cfg.FilterTables),
		idSet:     toInterfaceSet(cfg.FilterIDs),
		opSet:     toSet(cfg.FilterOps),
		changeAny: toSet(cfg.FilterChangeAny),
		changeAll: toSet(cfg.FilterChangeAll),
	}
}

func (f *Filter) Matches(ev *event.RowEvent) bool {
	if len(f.dbSet) > 0 && !inSet(ev.DB, f.dbSet) {
		return false
	}
	if len(f.tableSet) > 0 && !inSet(ev.Table, f.tableSet) {
		return false
	}
	if len(f.idSet) > 0 {
		k := rowKeyToInterface(ev.RowKey)
		if !inInterfaceSet(k, f.idSet) {
			return false
		}
	}
	if len(f.opSet) > 0 && !inSet(ev.Op, f.opSet) {
		return false
	}
	if len(f.changeAny) > 0 && !hasAnyColumns(ev.Changes, f.changeAny) {
		return false
	}
	if len(f.changeAll) > 0 && !hasAllColumns(ev.Changes, f.changeAll) {
		return false
	}
	return true
}

func toInterfaceSet(arr []interface{}) map[interface{}]struct{} {
	m := make(map[interface{}]struct{}, len(arr))
	for _, v := range arr {
		m[v] = struct{}{}
	}
	return m
}

func inInterfaceSet(val interface{}, s map[interface{}]struct{}) bool {
	_, ok := s[val]
	return ok
}

func rowKeyToInterface(k interface{}) interface{} {
	return k
}
