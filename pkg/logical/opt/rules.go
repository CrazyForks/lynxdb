package opt

import (
	"fmt"
	"strconv"
	"time"

	"github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

// Literal constructors (used by all folding rules)

func litInt(v int64) *ast.Literal {
	raw := strconv.FormatInt(v, 10)
	return &ast.Literal{Kind: ast.LitInt, Raw: raw, Value: v}
}

func litFloat(v float64) *ast.Literal {
	raw := strconv.FormatFloat(v, 'g', -1, 64)
	return &ast.Literal{Kind: ast.LitFloat, Raw: raw, Value: v}
}

func litBool(v bool) *ast.Literal {
	raw := "false"
	if v {
		raw = "true"
	}
	return &ast.Literal{Kind: ast.LitBool, Raw: raw, Value: v}
}

func litString(v string) *ast.Literal {
	return &ast.Literal{Kind: ast.LitString, Raw: strconv.Quote(v), Value: v}
}

func litNull() *ast.Literal {
	return &ast.Literal{Kind: ast.LitNull, Raw: "null", Value: nil}
}

func litDuration(d time.Duration) *ast.Literal {
	raw := formatDuration(d)
	return &ast.Literal{Kind: ast.LitDuration, Raw: raw, Value: d}
}

// formatDuration renders a time.Duration in the most natural unit,
// matching the ast package's formatDuration.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return "0s"
	}
	neg := ""
	if d < 0 {
		neg = "-"
		d = -d
	}
	switch {
	case d%time.Hour == 0 && d >= time.Hour:
		return fmt.Sprintf("%s%dh", neg, d/time.Hour)
	case d%time.Minute == 0 && d >= time.Minute:
		return fmt.Sprintf("%s%dm", neg, d/time.Minute)
	case d%time.Second == 0 && d >= time.Second:
		return fmt.Sprintf("%s%ds", neg, d/time.Second)
	case d%time.Millisecond == 0 && d >= time.Millisecond:
		return fmt.Sprintf("%s%dms", neg, d/time.Millisecond)
	case d%time.Microsecond == 0 && d >= time.Microsecond:
		return fmt.Sprintf("%s%dus", neg, d/time.Microsecond)
	default:
		return fmt.Sprintf("%s%dns", neg, d/time.Nanosecond)
	}
}

// Literal type helpers

func asInt(e ast.Expr) (int64, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != ast.LitInt {
		return 0, false
	}
	v, ok := lit.Value.(int64)
	return v, ok
}

func asFloat(e ast.Expr) (float64, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != ast.LitFloat {
		return 0, false
	}
	v, ok := lit.Value.(float64)
	return v, ok
}

func asBool(e ast.Expr) (bool, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != ast.LitBool {
		return false, false
	}
	v, ok := lit.Value.(bool)
	return v, ok
}

func asString(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != ast.LitString {
		return "", false
	}
	v, ok := lit.Value.(string)
	return v, ok
}

func asDuration(e ast.Expr) (time.Duration, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != ast.LitDuration {
		return 0, false
	}
	v, ok := lit.Value.(time.Duration)
	return v, ok
}

func isNull(e ast.Expr) bool {
	lit, ok := e.(*ast.Literal)
	return ok && lit.Kind == ast.LitNull
}

func isLit(e ast.Expr) bool {
	_, ok := e.(*ast.Literal)
	return ok
}

// asNumber returns a float64 for either int or float literals.
func asNumber(e ast.Expr) (float64, bool) {
	if v, ok := asInt(e); ok {
		return float64(v), true
	}
	return asFloat(e)
}

// Rule 7: paren-strip

// parenStrip removes ast.Paren wrappers. The formatter re-derives parens from
// operator precedence; the IR does not need them.
func parenStrip(e ast.Expr) (ast.Expr, bool) {
	if p, ok := e.(*ast.Paren); ok {
		return p.Inner, true
	}
	return e, false
}

