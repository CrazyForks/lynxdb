package vm

// RFC-002 LynxFlow strict-semantics opcode runtime implementations.
//
// These functions implement the new opcodes added for the LynxFlow v2 compiler.
// They follow RFC-002 §5.2 (3VL logic), §5.4 (arithmetic/comparison rules),
// and §5.5 (case sensitivity). The old SPL2 opcodes are untouched.

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode"

	"github.com/lynxbase/lynxdb/pkg/event"
)

// ---------------------------------------------------------------------------
// Strict comparison (RFC-002 §5.4)
// ---------------------------------------------------------------------------

// strictCompare implements RFC-002 strict comparison semantics:
// - null/missing operand → null
// - same type: typed comparison (string is lexical CS, numeric, bool ==/!= only, timestamp, duration)
// - int vs float: cross-promote (both are "number" — NOT a type error)
// - incompatible types → null + warning counter increment
//
// Returns (result int, isNull bool). result is -1/0/1; isNull means the
// comparison could not be performed (null operand or incompatible types).
func strictCompare(a, b event.Value, warnings *WarningCounters) (int, bool) {
	if a.IsNull() || b.IsNull() {
		return 0, true
	}

	at, bt := a.Type(), b.Type()

	// Same-type fast paths
	if at == bt {
		switch at {
		case event.FieldTypeString:
			return strings.Compare(a.AsString(), b.AsString()), false
		case event.FieldTypeInt:
			ai, bi := a.AsInt(), b.AsInt()
			if ai < bi {
				return -1, false
			} else if ai > bi {
				return 1, false
			}
			return 0, false
		case event.FieldTypeFloat:
			af, bf := a.AsFloat(), b.AsFloat()
			if af < bf {
				return -1, false
			} else if af > bf {
				return 1, false
			}
			return 0, false
		case event.FieldTypeBool:
			// Bool supports ==/!= only. For < > we treat as incompatible.
			// But the caller (eq/neq) will use this correctly.
			ab, bb := boolToInt(a.AsBool()), boolToInt(b.AsBool())
			return ab - bb, false
		case event.FieldTypeTimestamp:
			at, bt := a.AsTimestamp(), b.AsTimestamp()
			if at.Before(bt) {
				return -1, false
			} else if at.After(bt) {
				return 1, false
			}
			return 0, false
		case event.FieldTypeDuration:
			ad, bd := a.AsDuration(), b.AsDuration()
			if ad < bd {
				return -1, false
			} else if ad > bd {
				return 1, false
			}
			return 0, false
		case event.FieldTypeArray:
			return compareArrays(a.AsArray(), b.AsArray()), false
		case event.FieldTypeObject:
			if objectsEqual(a.AsObject(), b.AsObject()) {
				return 0, false
			}
			return strings.Compare(valueToString(a), valueToString(b)), false
		}
	}

	// Cross-type numeric: int vs float (RFC-002: both are "number", cross-promote IS allowed)
	if (at == event.FieldTypeInt && bt == event.FieldTypeFloat) ||
		(at == event.FieldTypeFloat && bt == event.FieldTypeInt) {
		af := toFloat64ForCompare(a)
		bf := toFloat64ForCompare(b)
		if af < bf {
			return -1, false
		} else if af > bf {
			return 1, false
		}
		return 0, false
	}

	// Incompatible types → null + warning
	if warnings != nil {
		warnings.Increment(warnIncompatibleTypes)
	}
	return 0, true
}

func toFloat64ForCompare(v event.Value) float64 {
	switch v.Type() {
	case event.FieldTypeInt:
		return float64(v.AsInt())
	case event.FieldTypeFloat:
		return v.AsFloat()
	default:
		return 0
	}
}

// strictEq implements RFC-002 strict equality. Returns (bool, isNull).
func strictEq(a, b event.Value, warnings *WarningCounters) (bool, bool) {
	cmp, isNull := strictCompare(a, b, warnings)
	if isNull {
		return false, true
	}
	return cmp == 0, false
}

// ---------------------------------------------------------------------------
// Strict arithmetic (RFC-002 §5.4)
// ---------------------------------------------------------------------------

