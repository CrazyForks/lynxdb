package pipeline

import (
	"context"
	"regexp"
	"strings"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// MakemvIterator converts a single-value field into LynxDB's internal multivalue string.
type MakemvIterator struct {
	child       Iterator
	field       string
	delim       string
	tokenizer   string
	tokenizerRE *regexp.Regexp
	allowEmpty  bool
}

// NewMakemvIterator creates a streaming makemv operator.
func NewMakemvIterator(child Iterator, field, delim, tokenizer string, allowEmpty bool) (*MakemvIterator, error) {
	if delim == "" && tokenizer == "" {
		delim = " "
	}
	var tokenizerRE *regexp.Regexp
	if tokenizer != "" {
		re, err := regexp.Compile(tokenizer)
		if err != nil {
			return nil, err
		}
		tokenizerRE = re
	}

	return &MakemvIterator{
		child:       child,
		field:       field,
		delim:       delim,
		tokenizer:   tokenizer,
		tokenizerRE: tokenizerRE,
		allowEmpty:  allowEmpty,
	}, nil
}

func (m *MakemvIterator) Init(ctx context.Context) error {
	return m.child.Init(ctx)
}

func (m *MakemvIterator) Next(ctx context.Context) (*Batch, error) {
	batch, err := m.child.Next(ctx)
	if err != nil || batch == nil {
		return batch, err
	}

	col, ok := batch.Columns[m.field]
	if !ok {
		return batch, nil
	}
	for i := range col {
		col[i] = m.makeValue(col[i])
	}
	batch.Columns[m.field] = col

	return batch, nil
}

func (m *MakemvIterator) Close() error {
	return m.child.Close()
}

func (m *MakemvIterator) Schema() []FieldInfo {
	return m.child.Schema()
}

func (m *MakemvIterator) makeValue(v event.Value) event.Value {
	if v.IsNull() {
		return v
	}
	raw := v.String()
	if m.tokenizerRE != nil {
		return makemvTokenizerValue(raw, m.tokenizerRE, m.allowEmpty)
	}

	return makemvDelimitedValue(raw, m.delim, m.allowEmpty)
}

func makemvDelimitedValue(raw, delim string, allowEmpty bool) event.Value {
	if delim == "" {
		return event.StringValue(raw)
	}
	parts := strings.Split(raw, delim)
	values := filterMakemvValues(parts, allowEmpty)
	if len(values) == 0 {
		return event.StringValue("")
	}

	return event.StringValue(strings.Join(values, "|||"))
}

func makemvTokenizerValue(raw string, re *regexp.Regexp, allowEmpty bool) event.Value {
	matches := re.FindAllStringSubmatch(raw, -1)
	values := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) < 2 {
			continue
		}
		values = append(values, match[1])
	}
	values = filterMakemvValues(values, allowEmpty)
	if len(values) == 0 {
		return event.StringValue("")
	}

	return event.StringValue(strings.Join(values, "|||"))
}

func filterMakemvValues(values []string, allowEmpty bool) []string {
	if allowEmpty {
		return values
	}
	filtered := values[:0]
	for _, value := range values {
		if value != "" {
			filtered = append(filtered, value)
		}
	}

	return filtered
}
