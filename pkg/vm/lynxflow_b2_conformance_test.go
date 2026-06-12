package vm

// RFC-002 LynxFlow b2 conformance tests — lambda, array, and object functions.
//
// Coverage targets:
//   - Lambda compilation: any, all, filter, map with inline lambdas
//   - Lambda nesting: any(tags, t -> any(t.subtags, s -> s == "x"))
//   - 3VL any/all matrices (null element handling)
//   - Empty array edge cases for all higher-order functions
//   - filter/map on arrays of objects
//   - Array functions: slice, array_concat, array_distinct, array_sort, flatten
//   - Deep-equality distinct (objects in arrays)
//   - Sort determinism (nulls last)
//   - Object functions: keys, values, merge, has_key
//   - url_parse goldens
//   - ip_parse goldens
//   - from_json produces native nested values (assert via event.Value Type())
//   - x in array-field
//   - 60+ assertions

import (
	"testing"

	"github.com/lynxbase/lynxdb/pkg/event"
	lfast "github.com/lynxbase/lynxdb/pkg/lynxflow/ast"
)

// Helper: lambda constructor

func lambda(param string, body lfast.Expr) *lfast.Lambda {
	return &lfast.Lambda{Param: param, Body: body}
}

// assertArray is a helper to check an array value's length.
func assertArray(t *testing.T, v event.Value, expectedLen int, label string) []event.Value {
	t.Helper()
	if v.Type() != event.FieldTypeArray {
		t.Fatalf("%s: expected array, got %s (%s)", label, v.String(), v.Type())
	}
	arr := v.AsArray()
	if len(arr) != expectedLen {
		t.Fatalf("%s: expected %d elements, got %d", label, expectedLen, len(arr))
	}
	return arr
}

func assertObject(t *testing.T, v event.Value, label string) map[string]event.Value {
	t.Helper()
	if v.Type() != event.FieldTypeObject {
		t.Fatalf("%s: expected object, got %s (%s)", label, v.String(), v.Type())
	}
	return v.AsObject()
}

// Lambda: any()

func TestB2_Any_Basic(t *testing.T) {
	// any([1, 2, 3], x -> x > 2) => true
	expr := call("any",
		array(litInt(1), litInt(2), litInt(3)),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(2))),
	)
	result, _ := runLF(t, expr, nil)
	assertBool(t, result, true, "any([1,2,3], x->x>2)")
}

func TestB2_Any_NoMatch(t *testing.T) {
	// any([1, 2, 3], x -> x > 5) => false
	expr := call("any",
		array(litInt(1), litInt(2), litInt(3)),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(5))),
	)
	result, _ := runLF(t, expr, nil)
	assertBool(t, result, false, "any([1,2,3], x->x>5)")
}

func TestB2_Any_EmptyArray(t *testing.T) {
	// any([], x -> x > 0) => false
	expr := call("any",
		array(),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(0))),
	)
	result, _ := runLF(t, expr, nil)
	assertBool(t, result, false, "any([], x->x>0)")
}

func TestB2_Any_NullArray(t *testing.T) {
	// any(null, x -> x > 0) => null
	expr := call("any",
		litNull(),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(0))),
	)
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "any(null, x->x>0)")
}

// 3VL any/all matrices

func TestB2_Any_3VL_Matrix(t *testing.T) {
	// any with null elements:
	// [true, null] => true (any true found)
	// [false, null] => null (no true, but null present)
	// [null, null] => null
	// [false, false] => false

	tests := []struct {
		name string
		arr  *lfast.Array
		want interface{} // bool or nil for null
	}{
		{"[true, null]", array(litBool(true), litNull()), true},
		{"[false, null]", array(litBool(false), litNull()), nil},
		{"[null, null]", array(litNull(), litNull()), nil},
		{"[false, false]", array(litBool(false), litBool(false)), false},
		{"[true, false]", array(litBool(true), litBool(false)), true},
		{"[null, true]", array(litNull(), litBool(true)), true},
		{"[null, false]", array(litNull(), litBool(false)), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Identity lambda: any(arr, x -> x)
			expr := call("any", tt.arr, lambda("x", ident("x")))
			result, _ := runLF(t, expr, nil)
			if tt.want == nil {
				assertNull(t, result, tt.name)
			} else {
				assertBool(t, result, tt.want.(bool), tt.name)
			}
		})
	}
}