// addStrict implements RFC-002 §5.4 addition:
// - null propagation
// - string + string = concat ONLY
// - string + non-string = null + warning (sema catches known cases; runtime is lenient-null)
// - int + int = int
// - int + float or float + float = float
// - timestamp/duration algebra
func addStrict(a, b event.Value, warnings *WarningCounters) event.Value {
	if a.IsNull() || b.IsNull() {
		return event.NullValue()
	}
	at, bt := a.Type(), b.Type()

	// String + string = concat
	if at == event.FieldTypeString && bt == event.FieldTypeString {
		return event.StringValue(a.AsString() + b.AsString())
	}
	// String + non-string or non-string + string = null + warning
	if at == event.FieldTypeString || bt == event.FieldTypeString {
		if warnings != nil {
			warnings.Increment(warnStringArithmetic)
		}
		return event.NullValue()
	}
	// int + int
	if at == event.FieldTypeInt && bt == event.FieldTypeInt {
		return event.IntValue(a.AsInt() + b.AsInt())
	}
	// float/float, int/float, float/int → float
	if isNumericType(at) && isNumericType(bt) {
		return event.FloatValue(toFloat64ForCompare(a) + toFloat64ForCompare(b))
	}
	// Timestamp/duration algebra
	if at == event.FieldTypeTimestamp && bt == event.FieldTypeDuration {
		return event.TimestampValue(a.AsTimestamp().Add(b.AsDuration()))
	}
	if at == event.FieldTypeDuration && bt == event.FieldTypeTimestamp {
		return event.TimestampValue(b.AsTimestamp().Add(a.AsDuration()))
	}
	if at == event.FieldTypeDuration && bt == event.FieldTypeDuration {
		return event.DurationValue(a.AsDuration() + b.AsDuration())
	}
	// Incompatible
	if warnings != nil {
		warnings.Increment(warnIncompatibleTypes)
	}
	return event.NullValue()
}

func subStrict(a, b event.Value, warnings *WarningCounters) event.Value {
	if a.IsNull() || b.IsNull() {
		return event.NullValue()
	}
	at, bt := a.Type(), b.Type()

	if at == event.FieldTypeInt && bt == event.FieldTypeInt {
		return event.IntValue(a.AsInt() - b.AsInt())
	}
	if isNumericType(at) && isNumericType(bt) {
		return event.FloatValue(toFloat64ForCompare(a) - toFloat64ForCompare(b))
	}
	// ts - ts → dur
	if at == event.FieldTypeTimestamp && bt == event.FieldTypeTimestamp {
		return event.DurationValue(a.AsTimestamp().Sub(b.AsTimestamp()))
	}
	// ts - dur → ts
	if at == event.FieldTypeTimestamp && bt == event.FieldTypeDuration {
		return event.TimestampValue(a.AsTimestamp().Add(-b.AsDuration()))
	}
	// dur - dur → dur
	if at == event.FieldTypeDuration && bt == event.FieldTypeDuration {
		return event.DurationValue(a.AsDuration() - b.AsDuration())
	}
	if warnings != nil {
		warnings.Increment(warnIncompatibleTypes)
	}
	return event.NullValue()
}

func mulStrict(a, b event.Value, warnings *WarningCounters) event.Value {
	if a.IsNull() || b.IsNull() {
		return event.NullValue()
	}
	at, bt := a.Type(), b.Type()

	if at == event.FieldTypeInt && bt == event.FieldTypeInt {
		return event.IntValue(a.AsInt() * b.AsInt())
	}
	if isNumericType(at) && isNumericType(bt) {
		return event.FloatValue(toFloat64ForCompare(a) * toFloat64ForCompare(b))
	}
	// dur * number → dur (commutative)
	if at == event.FieldTypeDuration && isNumericType(bt) {
		return event.DurationValue(time.Duration(float64(a.AsDuration()) * toFloat64ForCompare(b)))
	}
	if isNumericType(at) && bt == event.FieldTypeDuration {
		return event.DurationValue(time.Duration(toFloat64ForCompare(a) * float64(b.AsDuration())))
	}
	if warnings != nil {
		warnings.Increment(warnIncompatibleTypes)
	}
	return event.NullValue()
}

