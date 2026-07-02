package store

// QueryResult is a generic tabular result: column names + rows of scalar values,
// with Truncated set when more rows existed than the limit allowed.
type QueryResult struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
}

const defaultQueryLimit = 1000

// Query runs a read-only SQL statement and returns up to limit rows. Writes are
// rejected by the engine (open the DB via OpenReadOnly). limit<=0 uses the default.
func (d *DB) Query(sql string, limit int) (*QueryResult, error) {
	if limit <= 0 {
		limit = defaultQueryLimit
	}
	rows, err := d.db.Query(sql)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := &QueryResult{Columns: cols, Rows: [][]any{}}
	for rows.Next() {
		if len(out.Rows) >= limit {
			out.Truncated = true
			break
		}
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		// []byte → string 로 정규화 (JSON 직렬화·가독성).
		for i, v := range vals {
			if b, ok := v.([]byte); ok {
				vals[i] = string(b)
			}
		}
		out.Rows = append(out.Rows, vals)
	}
	return out, rows.Err()
}