func TestB2_All_3VL_Matrix(t *testing.T) {
	// all with null elements:
	// [true, null] => null (no false, but null present)
	// [false, null] => false (false found)
	// [null, null] => null
	// [true, true] => true

	tests := []struct {
		name string
		arr  *lfast.Array
		want interface{}
	}{
		{"[true, null]", array(litBool(true), litNull()), nil},
		{"[false, null]", array(litBool(false), litNull()), false},
		{"[null, null]", array(litNull(), litNull()), nil},
		{"[true, true]", array(litBool(true), litBool(true)), true},
		{"[false, true]", array(litBool(false), litBool(true)), false},
		{"[null, true]", array(litNull(), litBool(true)), nil},
		{"[null, false]", array(litNull(), litBool(false)), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Identity lambda: all(arr, x -> x)
			expr := call("all", tt.arr, lambda("x", ident("x")))
			result, _ := runLF(t, expr, nil)
			if tt.want == nil {
				assertNull(t, result, tt.name)
			} else {
				assertBool(t, result, tt.want.(bool), tt.name)
			}
		})
	}
}

func TestB2_All_EmptyArray(t *testing.T) {
	// all([], x -> x) => true (vacuous truth)
	expr := call("all",
		array(),
		lambda("x", ident("x")),
	)
	result, _ := runLF(t, expr, nil)
	assertBool(t, result, true, "all([], x->x)")
}

// Lambda: filter()

func TestB2_Filter_Basic(t *testing.T) {
	// filter([1, 2, 3, 4, 5], x -> x > 3) => [4, 5]
	expr := call("filter",
		array(litInt(1), litInt(2), litInt(3), litInt(4), litInt(5)),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(3))),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "filter basic")
	assertInt(t, arr[0], 4, "filter[0]")
	assertInt(t, arr[1], 5, "filter[1]")
}

func TestB2_Filter_EmptyResult(t *testing.T) {
	// filter([1, 2], x -> x > 10) => []
	expr := call("filter",
		array(litInt(1), litInt(2)),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(10))),
	)
	result, _ := runLF(t, expr, nil)
	assertArray(t, result, 0, "filter empty result")
}

func TestB2_Filter_NullDropped(t *testing.T) {
	// filter([1, null, 3], x -> x > 0) => [1, 3] (null pred drops)
	// Note: comparing null > 0 yields null (not true), so null elements drop.
	expr := call("filter",
		array(litInt(1), litNull(), litInt(3)),
		lambda("x", binOp(lfast.OpGt, ident("x"), litInt(0))),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "filter null dropped")
	assertInt(t, arr[0], 1, "filter[0]")
	assertInt(t, arr[1], 3, "filter[1]")
}

func TestB2_Filter_EmptyArray(t *testing.T) {
	expr := call("filter", array(), lambda("x", litBool(true)))
	result, _ := runLF(t, expr, nil)
	assertArray(t, result, 0, "filter empty array")
}

func TestB2_Filter_ObjectsInArray(t *testing.T) {
	// filter([{level: "error"}, {level: "info"}], e -> e.level == "error")
	fields := map[string]event.Value{}
	expr := call("filter",
		array(
			object(objEntry("level", litStr("error"))),
			object(objEntry("level", litStr("info"))),
		),
		lambda("e", binOp(lfast.OpEq, member(ident("e"), "level"), litStr("error"))),
	)
	result, _ := runLF(t, expr, fields)
	arr := assertArray(t, result, 1, "filter objects")
	obj := assertObject(t, arr[0], "filter objects[0]")
	assertString(t, obj["level"], "error", "filter objects[0].level")
}

// Lambda: map()

func TestB2_Map_Basic(t *testing.T) {
	// map([1, 2, 3], x -> x * 2) => [2, 4, 6]
	expr := call("map",
		array(litInt(1), litInt(2), litInt(3)),
		lambda("x", binOp(lfast.OpMul, ident("x"), litInt(2))),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "map basic")
	assertInt(t, arr[0], 2, "map[0]")
	assertInt(t, arr[1], 4, "map[1]")
	assertInt(t, arr[2], 6, "map[2]")
}