func divStrict(a, b event.Value, warnings *WarningCounters) event.Value {
	if a.IsNull() || b.IsNull() {
		return event.NullValue()
	}
	at, bt := a.Type(), b.Type()

	// int / int → TRUNCATING int (5/2 == 2 per §5.4)
	if at == event.FieldTypeInt && bt == event.FieldTypeInt {
		bv := b.AsInt()
		if bv == 0 {
			return event.NullValue()
		}
		return event.IntValue(a.AsInt() / bv)
	}
	// numeric / numeric → float
	if isNumericType(at) && isNumericType(bt) {
		bf := toFloat64ForCompare(b)
		if bf == 0 {
			return event.NullValue()
		}
		return event.FloatValue(toFloat64ForCompare(a) / bf)
	}
	// dur / dur → float
	if at == event.FieldTypeDuration && bt == event.FieldTypeDuration {
		bd := b.AsDuration()
		if bd == 0 {
			return event.NullValue()
		}
		return event.FloatValue(float64(a.AsDuration()) / float64(bd))
	}
	// dur / number → dur
	if at == event.FieldTypeDuration && isNumericType(bt) {
		bf := toFloat64ForCompare(b)
		if bf == 0 {
			return event.NullValue()
		}
		return event.DurationValue(time.Duration(float64(a.AsDuration()) / bf))
	}
	if warnings != nil {
		warnings.Increment(warnIncompatibleTypes)
	}
	return event.NullValue()
}

func modStrict(a, b event.Value, warnings *WarningCounters) event.Value {
	if a.IsNull() || b.IsNull() {
		return event.NullValue()
	}
	// RFC-002 §5.4: % is int-only (other types → null + warning)
	if a.Type() == event.FieldTypeInt && b.Type() == event.FieldTypeInt {
		bv := b.AsInt()
		if bv == 0 {
			return event.NullValue()
		}
		return event.IntValue(a.AsInt() % bv)
	}
	// dur % dur → dur (also valid per §5.4)
	if a.Type() == event.FieldTypeDuration && b.Type() == event.FieldTypeDuration {
		bd := b.AsDuration()
		if bd == 0 {
			return event.NullValue()
		}
		return event.DurationValue(a.AsDuration() % bd)
	}
	if warnings != nil {
		warnings.Increment(warnModOnNonInt)
	}
	return event.NullValue()
}

func negStrict(a event.Value, warnings *WarningCounters) event.Value {
	if a.IsNull() {
		return event.NullValue()
	}
	switch a.Type() {
	case event.FieldTypeInt:
		return event.IntValue(-a.AsInt())
	case event.FieldTypeFloat:
		return event.FloatValue(-a.AsFloat())
	case event.FieldTypeDuration:
		return event.DurationValue(-a.AsDuration())
	default:
		if warnings != nil {
			warnings.Increment(warnIncompatibleTypes)
		}
		return event.NullValue()
	}
}

func isNumericType(t event.FieldType) bool {
	return t == event.FieldTypeInt || t == event.FieldTypeFloat
}

// ---------------------------------------------------------------------------
// OpLoadPath: flat-column first, then object walk, no _raw fallback (D25)
// ---------------------------------------------------------------------------

func execLoadPath(fields map[string]event.Value, path string) event.Value {
	// 1. Flat column lookup (the full dotted path as a literal column name)
	if val, ok := fields[path]; ok {
		return val
	}
	// 2. Object walk: split on '.', load root, walk members
	dot := strings.IndexByte(path, '.')
	if dot <= 0 {
		// No dot or leading dot — field simply missing
		return event.NullValue()
	}
	root := path[:dot]
	rest := path[dot+1:]
	rootVal, ok := fields[root]
	if !ok || rootVal.IsNull() {
		return event.NullValue()
	}
	// Walk the chain
	val := rootVal
	for _, part := range strings.Split(rest, ".") {
		if val.IsNull() {
			return event.NullValue()
		}
		if val.Type() != event.FieldTypeObject {
			return event.NullValue()
		}
		m := val.AsObject()
		next, found := m[part]
		if !found {
			return event.NullValue()
		}
		val = next
	}
	return val
}

// ---------------------------------------------------------------------------
// OpHasToken: case-insensitive whole-token match per §6.1/6.2
// ---------------------------------------------------------------------------