// Rule 1: const-fold-arith
//
// Folds literal op literal for arithmetic operators per RFC-002 §5.4:
//   - int op int -> int (division truncates: 5/2=2)
//   - int op float -> float (promote)
//   - division by zero -> null
//   - % is int-only (float % -> leave unfolded)
//   - string + string -> concatenation
//   - duration ± duration -> duration
//   - duration * number, number * duration -> duration
//   - duration / number -> duration
//   - duration / duration -> float
//   - Unary negation of numeric/duration literals.
//
// Mixed-type combos that are plan-time errors (string+int) are NOT folded
// (sema already diagnoses them; leave the expr).
func constFoldArith(e ast.Expr) (ast.Expr, bool) {
	// Unary negation of literal.
	if u, ok := e.(*ast.Unary); ok && u.Op == ast.OpNeg {
		if iv, ok := asInt(u.Operand); ok {
			return litInt(-iv), true
		}
		if fv, ok := asFloat(u.Operand); ok {
			return litFloat(-fv), true
		}
		if dv, ok := asDuration(u.Operand); ok {
			return litDuration(-dv), true
		}
		return e, false
	}

	b, ok := e.(*ast.Binary)
	if !ok {
		return e, false
	}

	switch b.Op {
	case ast.OpAdd:
		return foldAdd(b)
	case ast.OpSub:
		return foldSub(b)
	case ast.OpMul:
		return foldMul(b)
	case ast.OpDiv:
		return foldDiv(b)
	case ast.OpMod:
		return foldMod(b)
	}
	return e, false
}

func foldAdd(b *ast.Binary) (ast.Expr, bool) {
	// int + int -> int
	if lv, ok := asInt(b.Left); ok {
		if rv, ok := asInt(b.Right); ok {
			return litInt(lv + rv), true
		}
	}
	// int + float or float + int or float + float -> float
	if lv, lOk := asNumber(b.Left); lOk {
		if rv, rOk := asNumber(b.Right); rOk {
			// Only promote if at least one is float.
			_, lIsFloat := asFloat(b.Left)
			_, rIsFloat := asFloat(b.Right)
			if lIsFloat || rIsFloat {
				return litFloat(lv + rv), true
			}
		}
	}
	// string + string -> concatenation
	if lv, ok := asString(b.Left); ok {
		if rv, ok := asString(b.Right); ok {
			return litString(lv + rv), true
		}
	}
	// duration + duration -> duration
	if lv, ok := asDuration(b.Left); ok {
		if rv, ok := asDuration(b.Right); ok {
			return litDuration(lv + rv), true
		}
	}
	return b, false
}

func foldSub(b *ast.Binary) (ast.Expr, bool) {
	// int - int -> int
	if lv, ok := asInt(b.Left); ok {
		if rv, ok := asInt(b.Right); ok {
			return litInt(lv - rv), true
		}
	}
	// float involved -> float
	if lv, lOk := asNumber(b.Left); lOk {
		if rv, rOk := asNumber(b.Right); rOk {
			_, lIsFloat := asFloat(b.Left)
			_, rIsFloat := asFloat(b.Right)
			if lIsFloat || rIsFloat {
				return litFloat(lv - rv), true
			}
		}
	}
	// duration - duration -> duration
	if lv, ok := asDuration(b.Left); ok {
		if rv, ok := asDuration(b.Right); ok {
			return litDuration(lv - rv), true
		}
	}
	return b, false
}

func foldMul(b *ast.Binary) (ast.Expr, bool) {
	// int * int -> int
	if lv, ok := asInt(b.Left); ok {
		if rv, ok := asInt(b.Right); ok {
			return litInt(lv * rv), true
		}
	}
	// float involved -> float
	if lv, lOk := asNumber(b.Left); lOk {
		if rv, rOk := asNumber(b.Right); rOk {
			_, lIsFloat := asFloat(b.Left)
			_, rIsFloat := asFloat(b.Right)
			if lIsFloat || rIsFloat {
				return litFloat(lv * rv), true
			}
		}
	}
	// duration * number or number * duration -> duration
	if dv, ok := asDuration(b.Left); ok {
		if nv, ok := asNumber(b.Right); ok {
			return litDuration(time.Duration(float64(dv) * nv)), true
		}
	}
	if nv, ok := asNumber(b.Left); ok {
		if dv, ok := asDuration(b.Right); ok {
			return litDuration(time.Duration(nv * float64(dv))), true
		}
	}
	return b, false
}