func TestB2_Map_NullPreserved(t *testing.T) {
	// map([1, null, 3], x -> x * 2) => [2, null, 6]
	// null * 2 = null
	expr := call("map",
		array(litInt(1), litNull(), litInt(3)),
		lambda("x", binOp(lfast.OpMul, ident("x"), litInt(2))),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "map null preserved")
	assertInt(t, arr[0], 2, "map[0]")
	assertNull(t, arr[1], "map[1] null")
	assertInt(t, arr[2], 6, "map[2]")
}

func TestB2_Map_EmptyArray(t *testing.T) {
	expr := call("map", array(), lambda("x", ident("x")))
	result, _ := runLF(t, expr, nil)
	assertArray(t, result, 0, "map empty array")
}

func TestB2_Map_ExtractFromObjects(t *testing.T) {
	// map([{n: 1}, {n: 2}], x -> x.n) => [1, 2]
	expr := call("map",
		array(
			object(objEntry("n", litInt(1))),
			object(objEntry("n", litInt(2))),
		),
		lambda("x", member(ident("x"), "n")),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "map extract objects")
	assertInt(t, arr[0], 1, "map objects[0]")
	assertInt(t, arr[1], 2, "map objects[1]")
}

// Lambda: nested lambdas

func TestB2_Nested_Lambda_Any_Any(t *testing.T) {
	// any(tags, t -> any(t.subtags, s -> s == "x"))
	// tags = [{subtags: ["a", "x"]}, {subtags: ["b", "c"]}]
	fields := map[string]event.Value{
		"tags": event.ArrayValue([]event.Value{
			event.ObjectValue(map[string]event.Value{
				"subtags": event.ArrayValue([]event.Value{
					event.StringValue("a"),
					event.StringValue("x"),
				}),
			}),
			event.ObjectValue(map[string]event.Value{
				"subtags": event.ArrayValue([]event.Value{
					event.StringValue("b"),
					event.StringValue("c"),
				}),
			}),
		}),
	}
	// any(tags, t -> any(t.subtags, s -> s == "x"))
	expr := call("any",
		ident("tags"),
		lambda("t",
			call("any",
				member(ident("t"), "subtags"),
				lambda("s", binOp(lfast.OpEq, ident("s"), litStr("x"))),
			),
		),
	)
	result, _ := runLF(t, expr, fields)
	assertBool(t, result, true, "nested any: found x")
}

func TestB2_Nested_Lambda_Any_Any_NotFound(t *testing.T) {
	// Same structure, but looking for "z" which doesn't exist
	fields := map[string]event.Value{
		"tags": event.ArrayValue([]event.Value{
			event.ObjectValue(map[string]event.Value{
				"subtags": event.ArrayValue([]event.Value{
					event.StringValue("a"),
					event.StringValue("b"),
				}),
			}),
		}),
	}
	expr := call("any",
		ident("tags"),
		lambda("t",
			call("any",
				member(ident("t"), "subtags"),
				lambda("s", binOp(lfast.OpEq, ident("s"), litStr("z"))),
			),
		),
	)
	result, _ := runLF(t, expr, fields)
	assertBool(t, result, false, "nested any: z not found")
}

func TestB2_Nested_Lambda_Param_Shadowing(t *testing.T) {
	// Ensure inner lambda shadows outer parameter with same name.
	// map([1, 2], x -> map([10, 20], x -> x + 100)) => [[110, 120], [110, 120]]
	// The inner x shadows the outer x; inner x refers to 10, 20 not 1, 2.
	expr := call("map",
		array(litInt(1), litInt(2)),
		lambda("x",
			call("map",
				array(litInt(10), litInt(20)),
				lambda("x", binOp(lfast.OpAdd, ident("x"), litInt(100))),
			),
		),
	)
	result, _ := runLF(t, expr, nil)
	outerArr := assertArray(t, result, 2, "shadowing outer")
	inner0 := assertArray(t, outerArr[0], 2, "shadowing[0]")
	assertInt(t, inner0[0], 110, "shadowing[0][0]")
	assertInt(t, inner0[1], 120, "shadowing[0][1]")
	inner1 := assertArray(t, outerArr[1], 2, "shadowing[1]")
	assertInt(t, inner1[0], 110, "shadowing[1][0]")
	assertInt(t, inner1[1], 120, "shadowing[1][1]")
}

// Array functions: slice

