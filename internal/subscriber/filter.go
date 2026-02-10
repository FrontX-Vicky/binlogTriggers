package subscriber

import (
	"mysql_changelog_publisher/internal/event"
)

type Filter struct {
	dbSet     strset
	tableSet  strset
	idSet     strset
	opSet     strset
	changeAny strset
	changeAll strset
}

func NewFilter(cfg *Config) *Filter {
	return &Filter{
		dbSet:     toSet(cfg.FilterDBs, false),
		tableSet:  toSet(cfg.FilterTables, false),
		idSet:     toSet(cfg.FilterIDs, false),
		opSet:     toSet(cfg.FilterOps, true),
		changeAny: toSet(cfg.FilterChangeAny, false),
		changeAll: toSet(cfg.FilterChangeAll, false),
	}
}

func (f *Filter) Matches(ev *event.RowEvent) bool {
	if !inSet(f.dbSet, ev.DB) {
		return false
	}
	if !inSet(f.tableSet, ev.Table) {
		return false
	}
	if !inSet(f.idSet, rowKeyToString(ev.RowKey)) {
		return false
	}
	if !inSet(f.opSet, ev.Op) {
		return false
	}
	if !hasAnyColumns(ev.Changes, f.changeAny) {
		return false
	}
	if !hasAllColumns(ev.Changes, f.changeAll) {
		return false
	}
	return true
}