func foldDiv(b *ast.Binary) (ast.Expr, bool) {
	// Division by zero -> null (for any numeric type).
	if rv, ok := asNumber(b.Right); ok && rv == 0 {
		if isLit(b.Left) {
			return litNull(), true
		}
	}
	if rv, ok := asDuration(b.Right); ok && rv == 0 {
		if isLit(b.Left) {
			return litNull(), true
		}
	}

	// int / int -> int (truncating) per §5.4
	if lv, ok := asInt(b.Left); ok {
		if rv, ok := asInt(b.Right); ok {
			if rv == 0 {
				return litNull(), true
			}
			// Go integer division truncates toward zero, matching §5.4.
			return litInt(lv / rv), true
		}
	}
	// float involved -> float
	if lv, lOk := asNumber(b.Left); lOk {
		if rv, rOk := asNumber(b.Right); rOk {
			_, lIsFloat := asFloat(b.Left)
			_, rIsFloat := asFloat(b.Right)
			if lIsFloat || rIsFloat {
				if rv == 0 {
					return litNull(), true
				}
				return litFloat(lv / rv), true
			}
		}
	}
	// duration / number -> duration
	if dv, ok := asDuration(b.Left); ok {
		if nv, ok := asNumber(b.Right); ok {
			if nv == 0 {
				return litNull(), true
			}
			return litDuration(time.Duration(float64(dv) / nv)), true
		}
	}
	// duration / duration -> float
	if lv, ok := asDuration(b.Left); ok {
		if rv, ok := asDuration(b.Right); ok {
			if rv == 0 {
				return litNull(), true
			}
			return litFloat(float64(lv) / float64(rv)), true
		}
	}
	return b, false
}

func foldMod(b *ast.Binary) (ast.Expr, bool) {
	// % is int-only per §5.4. Float % -> leave unfolded for runtime/sema.
	lv, lOk := asInt(b.Left)
	rv, rOk := asInt(b.Right)
	if !lOk || !rOk {
		return b, false
	}
	if rv == 0 {
		return litNull(), true
	}
	return litInt(lv % rv), true
}