// hasToken checks if the field string contains the search term as a whole token,
// case-insensitively. A token is a run of ASCII alphanumerics and Unicode
// letters/digits; everything else delimits. Multi-word terms are treated as
// conjunction: all tokens must be present.
func hasToken(field, term string) bool {
	if field == "" || term == "" {
		return false
	}
	fieldLower := strings.ToLower(field)
	termLower := strings.ToLower(term)

	// Tokenize the term to get the tokens we need to find
	termTokens := tokenize(termLower)
	if len(termTokens) == 0 {
		return false
	}

	// Tokenize the field
	fieldTokens := tokenize(fieldLower)
	fieldSet := make(map[string]struct{}, len(fieldTokens))
	for _, t := range fieldTokens {
		fieldSet[t] = struct{}{}
	}

	// All term tokens must be present
	for _, tt := range termTokens {
		if _, ok := fieldSet[tt]; !ok {
			return false
		}
	}
	return true
}

// tokenize splits a string into tokens per the tokenizer contract (§6.1):
// runs of ASCII alphanumerics and Unicode letters/digits.
func tokenize(s string) []string {
	var tokens []string
	start := -1
	for i, r := range s {
		isToken := unicode.IsLetter(r) || unicode.IsDigit(r)
		if isToken {
			if start < 0 {
				start = i
			}
		} else {
			if start >= 0 {
				tokens = append(tokens, s[start:i])
				start = -1
			}
		}
	}
	if start >= 0 {
		tokens = append(tokens, s[start:])
	}
	return tokens
}

// ---------------------------------------------------------------------------
// OpSubstr0Based: 0-based start per RFC-002
// ---------------------------------------------------------------------------

func substr0Based(str, start, length event.Value) event.Value {
	if str.IsNull() || start.IsNull() {
		return event.NullValue()
	}
	s := valueToString(str)
	runes := []rune(s)
	n := int64(len(runes))

	si, ok := valueToInt64(start)
	if !ok {
		return event.NullValue()
	}
	// Negative counts from end
	if si < 0 {
		si += n
	}
	if si < 0 {
		si = 0
	}
	if si >= n {
		return event.StringValue("")
	}

	var li int64
	if length.IsNull() {
		li = n - si // to end
	} else {
		l, lok := valueToInt64(length)
		if !lok {
			return event.NullValue()
		}
		li = l
	}
	if li < 0 {
		return event.StringValue("")
	}
	end := si + li
	if end > n {
		end = n
	}
	return event.StringValue(string(runes[si:end]))
}

// ---------------------------------------------------------------------------
// OpExtract / OpExtractAll
// ---------------------------------------------------------------------------

func extractFirst(re *regexp.Regexp, s string) event.Value {
	if re.NumSubexp() == 0 {
		// No capture groups, return the full match
		m := re.FindString(s)
		if m == "" {
			return event.NullValue()
		}
		return event.StringValue(m)
	}
	matches := re.FindStringSubmatch(s)
	if matches == nil {
		return event.NullValue()
	}
	if len(matches) > 1 {
		return event.StringValue(matches[1])
	}
	return event.NullValue()
}

func extractAllMatches(re *regexp.Regexp, s string) event.Value {
	if re.NumSubexp() == 0 {
		matches := re.FindAllString(s, -1)
		if matches == nil {
			return event.NullValue()
		}
		elems := make([]event.Value, len(matches))
		for i, m := range matches {
			elems[i] = event.StringValue(m)
		}
		return event.ArrayValue(elems)
	}
	all := re.FindAllStringSubmatch(s, -1)
	if all == nil {
		return event.NullValue()
	}
	elems := make([]event.Value, len(all))
	for i, m := range all {
		if len(m) > 1 {
			elems[i] = event.StringValue(m[1])
		} else {
			elems[i] = event.NullValue()
		}
	}
	return event.ArrayValue(elems)
}

// ---------------------------------------------------------------------------
// OpInStrict: strict equality in-list check with null awareness
// ---------------------------------------------------------------------------

// inStrict checks if val is in the list using strict equality.
// If val is null → result is null.
// If any item comparison is null (incompatible types), accumulate null.
// If any item matches → true.
// If no match and no null comparison → false.
// If no match and some null comparison → null (3VL).
func inStrict(val event.Value, items []event.Value, warnings *WarningCounters) event.Value {
	if val.IsNull() {
		return event.NullValue()
	}
	hasNullComparison := false
	for _, item := range items {
		eq, isNull := strictEq(val, item, warnings)
		if isNull {
			hasNullComparison = true
			continue
		}
		if eq {
			return event.BoolValue(true)
		}
	}
	if hasNullComparison {
		return event.NullValue()
	}
	return event.BoolValue(false)
}