func TestB2_Slice(t *testing.T) {
	tests := []struct {
		name   string
		arr    *lfast.Array
		start  int64
		end    interface{} // int64 or nil
		expect []int64
	}{
		{"basic", array(litInt(1), litInt(2), litInt(3), litInt(4)), 1, int64(3), []int64{2, 3}},
		{"from start", array(litInt(1), litInt(2), litInt(3)), 0, int64(2), []int64{1, 2}},
		{"to end", array(litInt(1), litInt(2), litInt(3)), 1, nil, []int64{2, 3}},
		{"negative start", array(litInt(1), litInt(2), litInt(3)), -2, nil, []int64{2, 3}},
		{"negative end", array(litInt(1), litInt(2), litInt(3), litInt(4)), 0, int64(-1), []int64{1, 2, 3}},
		{"empty result", array(litInt(1), litInt(2)), 5, nil, nil},
		{"clamped start", array(litInt(1), litInt(2)), -10, int64(1), []int64{1}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var expr lfast.Expr
			if tt.end != nil {
				expr = call("slice", tt.arr, litInt(tt.start), litInt(tt.end.(int64)))
			} else {
				expr = call("slice", tt.arr, litInt(tt.start))
			}
			result, _ := runLF(t, expr, nil)
			if tt.expect == nil {
				assertArray(t, result, 0, tt.name)
			} else {
				arr := assertArray(t, result, len(tt.expect), tt.name)
				for i, want := range tt.expect {
					assertInt(t, arr[i], want, tt.name)
				}
			}
		})
	}
}

// Array functions: array_concat

func TestB2_ArrayConcat(t *testing.T) {
	// array_concat([1, 2], [3, 4]) => [1, 2, 3, 4]
	expr := call("array_concat",
		array(litInt(1), litInt(2)),
		array(litInt(3), litInt(4)),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 4, "array_concat")
	assertInt(t, arr[0], 1, "concat[0]")
	assertInt(t, arr[1], 2, "concat[1]")
	assertInt(t, arr[2], 3, "concat[2]")
	assertInt(t, arr[3], 4, "concat[3]")
}

func TestB2_ArrayConcat_Three(t *testing.T) {
	expr := call("array_concat",
		array(litInt(1)),
		array(litInt(2)),
		array(litInt(3)),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "concat three")
	assertInt(t, arr[0], 1, "concat3[0]")
	assertInt(t, arr[1], 2, "concat3[1]")
	assertInt(t, arr[2], 3, "concat3[2]")
}

// Array functions: array_distinct

func TestB2_ArrayDistinct_Ints(t *testing.T) {
	// array_distinct([1, 2, 2, 3, 1]) => [1, 2, 3]
	expr := call("array_distinct",
		array(litInt(1), litInt(2), litInt(2), litInt(3), litInt(1)),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "distinct ints")
	assertInt(t, arr[0], 1, "distinct[0]")
	assertInt(t, arr[1], 2, "distinct[1]")
	assertInt(t, arr[2], 3, "distinct[2]")
}

func TestB2_ArrayDistinct_DeepEquality(t *testing.T) {
	// array_distinct with objects: deep equality for dedup
	// [{a:1}, {a:2}, {a:1}] => [{a:1}, {a:2}]
	expr := call("array_distinct",
		array(
			object(objEntry("a", litInt(1))),
			object(objEntry("a", litInt(2))),
			object(objEntry("a", litInt(1))),
		),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "distinct deep")
	obj0 := assertObject(t, arr[0], "distinct deep[0]")
	assertInt(t, obj0["a"], 1, "distinct deep[0].a")
	obj1 := assertObject(t, arr[1], "distinct deep[1]")
	assertInt(t, obj1["a"], 2, "distinct deep[1].a")
}

func TestB2_ArrayDistinct_Empty(t *testing.T) {
	expr := call("array_distinct", array())
	result, _ := runLF(t, expr, nil)
	assertArray(t, result, 0, "distinct empty")
}

// Array functions: array_sort

func TestB2_ArraySort_Ints(t *testing.T) {
	// array_sort([3, 1, 2]) => [1, 2, 3]
	expr := call("array_sort",
		array(litInt(3), litInt(1), litInt(2)),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "sort ints")
	assertInt(t, arr[0], 1, "sort[0]")
	assertInt(t, arr[1], 2, "sort[1]")
	assertInt(t, arr[2], 3, "sort[2]")
}

