package event

type RowEvent struct {
	Op        string                 `json:"op"`
	Timestamp string                 `json:"timestamp"`
	DB        string                 `json:"db"`
	Table     string                 `json:"table"`
	RowKey    interface{}            `json:"row_key"`
	After     map[string]interface{} `json:"after,omitempty"`
	Before    map[string]interface{} `json:"before,omitempty"`
	Changes   []ColumnChange         `json:"changes,omitempty"`
	Tombstone bool                   `json:"tombstone,omitempty"`
}

type ColumnChange struct {
	Column string      `json:"column"`
	From   interface{} `json:"from"`
	To     interface{} `json:"to"`
}