// ---------------------------------------------------------------------------
// Strict-cast bang error
// ---------------------------------------------------------------------------

// ErrStrictCast is returned when a strict-cast function (e.g. int!()) fails.
// The error includes the function name and the value that could not be cast,
// providing row context for debugging. The VM returns this error, halting the
// query — this is the designed behavior for bang variants per RFC-002 §5.4.
type ErrStrictCast struct {
	Func  string // e.g. "int!"
	Value string // string representation of the offending value
	Type  string // type of the offending value
}

func (e *ErrStrictCast) Error() string {
	return fmt.Sprintf("strict cast %s failed: cannot convert %s (type %s)", e.Func, e.Value, e.Type)
}

// ---------------------------------------------------------------------------
// Lambda execution: any, all, filter, map
// ---------------------------------------------------------------------------

// execLambdaForElement runs a sub-program against the current row with a
// lambda parameter value pushed onto the lambda param stack. Pool lookups
// (constants, field names, regex, sub-programs) use the root program.
func (vm *VM) execLambdaForElement(rootProg *Program, subIdx int, elem event.Value, fields map[string]event.Value) (event.Value, error) {
	sub := rootProg.SubPrograms[subIdx]
	// Build an execution program that uses the sub's instructions but the
	// root's pools. This avoids copying pools to every sub-program.
	execProg := &Program{
		Instructions:  sub.Instructions,
		Constants:     rootProg.Constants,
		FieldNames:    rootProg.FieldNames,
		RegexPatterns: rootProg.RegexPatterns,
		CIDRNets:      rootProg.CIDRNets,
		SubPrograms:   rootProg.SubPrograms,
	}
	// Push lambda param
	vm.lambdaParams = append(vm.lambdaParams, elem)
	// Save and reset SP for sub-program execution
	savedSP := vm.sp
	vm.sp = 0
	result, err := vm.ExecuteWithContext(execProg, fields, vm.predicateCtx)
	vm.sp = savedSP
	// Pop lambda param
	vm.lambdaParams = vm.lambdaParams[:len(vm.lambdaParams)-1]
	return result, err
}

// execArrayAny implements any(arr, lambda) with 3VL semantics:
// - Empty array: false
// - If any element pred is true: true
// - If no true and any null: null
// - Otherwise: false
func (vm *VM) execArrayAny(prog *Program, subIdx int, arrVal event.Value, fields map[string]event.Value) (event.Value, error) {
	if arrVal.IsNull() {
		return event.NullValue(), nil
	}
	if arrVal.Type() != event.FieldTypeArray {
		return event.NullValue(), nil
	}
	arr := arrVal.AsArray()
	if len(arr) == 0 {
		return event.BoolValue(false), nil
	}
	hasNull := false
	for _, elem := range arr {
		result, err := vm.execLambdaForElement(prog, subIdx, elem, fields)
		if err != nil {
			return event.NullValue(), err
		}
		if result.IsNull() {
			hasNull = true
			continue
		}
		if result.Type() == event.FieldTypeBool && result.AsBool() {
			return event.BoolValue(true), nil
		}
	}
	if hasNull {
		return event.NullValue(), nil
	}
	return event.BoolValue(false), nil
}

// execArrayAll implements all(arr, lambda) with 3VL semantics:
// - Empty array: true
// - If any element pred is false: false
// - If no false and any null: null
// - Otherwise: true
func (vm *VM) execArrayAll(prog *Program, subIdx int, arrVal event.Value, fields map[string]event.Value) (event.Value, error) {
	if arrVal.IsNull() {
		return event.NullValue(), nil
	}
	if arrVal.Type() != event.FieldTypeArray {
		return event.NullValue(), nil
	}
	arr := arrVal.AsArray()
	if len(arr) == 0 {
		return event.BoolValue(true), nil
	}
	hasNull := false
	for _, elem := range arr {
		result, err := vm.execLambdaForElement(prog, subIdx, elem, fields)
		if err != nil {
			return event.NullValue(), err
		}
		if result.IsNull() {
			hasNull = true
			continue
		}
		if result.Type() == event.FieldTypeBool && !result.AsBool() {
			return event.BoolValue(false), nil
		}
	}
	if hasNull {
		return event.NullValue(), nil
	}
	return event.BoolValue(true), nil
}