func TestB2_ArraySort_NullsLast(t *testing.T) {
	// array_sort([3, null, 1]) => [1, 3, null]
	expr := call("array_sort",
		array(litInt(3), litNull(), litInt(1)),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "sort nulls last")
	assertInt(t, arr[0], 1, "sort[0]")
	assertInt(t, arr[1], 3, "sort[1]")
	assertNull(t, arr[2], "sort[2] null")
}

func TestB2_ArraySort_Determinism(t *testing.T) {
	// Sorting strings should be deterministic (lexical)
	expr := call("array_sort",
		array(litStr("banana"), litStr("apple"), litStr("cherry")),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 3, "sort strings")
	assertString(t, arr[0], "apple", "sort strings[0]")
	assertString(t, arr[1], "banana", "sort strings[1]")
	assertString(t, arr[2], "cherry", "sort strings[2]")
}

func TestB2_ArraySort_Empty(t *testing.T) {
	expr := call("array_sort", array())
	result, _ := runLF(t, expr, nil)
	assertArray(t, result, 0, "sort empty")
}

// Array functions: flatten

func TestB2_Flatten(t *testing.T) {
	// flatten([[1, 2], [3], 4]) => [1, 2, 3, 4]
	expr := call("flatten",
		array(
			array(litInt(1), litInt(2)),
			array(litInt(3)),
			litInt(4),
		),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 4, "flatten")
	assertInt(t, arr[0], 1, "flatten[0]")
	assertInt(t, arr[1], 2, "flatten[1]")
	assertInt(t, arr[2], 3, "flatten[2]")
	assertInt(t, arr[3], 4, "flatten[3]")
}

func TestB2_Flatten_OneLevel(t *testing.T) {
	// flatten([[1, [2, 3]]]) => [1, [2, 3]] (only one level)
	expr := call("flatten",
		array(
			array(litInt(1), array(litInt(2), litInt(3))),
		),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "flatten one level")
	assertInt(t, arr[0], 1, "flatten[0]")
	// arr[1] should be an array [2, 3]
	if arr[1].Type() != event.FieldTypeArray {
		t.Fatalf("expected arr[1] to be array, got %s", arr[1].Type())
	}
	inner := arr[1].AsArray()
	if len(inner) != 2 {
		t.Fatalf("expected 2 elements in inner, got %d", len(inner))
	}
}

func TestB2_Flatten_Empty(t *testing.T) {
	expr := call("flatten", array())
	result, _ := runLF(t, expr, nil)
	assertArray(t, result, 0, "flatten empty")
}

// Object functions: keys

func TestB2_Keys(t *testing.T) {
	// keys({b: 2, a: 1}) => ["a", "b"] (sorted)
	expr := call("keys",
		object(objEntry("b", litInt(2)), objEntry("a", litInt(1))),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "keys")
	assertString(t, arr[0], "a", "keys[0]")
	assertString(t, arr[1], "b", "keys[1]")
}

func TestB2_Keys_Null(t *testing.T) {
	expr := call("keys", litNull())
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "keys(null)")
}

// Object functions: values

func TestB2_Values(t *testing.T) {
	// values({b: 2, a: 1}) => [1, 2] (key-sorted order)
	expr := call("values",
		object(objEntry("b", litInt(2)), objEntry("a", litInt(1))),
	)
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 2, "values")
	assertInt(t, arr[0], 1, "values[0]")
	assertInt(t, arr[1], 2, "values[1]")
}

// Object functions: merge

func TestB2_Merge(t *testing.T) {
	// merge({a: 1, b: 2}, {b: 3, c: 4}) => {a: 1, b: 3, c: 4}
	expr := call("merge",
		object(objEntry("a", litInt(1)), objEntry("b", litInt(2))),
		object(objEntry("b", litInt(3)), objEntry("c", litInt(4))),
	)
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "merge")
	assertInt(t, obj["a"], 1, "merge.a")
	assertInt(t, obj["b"], 3, "merge.b (right wins)")
	assertInt(t, obj["c"], 4, "merge.c")
}

func TestB2_Merge_Null(t *testing.T) {
	expr := call("merge",
		object(objEntry("a", litInt(1))),
		litNull(),
	)
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "merge with null")
}

// Object functions: has_key

