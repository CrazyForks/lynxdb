// Package pipeline vec_plan.go defines vectorized filter plan node types.
// RFC-002 Phase 10: the analyzeVecExpr function that built vec trees from
// spl2 AST has been removed. The lynxflow physical builder constructs vec
// plans directly from logical.Filter nodes.
package pipeline

import "github.com/lynxbase/lynxdb/pkg/event"

// vecNode is a node in the vectorized filter execution plan tree.
type vecNode interface {
	evalBitmap(batch *Batch) ([]bool, bool)
}

type vecCompareNode struct {
	field string
	op    string
	value string
}

type vecInNode struct {
	field    string
	negated  bool
	intSet   map[int64]struct{}
	floatSet map[float64]struct{}
	strSet   map[string]struct{}
}

type vecNullCheckNode struct {
	field    string
	wantNull bool
}

type vecLikeNode struct {
	field   string
	pattern string
	kind    string
	literal string
}

type vecRangeNode struct {
	field  string
	minVal string
	maxVal string
	minOp  string
	maxOp  string
}

type vecAndNode struct {
	left  vecNode
	right vecNode
}

type vecOrNode struct {
	left  vecNode
	right vecNode
}

type vecNotNode struct {
	child vecNode
}

func detectColumnType(col []event.Value) event.FieldType {
	for _, v := range col {
		if !v.IsNull() {
			return v.Type()
		}
	}
	return event.FieldTypeNull
}

func extractInt64Column(col []event.Value) ([]int64, []bool) {
	n := len(col)
	out := make([]int64, n)
	nulls := make([]bool, n)
	for i, v := range col {
		if v.IsNull() {
			nulls[i] = true
		} else {
			out[i], _ = v.TryAsInt()
		}
	}
	return out, nulls
}

func extractFloat64Column(col []event.Value) ([]float64, []bool) {
	n := len(col)
	out := make([]float64, n)
	nulls := make([]bool, n)
	for i, v := range col {
		if v.IsNull() {
			nulls[i] = true
		} else {
			out[i], _ = v.TryAsFloat()
		}
	}
	return out, nulls
}

func extractStringColumn(col []event.Value) ([]string, []bool) {
	n := len(col)
	out := make([]string, n)
	nulls := make([]bool, n)
	for i, v := range col {
		if v.IsNull() {
			nulls[i] = true
		} else {
			out[i], _ = v.TryAsString()
		}
	}
	return out, nulls
}

func applyNullMask(bitmap, nullMask []bool) {
	for i, isNull := range nullMask {
		if isNull && i < len(bitmap) {
			bitmap[i] = false
		}
	}
}
