package subscriber

import (
	"mysql_changelog_publisher/internal/event"
)

type Filter struct {
	dbSet        strset
	tableSet     strset
	idSet        strset
	opSet        strset
	changeAny    strset
	changeAll    strset
	excludeDBSet    strset
	excludeTableSet strset
}

func NewFilter(cfg *Config) *Filter {
	return &Filter{
		dbSet:           toSet(cfg.FilterDBs, false),
		tableSet:        toSet(cfg.FilterTables, false),
		idSet:           toSet(cfg.FilterIDs, false),
		opSet:           toSet(cfg.FilterOps, true),
		changeAny:       toSet(cfg.FilterChangeAny, false),
		changeAll:       toSet(cfg.FilterChangeAll, false),
		excludeDBSet:    toSet(cfg.ExcludeDBs, false),
		excludeTableSet: toSet(cfg.ExcludeTables, false),
	}
}

func (f *Filter) Matches(ev *event.RowEvent) bool {
	// Check exclude filters first (blacklist)
	if f.excludeDBSet != nil && len(f.excludeDBSet) > 0 {
		if _, excluded := f.excludeDBSet[ev.DB]; excluded {
			return false
		}
	}
	if f.excludeTableSet != nil && len(f.excludeTableSet) > 0 {
		if _, excluded := f.excludeTableSet[ev.Table]; excluded {
			return false
		}
	}

	// Check include filters (whitelist)
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