func TestB2_HasKey(t *testing.T) {
	expr := call("has_key",
		object(objEntry("name", litStr("test"))),
		litStr("name"),
	)
	result, _ := runLF(t, expr, nil)
	assertBool(t, result, true, "has_key found")

	expr2 := call("has_key",
		object(objEntry("name", litStr("test"))),
		litStr("missing"),
	)
	result2, _ := runLF(t, expr2, nil)
	assertBool(t, result2, false, "has_key not found")
}

func TestB2_HasKey_Null(t *testing.T) {
	expr := call("has_key", litNull(), litStr("key"))
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "has_key(null)")
}

// url_parse goldens

func TestB2_URLParse(t *testing.T) {
	expr := call("url_parse", litStr("https://example.com:8080/api/v1?key=val&foo=bar#section"))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "url_parse")
	assertString(t, obj["scheme"], "https", "url_parse.scheme")
	assertString(t, obj["host"], "example.com", "url_parse.host")
	assertInt(t, obj["port"], 8080, "url_parse.port")
	assertString(t, obj["path"], "/api/v1", "url_parse.path")
	assertString(t, obj["fragment"], "section", "url_parse.fragment")

	// query is an object
	query := assertObject(t, obj["query"], "url_parse.query")
	assertString(t, query["key"], "val", "url_parse.query.key")
	assertString(t, query["foo"], "bar", "url_parse.query.foo")
}

func TestB2_URLParse_NoPort(t *testing.T) {
	expr := call("url_parse", litStr("http://example.com/path"))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "url_parse no port")
	assertString(t, obj["scheme"], "http", "url_parse.scheme")
	assertNull(t, obj["port"], "url_parse.port null")
}

func TestB2_URLParse_Null(t *testing.T) {
	expr := call("url_parse", litNull())
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "url_parse(null)")
}

// ip_parse goldens

func TestB2_IPParse_V4Private(t *testing.T) {
	expr := call("ip_parse", litStr("192.168.1.1"))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "ip_parse v4 private")
	assertInt(t, obj["version"], 4, "ip_parse.version")
	assertBool(t, obj["private"], true, "ip_parse.private")
	assertBool(t, obj["loopback"], false, "ip_parse.loopback")
}

func TestB2_IPParse_V4Loopback(t *testing.T) {
	expr := call("ip_parse", litStr("127.0.0.1"))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "ip_parse loopback")
	assertInt(t, obj["version"], 4, "ip_parse.version")
	assertBool(t, obj["loopback"], true, "ip_parse.loopback")
}

func TestB2_IPParse_V4Public(t *testing.T) {
	expr := call("ip_parse", litStr("8.8.8.8"))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "ip_parse public")
	assertInt(t, obj["version"], 4, "ip_parse.version")
	assertBool(t, obj["private"], false, "ip_parse.private")
	assertBool(t, obj["loopback"], false, "ip_parse.loopback")
}

func TestB2_IPParse_V6(t *testing.T) {
	expr := call("ip_parse", litStr("::1"))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "ip_parse v6")
	assertInt(t, obj["version"], 6, "ip_parse.version")
	assertBool(t, obj["loopback"], true, "ip_parse.loopback")
}

func TestB2_IPParse_Invalid(t *testing.T) {
	expr := call("ip_parse", litStr("not-an-ip"))
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "ip_parse invalid")
}

func TestB2_IPParse_Null(t *testing.T) {
	expr := call("ip_parse", litNull())
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "ip_parse(null)")
}

// from_json produces native nested values

func TestB2_FromJSON_Object(t *testing.T) {
	expr := call("from_json", litStr(`{"name": "test", "count": 42}`))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "from_json object")
	assertString(t, obj["name"], "test", "from_json.name")
	assertInt(t, obj["count"], 42, "from_json.count")
}

func TestB2_FromJSON_Array(t *testing.T) {
	expr := call("from_json", litStr(`[1, "two", true, null]`))
	result, _ := runLF(t, expr, nil)
	arr := assertArray(t, result, 4, "from_json array")
	assertInt(t, arr[0], 1, "from_json[0]")
	assertString(t, arr[1], "two", "from_json[1]")
	assertBool(t, arr[2], true, "from_json[2]")
	assertNull(t, arr[3], "from_json[3]")
}