// execArrayFilter implements filter(arr, lambda):
// - Keeps elements where pred is true (null/false dropped)
// - Empty array: []
func (vm *VM) execArrayFilter(prog *Program, subIdx int, arrVal event.Value, fields map[string]event.Value) (event.Value, error) {
	if arrVal.IsNull() {
		return event.NullValue(), nil
	}
	if arrVal.Type() != event.FieldTypeArray {
		return event.NullValue(), nil
	}
	arr := arrVal.AsArray()
	if len(arr) == 0 {
		return event.ArrayValue(nil), nil
	}
	result := make([]event.Value, 0, len(arr))
	for _, elem := range arr {
		pred, err := vm.execLambdaForElement(prog, subIdx, elem, fields)
		if err != nil {
			return event.NullValue(), err
		}
		if pred.Type() == event.FieldTypeBool && pred.AsBool() {
			result = append(result, elem)
		}
		// null/false: dropped
	}
	return event.ArrayValue(result), nil
}

// execArrayMap implements map(arr, lambda):
// - Applies lambda to each element, collecting results
// - null results kept as null elements
// - Empty array: []
func (vm *VM) execArrayMap(prog *Program, subIdx int, arrVal event.Value, fields map[string]event.Value) (event.Value, error) {
	if arrVal.IsNull() {
		return event.NullValue(), nil
	}
	if arrVal.Type() != event.FieldTypeArray {
		return event.NullValue(), nil
	}
	arr := arrVal.AsArray()
	if len(arr) == 0 {
		return event.ArrayValue(nil), nil
	}
	result := make([]event.Value, len(arr))
	for i, elem := range arr {
		mapped, err := vm.execLambdaForElement(prog, subIdx, elem, fields)
		if err != nil {
			return event.NullValue(), err
		}
		result[i] = mapped
	}
	return event.ArrayValue(result), nil
}

// ---------------------------------------------------------------------------
// Slice, ArrayConcat, ArrayDistinct, ArraySort, Flatten
// ---------------------------------------------------------------------------

// execSlice implements slice(arr, start[, end]).
// Stack: [..., arr, start, end] or [..., arr, start] (end=null means to end).
// 0-based indexing, negative from end, clamped.
func (vm *VM) execSlice() {
	endVal := vm.stack[vm.sp-1]
	startVal := vm.stack[vm.sp-2]
	arrVal := vm.stack[vm.sp-3]
	vm.sp -= 2

	if arrVal.IsNull() || startVal.IsNull() {
		vm.stack[vm.sp-1] = event.NullValue()
		return
	}
	if arrVal.Type() != event.FieldTypeArray {
		vm.stack[vm.sp-1] = event.NullValue()
		return
	}

	arr := arrVal.AsArray()
	n := int64(len(arr))

	si, ok := valueToInt64(startVal)
	if !ok {
		vm.stack[vm.sp-1] = event.NullValue()
		return
	}
	if si < 0 {
		si += n
	}
	if si < 0 {
		si = 0
	}
	if si > n {
		si = n
	}

	var ei int64
	if endVal.IsNull() {
		ei = n
	} else {
		e, eok := valueToInt64(endVal)
		if !eok {
			vm.stack[vm.sp-1] = event.NullValue()
			return
		}
		ei = e
		if ei < 0 {
			ei += n
		}
		if ei < 0 {
			ei = 0
		}
		if ei > n {
			ei = n
		}
	}

	if si >= ei {
		vm.stack[vm.sp-1] = event.ArrayValue(nil)
		return
	}

	result := make([]event.Value, ei-si)
	copy(result, arr[si:ei])
	vm.stack[vm.sp-1] = event.ArrayValue(result)
}

