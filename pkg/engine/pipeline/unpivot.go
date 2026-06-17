package pipeline

import (
	"context"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// UnpivotIterator converts selected wide fields into name/value rows.
type UnpivotIterator struct {
	child      Iterator
	fields     []string
	nameField  string
	valueField string
	batchSize  int
	pending    []map[string]event.Value
	offset     int
}

// NewUnpivotIterator creates an iterator for explicit unpivot fields.
func NewUnpivotIterator(child Iterator, fields []string, nameField, valueField string, batchSize int) *UnpivotIterator {
	if batchSize <= 0 {
		batchSize = DefaultBatchSize
	}

	return &UnpivotIterator{
		child:      child,
		fields:     append([]string(nil), fields...),
		nameField:  nameField,
		valueField: valueField,
		batchSize:  batchSize,
	}
}

func (u *UnpivotIterator) Init(ctx context.Context) error {
	return u.child.Init(ctx)
}

func (u *UnpivotIterator) Next(ctx context.Context) (*Batch, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if u.offset < len(u.pending) {
		return u.emitPending(), nil
	}

	for {
		batch, err := u.child.Next(ctx)
		if err != nil {
			return nil, err
		}
		if batch == nil {
			return nil, nil
		}

		u.pending = u.unpivotBatch(batch)
		u.offset = 0
		if len(u.pending) > 0 {
			return u.emitPending(), nil
		}
	}
}

func (u *UnpivotIterator) Close() error {
	return u.child.Close()
}

func (u *UnpivotIterator) Schema() []FieldInfo {
	fields := u.child.Schema()
	selected := make(map[string]struct{}, len(u.fields))
	for _, field := range u.fields {
		selected[field] = struct{}{}
	}
	out := make([]FieldInfo, 0, len(fields)+2)
	for _, field := range fields {
		if _, ok := selected[field.Name]; ok || field.Name == u.nameField || field.Name == u.valueField {
			continue
		}
		out = append(out, field)
	}
	out = append(out,
		FieldInfo{Name: u.nameField, Type: "string"},
		FieldInfo{Name: u.valueField, Type: "any"},
	)
	return out
}

func (u *UnpivotIterator) emitPending() *Batch {
	end := u.offset + u.batchSize
	if end > len(u.pending) {
		end = len(u.pending)
	}
	batch := BatchFromRows(u.pending[u.offset:end])
	u.offset = end
	if u.offset >= len(u.pending) {
		u.pending = nil
		u.offset = 0
	}

	return batch
}

func (u *UnpivotIterator) unpivotBatch(batch *Batch) []map[string]event.Value {
	if len(u.fields) == 0 {
		return nil
	}
	selected := make(map[string]struct{}, len(u.fields))
	for _, field := range u.fields {
		selected[field] = struct{}{}
	}

	columns := batch.ColumnNames()
	rows := make([]map[string]event.Value, 0, batch.Len*len(u.fields))
	for i := 0; i < batch.Len; i++ {
		base := make(map[string]event.Value, len(columns)+2)
		for _, column := range columns {
			if _, ok := selected[column]; ok || column == u.nameField || column == u.valueField {
				continue
			}
			base[column] = batch.Value(column, i)
		}
		for _, field := range u.fields {
			row := make(map[string]event.Value, len(base)+2)
			for key, value := range base {
				row[key] = value
			}
			row[u.nameField] = event.StringValue(field)
			row[u.valueField] = batch.Value(field, i)
			rows = append(rows, row)
		}
	}

	return rows
}