// Rule 2: const-fold-compare
//
// Folds literal cmp literal -> true/false for same-type comparisons:
//   - string: lexical
//   - int/float: numeric (int promoted to float when mixed)
//   - bool: == and != only
//   - duration: numeric on underlying nanoseconds
//   - null: null == null -> true; null cmp non-null -> leave (runtime three-valued)
//
// Cross-type (e.g., string vs int) -> leave unfolded.
func constFoldCompare(e ast.Expr) (ast.Expr, bool) {
	b, ok := e.(*ast.Binary)
	if !ok {
		return e, false
	}

	switch b.Op {
	case ast.OpEq, ast.OpNotEq, ast.OpLt, ast.OpLtEq, ast.OpGt, ast.OpGtEq:
		// continue
	default:
		return e, false
	}

	lLit, lOk := b.Left.(*ast.Literal)
	rLit, rOk := b.Right.(*ast.Literal)
	if !lOk || !rOk {
		return e, false
	}

	// null == null -> true; null != null -> false;
	// null <relational> null -> leave (debatable, but consistent with three-valued).
	if lLit.Kind == ast.LitNull && rLit.Kind == ast.LitNull {
		switch b.Op {
		case ast.OpEq:
			return litBool(true), true
		case ast.OpNotEq:
			return litBool(false), true
		default:
			return e, false
		}
	}
	// Any comparison involving one null -> leave (three-valued at runtime).
	if lLit.Kind == ast.LitNull || rLit.Kind == ast.LitNull {
		return e, false
	}

	// Same-kind comparisons.
	if lLit.Kind != rLit.Kind {
		// Allow int vs float promotion.
		_, lIsInt := asInt(b.Left)
		_, lIsFloat := asFloat(b.Left)
		_, rIsInt := asInt(b.Right)
		_, rIsFloat := asFloat(b.Right)
		if (lIsInt || lIsFloat) && (rIsInt || rIsFloat) {
			lv, _ := asNumber(b.Left)
			rv, _ := asNumber(b.Right)
			return litBool(evalCmpFloat(b.Op, lv, rv)), true
		}
		// Cross-type -> leave.
		return e, false
	}

	switch lLit.Kind {
	case ast.LitInt:
		lv, _ := asInt(b.Left)
		rv, _ := asInt(b.Right)
		return litBool(evalCmpInt(b.Op, lv, rv)), true

	case ast.LitFloat:
		lv, _ := asFloat(b.Left)
		rv, _ := asFloat(b.Right)
		return litBool(evalCmpFloat(b.Op, lv, rv)), true

	case ast.LitString:
		lv, _ := asString(b.Left)
		rv, _ := asString(b.Right)
		return litBool(evalCmpString(b.Op, lv, rv)), true

	case ast.LitBool:
		lv, _ := asBool(b.Left)
		rv, _ := asBool(b.Right)
		switch b.Op {
		case ast.OpEq:
			return litBool(lv == rv), true
		case ast.OpNotEq:
			return litBool(lv != rv), true
		default:
			// Ordering on bools is not defined -> leave.
			return e, false
		}

	case ast.LitDuration:
		lv, _ := asDuration(b.Left)
		rv, _ := asDuration(b.Right)
		return litBool(evalCmpInt(b.Op, int64(lv), int64(rv))), true
	}

	return e, false
}

func evalCmpInt(op ast.BinaryOp, l, r int64) bool {
	switch op {
	case ast.OpEq:
		return l == r
	case ast.OpNotEq:
		return l != r
	case ast.OpLt:
		return l < r
	case ast.OpLtEq:
		return l <= r
	case ast.OpGt:
		return l > r
	case ast.OpGtEq:
		return l >= r
	}
	return false
}

func evalCmpFloat(op ast.BinaryOp, l, r float64) bool {
	switch op {
	case ast.OpEq:
		return l == r
	case ast.OpNotEq:
		return l != r
	case ast.OpLt:
		return l < r
	case ast.OpLtEq:
		return l <= r
	case ast.OpGt:
		return l > r
	case ast.OpGtEq:
		return l >= r
	}
	return false
}

func evalCmpString(op ast.BinaryOp, l, r string) bool {
	switch op {
	case ast.OpEq:
		return l == r
	case ast.OpNotEq:
		return l != r
	case ast.OpLt:
		return l < r
	case ast.OpLtEq:
		return l <= r
	case ast.OpGt:
		return l > r
	case ast.OpGtEq:
		return l >= r
	}
	return false
}