// execArrayConcat implements array_concat(variadic).
func (vm *VM) execArrayConcat(count int) {
	if count == 0 {
		vm.stack[vm.sp] = event.ArrayValue(nil)
		vm.sp++
		return
	}
	var result []event.Value
	for i := vm.sp - count; i < vm.sp; i++ {
		v := vm.stack[i]
		if v.IsNull() {
			continue
		}
		if v.Type() == event.FieldTypeArray {
			result = append(result, v.AsArray()...)
		}
	}
	vm.sp -= count
	if result == nil {
		result = []event.Value{}
	}
	vm.stack[vm.sp] = event.ArrayValue(result)
	vm.sp++
}

// execArrayDistinct returns order-preserving first-wins deduplicated array.
func execArrayDistinct(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeArray {
		return event.NullValue()
	}
	arr := v.AsArray()
	if len(arr) == 0 {
		return event.ArrayValue(nil)
	}
	// Order-preserving dedup using deep equality (valuesEqual).
	// O(n^2) in worst case but arrays in log analytics are typically small.
	result := make([]event.Value, 0, len(arr))
	for _, elem := range arr {
		found := false
		for _, existing := range result {
			if valuesEqual(elem, existing) {
				found = true
				break
			}
		}
		if !found {
			result = append(result, elem)
		}
	}
	return event.ArrayValue(result)
}

// execArraySort sorts using CompareValues order; nulls last.
func execArraySort(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeArray {
		return event.NullValue()
	}
	arr := v.AsArray()
	if len(arr) == 0 {
		return event.ArrayValue(nil)
	}
	// Copy to avoid mutating the original
	sorted := make([]event.Value, len(arr))
	copy(sorted, arr)
	sort.SliceStable(sorted, func(i, j int) bool {
		ai, aj := sorted[i], sorted[j]
		// Nulls last
		if ai.IsNull() && aj.IsNull() {
			return false
		}
		if ai.IsNull() {
			return false
		}
		if aj.IsNull() {
			return true
		}
		return CompareValues(ai, aj) < 0
	})
	return event.ArrayValue(sorted)
}

// execFlatten flattens one level of nested arrays.
func execFlatten(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeArray {
		return event.NullValue()
	}
	arr := v.AsArray()
	if len(arr) == 0 {
		return event.ArrayValue(nil)
	}
	var result []event.Value
	for _, elem := range arr {
		if elem.Type() == event.FieldTypeArray {
			result = append(result, elem.AsArray()...)
		} else {
			result = append(result, elem)
		}
	}
	return event.ArrayValue(result)
}

// ---------------------------------------------------------------------------
// Object functions: keys, values, merge, has_key
// ---------------------------------------------------------------------------

// execKeys returns sorted array of key strings.
func execKeys(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeObject {
		return event.NullValue()
	}
	obj := v.AsObject()
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]event.Value, len(keys))
	for i, k := range keys {
		result[i] = event.StringValue(k)
	}
	return event.ArrayValue(result)
}

// execValues returns array of values in key-sorted order (deterministic).
func execValues(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeObject {
		return event.NullValue()
	}
	obj := v.AsObject()
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	result := make([]event.Value, len(keys))
	for i, k := range keys {
		result[i] = obj[k]
	}
	return event.ArrayValue(result)
}

// execMerge merges two objects (right wins on key collision).
func execMerge(a, b event.Value) event.Value {
	if a.IsNull() || b.IsNull() {
		return event.NullValue()
	}
	if a.Type() != event.FieldTypeObject || b.Type() != event.FieldTypeObject {
		return event.NullValue()
	}
	aObj := a.AsObject()
	bObj := b.AsObject()
	result := make(map[string]event.Value, len(aObj)+len(bObj))
	for k, v := range aObj {
		result[k] = v
	}
	for k, v := range bObj {
		result[k] = v // right wins
	}
	return event.ObjectValue(result)
}

// execHasKey checks if an object has a key.
func execHasKey(obj, key event.Value) event.Value {
	if obj.IsNull() || key.IsNull() {
		return event.NullValue()
	}
	if obj.Type() != event.FieldTypeObject {
		return event.NullValue()
	}
	k := valueToString(key)
	_, ok := obj.AsObject()[k]
	return event.BoolValue(ok)
}

// ---------------------------------------------------------------------------
// url_parse, ip_parse, from_json (native)
// ---------------------------------------------------------------------------