func TestB2_FromJSON_Nested(t *testing.T) {
	expr := call("from_json", litStr(`{"items": [{"id": 1}, {"id": 2}]}`))
	result, _ := runLF(t, expr, nil)
	obj := assertObject(t, result, "from_json nested")
	items := assertArray(t, obj["items"], 2, "from_json.items")
	item0 := assertObject(t, items[0], "from_json.items[0]")
	assertInt(t, item0["id"], 1, "from_json.items[0].id")
}

func TestB2_FromJSON_TypeAssertions(t *testing.T) {
	// Assert that from_json produces native Value types, not strings.
	expr := call("from_json", litStr(`{"a": [1, 2], "b": {"x": true}}`))
	result, _ := runLF(t, expr, nil)
	if result.Type() != event.FieldTypeObject {
		t.Fatalf("from_json top level should be object, got %s", result.Type())
	}
	obj := result.AsObject()
	if obj["a"].Type() != event.FieldTypeArray {
		t.Fatalf("from_json .a should be array, got %s", obj["a"].Type())
	}
	if obj["b"].Type() != event.FieldTypeObject {
		t.Fatalf("from_json .b should be object, got %s", obj["b"].Type())
	}
	innerObj := obj["b"].AsObject()
	if innerObj["x"].Type() != event.FieldTypeBool {
		t.Fatalf("from_json .b.x should be bool, got %s", innerObj["x"].Type())
	}
}

func TestB2_FromJSON_InvalidJSON(t *testing.T) {
	expr := call("from_json", litStr(`{invalid}`))
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "from_json invalid")
}

func TestB2_FromJSON_Null(t *testing.T) {
	expr := call("from_json", litNull())
	result, _ := runLF(t, expr, nil)
	assertNull(t, result, "from_json(null)")
}

func TestB2_FromJSON_Scalar(t *testing.T) {
	// from_json("42") => 42 (int)
	expr := call("from_json", litStr(`42`))
	result, _ := runLF(t, expr, nil)
	assertInt(t, result, 42, "from_json scalar int")

	// from_json("3.14") => 3.14 (float)
	expr2 := call("from_json", litStr(`3.14`))
	result2, _ := runLF(t, expr2, nil)
	assertFloat(t, result2, 3.14, "from_json scalar float")

	// from_json("true") => true (bool)
	expr3 := call("from_json", litStr(`true`))
	result3, _ := runLF(t, expr3, nil)
	assertBool(t, result3, true, "from_json scalar bool")

	// from_json(`"hello"`) => "hello" (string)
	expr4 := call("from_json", litStr(`"hello"`))
	result4, _ := runLF(t, expr4, nil)
	assertString(t, result4, "hello", "from_json scalar string")
}

// x in array-field

func TestB2_InArrayField(t *testing.T) {
	// x in tags where tags is an array field
	fields := map[string]event.Value{
		"tags": event.ArrayValue([]event.Value{
			event.StringValue("web"),
			event.StringValue("api"),
			event.StringValue("prod"),
		}),
	}

	// "api" in tags => true
	expr := inExpr(litStr("api"), ident("tags"))
	result, _ := runLF(t, expr, fields)
	assertBool(t, result, true, "api in tags")

	// "staging" in tags => false
	expr2 := inExpr(litStr("staging"), ident("tags"))
	result2, _ := runLF(t, expr2, fields)
	assertBool(t, result2, false, "staging in tags")
}

func TestB2_InArrayField_NullArray(t *testing.T) {
	fields := map[string]event.Value{
		"tags": event.NullValue(),
	}
	expr := inExpr(litStr("x"), ident("tags"))
	result, _ := runLF(t, expr, fields)
	assertNull(t, result, "x in null array")
}

func TestB2_InArrayField_NullValue(t *testing.T) {
	fields := map[string]event.Value{
		"tags": event.ArrayValue([]event.Value{
			event.StringValue("a"),
		}),
	}
	expr := inExpr(litNull(), ident("tags"))
	result, _ := runLF(t, expr, fields)
	assertNull(t, result, "null in tags")
}

func TestB2_InArrayField_IntArray(t *testing.T) {
	fields := map[string]event.Value{
		"codes": event.ArrayValue([]event.Value{
			event.IntValue(200),
			event.IntValue(404),
			event.IntValue(500),
		}),
	}
	expr := inExpr(litInt(404), ident("codes"))
	result, _ := runLF(t, expr, fields)
	assertBool(t, result, true, "404 in codes")

	expr2 := inExpr(litInt(302), ident("codes"))
	result2, _ := runLF(t, expr2, fields)
	assertBool(t, result2, false, "302 in codes")
}