// Rule 3: bool-simplify (three-valued SOUND only)
//
// Applies absorption/identity laws that are sound under three-valued (null)
// logic per RFC-002 §5.2:
//
//	true and X  -> X
//	X and true  -> X
//	false and X -> false  (sound: null and false = false)
//	X and false -> false
//	true or X   -> true   (sound: null or true = true)
//	X or true   -> true
//	false or X  -> X
//	X or false  -> X
//	not true    -> false
//	not false   -> true
//	not not X   -> X
func boolSimplify(e ast.Expr) (ast.Expr, bool) {
	// not true -> false; not false -> true; not not X -> X
	if u, ok := e.(*ast.Unary); ok && u.Op == ast.OpNot {
		if v, ok := asBool(u.Operand); ok {
			return litBool(!v), true
		}
		// not not X -> X
		if inner, ok := u.Operand.(*ast.Unary); ok && inner.Op == ast.OpNot {
			return inner.Operand, true
		}
		return e, false
	}

	b, ok := e.(*ast.Binary)
	if !ok {
		return e, false
	}

	switch b.Op {
	case ast.OpAnd:
		// true and X -> X
		if v, ok := asBool(b.Left); ok && v {
			return b.Right, true
		}
		// X and true -> X
		if v, ok := asBool(b.Right); ok && v {
			return b.Left, true
		}
		// false and X -> false (sound under 3VL: null AND false = false)
		if v, ok := asBool(b.Left); ok && !v {
			return litBool(false), true
		}
		// X and false -> false
		if v, ok := asBool(b.Right); ok && !v {
			return litBool(false), true
		}

	case ast.OpOr:
		// true or X -> true (sound under 3VL: null OR true = true)
		if v, ok := asBool(b.Left); ok && v {
			return litBool(true), true
		}
		// X or true -> true
		if v, ok := asBool(b.Right); ok && v {
			return litBool(true), true
		}
		// false or X -> X
		if v, ok := asBool(b.Left); ok && !v {
			return b.Right, true
		}
		// X or false -> X
		if v, ok := asBool(b.Right); ok && !v {
			return b.Left, true
		}
	}

	return e, false
}

// Rule 4: coalesce-fold
//
// non-null-literal ?? X -> literal (the literal is known non-null)
// null ?? X -> X
func coalesceFold(e ast.Expr) (ast.Expr, bool) {
	b, ok := e.(*ast.Binary)
	if !ok || b.Op != ast.OpCoalesce {
		return e, false
	}

	lit, ok := b.Left.(*ast.Literal)
	if !ok {
		return e, false
	}

	if lit.Kind == ast.LitNull {
		// null ?? X -> X
		return b.Right, true
	}

	// Non-null literal ?? X -> literal.
	return b.Left, true
}

// Rule 5: if-fold
//
// if(true,  a, b) -> a
// if(false, a, b) -> b
// if(null,  a, b) -> null (§5.2: null condition yields null)
func ifFold(e ast.Expr) (ast.Expr, bool) {
	c, ok := e.(*ast.Call)
	if !ok || c.Callee != "if" || len(c.Args) != 3 {
		return e, false
	}

	cond := c.Args[0]

	if v, ok := asBool(cond); ok {
		if v {
			return c.Args[1], true
		}
		return c.Args[2], true
	}

	if isNull(cond) {
		return litNull(), true
	}

	return e, false
}

// Rule 6: cmp-normalize
//
// Literal on the left of a comparison is flipped to field-on-left so later
// pushdown rules see a canonical shape. Only fires when exactly one side is a
// literal.
//
//	5 < x   -> x > 5
//	"a" == x -> x == "a"
//
// Symmetric ops (== !=) keep the same operator; ordered ops are flipped.
func cmpNormalize(e ast.Expr) (ast.Expr, bool) {
	b, ok := e.(*ast.Binary)
	if !ok {
		return e, false
	}

	switch b.Op {
	case ast.OpEq, ast.OpNotEq, ast.OpLt, ast.OpLtEq, ast.OpGt, ast.OpGtEq:
		// continue
	default:
		return e, false
	}

	leftIsLit := isLit(b.Left)
	rightIsLit := isLit(b.Right)

	// Only flip when left is literal and right is not.
	if !leftIsLit || rightIsLit {
		return e, false
	}

	flipped := flipCmpOp(b.Op)
	return &ast.Binary{Op: flipped, Left: b.Right, Right: b.Left, Pos: b.Pos}, true
}

// flipCmpOp returns the comparison operator with operands swapped.
func flipCmpOp(op ast.BinaryOp) ast.BinaryOp {
	switch op {
	case ast.OpLt:
		return ast.OpGt
	case ast.OpLtEq:
		return ast.OpGtEq
	case ast.OpGt:
		return ast.OpLt
	case ast.OpGtEq:
		return ast.OpLtEq
	default:
		// == and != are symmetric.
		return op
	}
}