// execURLParse parses a URL into an object.
func execURLParse(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeString {
		return event.NullValue()
	}
	s := v.AsString()
	u, err := url.Parse(s)
	if err != nil {
		return event.NullValue()
	}
	result := map[string]event.Value{
		"scheme":   event.StringValue(u.Scheme),
		"host":     event.StringValue(u.Hostname()),
		"path":     event.StringValue(u.Path),
		"fragment": event.StringValue(u.Fragment),
	}
	// Port: int if numeric, else null
	if portStr := u.Port(); portStr != "" {
		if port, pErr := strconv.ParseInt(portStr, 10, 64); pErr == nil {
			result["port"] = event.IntValue(port)
		} else {
			result["port"] = event.NullValue()
		}
	} else {
		result["port"] = event.NullValue()
	}
	// Query: object of string key-value pairs
	queryObj := make(map[string]event.Value, len(u.Query()))
	for k, vals := range u.Query() {
		if len(vals) > 0 {
			queryObj[k] = event.StringValue(vals[0])
		}
	}
	result["query"] = event.ObjectValue(queryObj)
	return event.ObjectValue(result)
}

// execIPParse parses an IP address into an object.
func execIPParse(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeString {
		return event.NullValue()
	}
	s := v.AsString()
	ip := net.ParseIP(s)
	if ip == nil {
		return event.NullValue()
	}
	var version int64
	if ip.To4() != nil {
		version = 4
	} else {
		version = 6
	}
	isPrivate := isPrivateIP(ip)
	isLoopback := ip.IsLoopback()
	return event.ObjectValue(map[string]event.Value{
		"version":  event.IntValue(version),
		"private":  event.BoolValue(isPrivate),
		"loopback": event.BoolValue(isLoopback),
	})
}

// isPrivateIP checks if an IP address is in a private range (RFC 1918 / RFC 4193).
func isPrivateIP(ip net.IP) bool {
	privateRanges := []struct {
		network *net.IPNet
	}{
		{network: mustParseCIDR("10.0.0.0/8")},
		{network: mustParseCIDR("172.16.0.0/12")},
		{network: mustParseCIDR("192.168.0.0/16")},
		{network: mustParseCIDR("fc00::/7")},
	}
	for _, r := range privateRanges {
		if r.network.Contains(ip) {
			return true
		}
	}
	return false
}

func mustParseCIDR(s string) *net.IPNet {
	_, n, err := net.ParseCIDR(s)
	if err != nil {
		panic(err)
	}
	return n
}

// execFromJSONNative parses a JSON string into native Values (recursive).
func execFromJSONNative(v event.Value) event.Value {
	if v.IsNull() {
		return event.NullValue()
	}
	if v.Type() != event.FieldTypeString {
		return event.NullValue()
	}
	s := v.AsString()
	return parseJSONToValue([]byte(s))
}

// parseJSONToValue recursively converts JSON bytes to event.Value.
func parseJSONToValue(data []byte) event.Value {
	data = bytes.TrimSpace(data)
	if len(data) == 0 {
		return event.NullValue()
	}
	switch data[0] {
	case '{':
		var obj map[string]json.RawMessage
		if err := json.Unmarshal(data, &obj); err != nil {
			return event.NullValue()
		}
		result := make(map[string]event.Value, len(obj))
		for k, raw := range obj {
			result[k] = parseJSONToValue(raw)
		}
		return event.ObjectValue(result)
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(data, &arr); err != nil {
			return event.NullValue()
		}
		result := make([]event.Value, len(arr))
		for i, raw := range arr {
			result[i] = parseJSONToValue(raw)
		}
		return event.ArrayValue(result)
	case '"':
		var s string
		if err := json.Unmarshal(data, &s); err != nil {
			return event.NullValue()
		}
		return event.StringValue(s)
	case 't', 'f':
		var b bool
		if err := json.Unmarshal(data, &b); err != nil {
			return event.NullValue()
		}
		return event.BoolValue(b)
	case 'n':
		return event.NullValue()
	default:
		// Number
		s := string(data)
		// Try int first
		if !strings.Contains(s, ".") && !strings.Contains(s, "e") && !strings.Contains(s, "E") {
			if i, err := strconv.ParseInt(s, 10, 64); err == nil {
				return event.IntValue(i)
			}
		}
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return event.FloatValue(f)
		}
		return event.NullValue()
	}
}

// Ensure unused imports are referenced.
var _ = binary.BigEndian
var _ = math.Pi