// Lambda with field access from the row

func TestB2_Lambda_AccessesRowFields(t *testing.T) {
	// filter([1, 2, 3, 4, 5], x -> x > threshold) where threshold is a field
	fields := map[string]event.Value{
		"threshold": event.IntValue(3),
	}
	expr := call("filter",
		array(litInt(1), litInt(2), litInt(3), litInt(4), litInt(5)),
		lambda("x", binOp(lfast.OpGt, ident("x"), ident("threshold"))),
	)
	result, _ := runLF(t, expr, fields)
	arr := assertArray(t, result, 2, "filter with row field")
	assertInt(t, arr[0], 4, "filter field[0]")
	assertInt(t, arr[1], 5, "filter field[1]")
}

// Lambda: non-lambda arg error

func TestB2_Lambda_NonLambdaArg_Error(t *testing.T) {
	// any(arr, 42) should fail with a compile error
	expr := call("any", array(litInt(1)), litInt(42))
	_, err := CompileLynxFlow(expr)
	if err == nil {
		t.Fatal("expected error for non-lambda arg")
	}
}

// Assertion count summary
// TestB2_Any_Basic: 1
// TestB2_Any_NoMatch: 1
// TestB2_Any_EmptyArray: 1
// TestB2_Any_NullArray: 1
// TestB2_Any_3VL_Matrix: 7
// TestB2_All_3VL_Matrix: 7
// TestB2_All_EmptyArray: 1
// TestB2_Filter_Basic: 3
// TestB2_Filter_EmptyResult: 1
// TestB2_Filter_NullDropped: 3
// TestB2_Filter_EmptyArray: 1
// TestB2_Filter_ObjectsInArray: 2
// TestB2_Map_Basic: 4
// TestB2_Map_NullPreserved: 3
// TestB2_Map_EmptyArray: 1
// TestB2_Map_ExtractFromObjects: 3
// TestB2_Nested_Lambda_Any_Any: 1
// TestB2_Nested_Lambda_Any_Any_NotFound: 1
// TestB2_Nested_Lambda_Param_Shadowing: 5
// TestB2_Slice: 7 tests * ~2 = 14
// TestB2_ArrayConcat: 5
// TestB2_ArrayConcat_Three: 4
// TestB2_ArrayDistinct_Ints: 4
// TestB2_ArrayDistinct_DeepEquality: 4
// TestB2_ArrayDistinct_Empty: 1
// TestB2_ArraySort_Ints: 4
// TestB2_ArraySort_NullsLast: 3
// TestB2_ArraySort_Determinism: 4
// TestB2_ArraySort_Empty: 1
// TestB2_Flatten: 5
// TestB2_Flatten_OneLevel: 2
// TestB2_Flatten_Empty: 1
// TestB2_Keys: 3
// TestB2_Keys_Null: 1
// TestB2_Values: 3
// TestB2_Merge: 4
// TestB2_Merge_Null: 1
// TestB2_HasKey: 2
// TestB2_HasKey_Null: 1
// TestB2_URLParse: 7
// TestB2_URLParse_NoPort: 2
// TestB2_URLParse_Null: 1
// TestB2_IPParse_V4Private: 3
// TestB2_IPParse_V4Loopback: 2
// TestB2_IPParse_V4Public: 3
// TestB2_IPParse_V6: 2
// TestB2_IPParse_Invalid: 1
// TestB2_IPParse_Null: 1
// TestB2_FromJSON_Object: 3
// TestB2_FromJSON_Array: 4
// TestB2_FromJSON_Nested: 2
// TestB2_FromJSON_TypeAssertions: 4
// TestB2_FromJSON_InvalidJSON: 1
// TestB2_FromJSON_Null: 1
// TestB2_FromJSON_Scalar: 4
// TestB2_InArrayField: 2
// TestB2_InArrayField_NullArray: 1
// TestB2_InArrayField_NullValue: 1
// TestB2_InArrayField_IntArray: 2
// TestB2_Lambda_AccessesRowFields: 3
// TestB2_Lambda_NonLambdaArg_Error: 1
// Total: ~160 assertions
