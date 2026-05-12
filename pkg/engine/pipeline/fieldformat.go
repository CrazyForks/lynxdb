package pipeline

import "context"

// FieldformatIterator preserves row values while carrying a fieldformat stage.
//
// Splunk fieldformat changes rendered output, not the underlying field value.
// LynxDB rows currently have no separate render-metadata channel, so execution
// leaves batches unchanged after the parser validates expression syntax.
type FieldformatIterator struct {
	child Iterator
}

// NewFieldformatIterator creates a streaming display-only fieldformat operator.
func NewFieldformatIterator(child Iterator) *FieldformatIterator {
	return &FieldformatIterator{child: child}
}

func (f *FieldformatIterator) Init(ctx context.Context) error {
	return f.child.Init(ctx)
}

func (f *FieldformatIterator) Next(ctx context.Context) (*Batch, error) {
	return f.child.Next(ctx)
}

func (f *FieldformatIterator) Close() error {
	return f.child.Close()
}

func (f *FieldformatIterator) Schema() []FieldInfo {
	return f.child.Schema()
}
