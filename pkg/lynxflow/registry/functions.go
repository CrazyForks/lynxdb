package registry

// functions is the frozen v1 scalar-function surface (RFC-002 §10).
var functions = []Function{
	// ---- conversion (strict ! variants available) -----------------------------
	{Name: "int", Category: "conversion", Params: []Param{{Name: "x", Type: TAny}}, Result: TInt, Fallibility: NullOnFailure, StrictVariant: true, Doc: "Cast to int; null on failure."},
	{Name: "float", Category: "conversion", Params: []Param{{Name: "x", Type: TAny}}, Result: TFloat, Fallibility: NullOnFailure, StrictVariant: true, Doc: "Cast to float; null on failure."},
	{Name: "string", Category: "conversion", Params: []Param{{Name: "x", Type: TAny}}, Result: TString, Fallibility: Infallible, Doc: "Render any value as a string."},
	{Name: "bool", Category: "conversion", Params: []Param{{Name: "x", Type: TAny}}, Result: TBool, Fallibility: NullOnFailure, StrictVariant: true, Doc: "Cast to bool; null on failure."},
	{Name: "timestamp", Category: "conversion", Params: []Param{{Name: "x", Type: TAny}, {Name: "layout", Type: TString, Optional: true}}, Result: TTimestamp, Fallibility: NullOnFailure, StrictVariant: true, Doc: "Parse RFC3339 (or layout) to timestamp; null on failure."},
	{Name: "duration", Category: "conversion", Params: []Param{{Name: "x", Type: TString}}, Result: TDuration, Fallibility: NullOnFailure, StrictVariant: true, Doc: "Parse a duration string (\"100ms\", \"5m\"); numbers use n * 1ms instead."},

	// ---- conditional / null ----------------------------------------------------
	{Name: "if", Category: "conditional", Params: []Param{{Name: "cond", Type: TBool}, {Name: "then", Type: TAny}, {Name: "else", Type: TAny}}, Result: TAny, Fallibility: Infallible, Doc: "Null condition yields null."},
	{Name: "case", Category: "conditional", Params: []Param{{Name: "pairs", Type: TAny, Variadic: true}}, Result: TAny, Fallibility: Infallible, Doc: "case(cond1, v1, cond2, v2, ...[, default]); trailing odd argument is the default."},
	{Name: "coalesce", Category: "conditional", Params: []Param{{Name: "values", Type: TAny, Variadic: true}}, Result: TAny, Fallibility: Infallible, Doc: "First non-null, non-missing argument."},
	{Name: "nullif", Category: "conditional", Params: []Param{{Name: "a", Type: TAny}, {Name: "b", Type: TAny}}, Result: TAny, Fallibility: Infallible, Doc: "Null when a == b, else a."},
	{Name: "exists", Category: "conditional", Params: []Param{{Name: "field", Type: TAny}}, Result: TBool, Fallibility: Infallible, Doc: "True when the field is present with a non-null value."},
	{Name: "is_null", Category: "conditional", Params: []Param{{Name: "field", Type: TAny}}, Result: TBool, Fallibility: Infallible, Doc: "True when present with an explicit null value."},
	{Name: "is_missing", Category: "conditional", Params: []Param{{Name: "field", Type: TAny}}, Result: TBool, Fallibility: Infallible, Doc: "True when the field was never extracted."},
	{Name: "typeof", Category: "conditional", Params: []Param{{Name: "x", Type: TAny}}, Result: TString, Fallibility: Infallible, Doc: "Type name: string, int, float, bool, timestamp, duration, array, object, null, missing."},

	// ---- string -----------------------------------------------------------------
	{Name: "len", Category: "string", Params: []Param{{Name: "x", Type: TAny}}, Result: TInt, Fallibility: NullOnFailure, Doc: "Length of a string (runes) or array (elements)."},
	{Name: "lower", Category: "string", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "upper", Category: "string", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "trim", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "chars", Type: TString, Optional: true}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "ltrim", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "chars", Type: TString, Optional: true}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "rtrim", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "chars", Type: TString, Optional: true}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "substr", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "start", Type: TInt}, {Name: "len", Type: TInt, Optional: true}}, Result: TString, Fallibility: NullOnFailure, Doc: "0-based start; negative counts from end."},
	{Name: "replace", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "pattern", Type: TRegex}, {Name: "with", Type: TString}}, Result: TString, Fallibility: NullOnFailure, Doc: "Regex replace all."},
	{Name: "split", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "sep", Type: TString}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "join", Category: "string", Params: []Param{{Name: "arr", Type: TArray}, {Name: "sep", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "starts_with", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "prefix", Type: TString}}, Result: TBool, Fallibility: NullOnFailure},
	{Name: "ends_with", Category: "string", Params: []Param{{Name: "s", Type: TString}, {Name: "suffix", Type: TString}}, Result: TBool, Fallibility: NullOnFailure},
	{Name: "printf", Category: "string", Params: []Param{{Name: "format", Type: TString}, {Name: "args", Type: TAny, Variadic: true}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "urldecode", Category: "string", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "url_parse", Category: "string", Params: []Param{{Name: "s", Type: TString}}, Result: TObject, Fallibility: NullOnFailure, Doc: "Parse a URL into {scheme, host, port, path, query, fragment}."},
	{Name: "path_normalize", Category: "string", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "useragent_parse", Category: "string", Params: []Param{{Name: "s", Type: TString}}, Result: TObject, Fallibility: NullOnFailure, Doc: "Optional build."},

	// ---- text search (index-honest cost tiers, RFC-002 §6) ----------------------
	{Name: "has", Category: "search", Params: []Param{{Name: "field", Type: TString}, {Name: "term", Type: TString}}, Result: TBool, Fallibility: NullOnFailure, Doc: "Whole-token match, always case-insensitive; FST term index. Fast."},
	{Name: "contains", Category: "search", Params: []Param{{Name: "field", Type: TString}, {Name: "sub", Type: TString}}, Result: TBool, Fallibility: NullOnFailure, Doc: "Substring, case-insensitive; bloom-assisted scan. Moderate."},
	{Name: "contains_cs", Category: "search", Params: []Param{{Name: "field", Type: TString}, {Name: "sub", Type: TString}}, Result: TBool, Fallibility: NullOnFailure, Doc: "Case-sensitive substring."},
	{Name: "glob", Category: "search", Params: []Param{{Name: "field", Type: TString}, {Name: "pattern", Type: TString}}, Result: TBool, Fallibility: NullOnFailure, Doc: "Glob match, case-sensitive; literal-prefix extraction when possible."},
	{Name: "has_glob", Category: "search", Params: []Param{{Name: "field", Type: TString}, {Name: "pattern", Type: TString}}, Result: TBool, Fallibility: NullOnFailure, Doc: "Whole-token glob match (*, ?, \\-escapes), always case-insensitive; FST term-dictionary expansion. Moderate."},

	// ---- regex --------------------------------------------------------------------
	{Name: "matches", Category: "regex", Params: []Param{{Name: "s", Type: TString}, {Name: "pattern", Type: TRegex}}, Result: TBool, Fallibility: NullOnFailure, Doc: "Regex match (linear-time engine). Slow tier; (?i) for case-insensitive."},
	{Name: "extract", Category: "regex", Params: []Param{{Name: "s", Type: TString}, {Name: "pattern", Type: TRegex}}, Result: TString, Fallibility: NullOnFailure, Doc: "First capture group."},
	{Name: "extract_all", Category: "regex", Params: []Param{{Name: "s", Type: TString}, {Name: "pattern", Type: TRegex}}, Result: TArray, Fallibility: NullOnFailure},

	// ---- math -----------------------------------------------------------------------
	{Name: "abs", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TNumber, Fallibility: NullOnFailure},
	{Name: "round", Category: "math", Params: []Param{{Name: "x", Type: TNumber}, {Name: "digits", Type: TInt, Optional: true}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "floor", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TInt, Fallibility: NullOnFailure},
	{Name: "ceil", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TInt, Fallibility: NullOnFailure},
	{Name: "sqrt", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "ln", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "log", Category: "math", Params: []Param{{Name: "x", Type: TNumber}, {Name: "base", Type: TNumber, Optional: true}}, Result: TFloat, Fallibility: NullOnFailure, Doc: "Base 10 by default."},
	{Name: "exp", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "pow", Category: "math", Params: []Param{{Name: "x", Type: TNumber}, {Name: "y", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "clamp", Category: "math", Params: []Param{{Name: "x", Type: TNumber}, {Name: "lo", Type: TNumber}, {Name: "hi", Type: TNumber}}, Result: TNumber, Fallibility: NullOnFailure},
	{Name: "bucket", Category: "math", Params: []Param{{Name: "x", Type: TNumber}, {Name: "bounds", Type: TArray}}, Result: TNumber, Fallibility: NullOnFailure, Doc: "Snap x to the largest bound <= x."},
	{Name: "sin", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "cos", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "tan", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "asin", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "acos", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "atan", Category: "math", Params: []Param{{Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},
	{Name: "atan2", Category: "math", Params: []Param{{Name: "y", Type: TNumber}, {Name: "x", Type: TNumber}}, Result: TFloat, Fallibility: NullOnFailure},

	// ---- time ------------------------------------------------------------------------
	{Name: "now", Category: "time", Result: TTimestamp, Fallibility: Infallible, Doc: "Query start time (stable within one query)."},
	{Name: "bin", Category: "time", Params: []Param{{Name: "ts", Type: TTimestamp}, {Name: "span", Type: TDuration}}, Result: TTimestamp, Fallibility: NullOnFailure, Doc: "Snap to span boundary; in stats by-lists the binned key emits as _time."},
	{Name: "strftime", Category: "time", Params: []Param{{Name: "ts", Type: TTimestamp}, {Name: "format", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "strptime", Category: "time", Params: []Param{{Name: "s", Type: TString}, {Name: "format", Type: TString}}, Result: TTimestamp, Fallibility: NullOnFailure, StrictVariant: true},
	{Name: "time_of_day", Category: "time", Params: []Param{{Name: "ts", Type: TTimestamp}}, Result: TDuration, Fallibility: NullOnFailure},
	{Name: "day_of_week", Category: "time", Params: []Param{{Name: "ts", Type: TTimestamp}}, Result: TInt, Fallibility: NullOnFailure, Doc: "0 = Sunday."},

	// ---- hash / network -----------------------------------------------------------------
	{Name: "md5", Category: "hash", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "sha1", Category: "hash", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "sha256", Category: "hash", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "xxhash64", Category: "hash", Params: []Param{{Name: "s", Type: TString}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "cidr_match", Category: "network", Params: []Param{{Name: "cidr", Type: TString}, {Name: "ip", Type: TString}}, Result: TBool, Fallibility: NullOnFailure},
	{Name: "ip_parse", Category: "network", Params: []Param{{Name: "s", Type: TString}}, Result: TObject, Fallibility: NullOnFailure},
	{Name: "ipmask", Category: "network", Params: []Param{{Name: "mask", Type: TString}, {Name: "ip", Type: TString}}, Result: TString, Fallibility: NullOnFailure},

	// ---- array -----------------------------------------------------------------------------
	{Name: "slice", Category: "array", Params: []Param{{Name: "arr", Type: TArray}, {Name: "start", Type: TInt}, {Name: "end", Type: TInt, Optional: true}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "array_concat", Category: "array", Params: []Param{{Name: "arrays", Type: TArray, Variadic: true}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "array_distinct", Category: "array", Params: []Param{{Name: "arr", Type: TArray}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "array_sort", Category: "array", Params: []Param{{Name: "arr", Type: TArray}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "flatten", Category: "array", Params: []Param{{Name: "arr", Type: TArray}}, Result: TArray, Fallibility: NullOnFailure, Doc: "One level."},
	{Name: "any", Category: "array", Params: []Param{{Name: "arr", Type: TArray}, {Name: "pred", Type: TLambda}}, Result: TBool, Fallibility: NullOnFailure, Doc: "any(tags, t -> t.name == \"vip\")"},
	{Name: "all", Category: "array", Params: []Param{{Name: "arr", Type: TArray}, {Name: "pred", Type: TLambda}}, Result: TBool, Fallibility: NullOnFailure},
	{Name: "filter", Category: "array", Params: []Param{{Name: "arr", Type: TArray}, {Name: "pred", Type: TLambda}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "map", Category: "array", Params: []Param{{Name: "arr", Type: TArray}, {Name: "fn", Type: TLambda}}, Result: TArray, Fallibility: NullOnFailure},

	// ---- object ------------------------------------------------------------------------------
	{Name: "keys", Category: "object", Params: []Param{{Name: "obj", Type: TObject}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "values", Category: "object", Params: []Param{{Name: "obj", Type: TObject}}, Result: TArray, Fallibility: NullOnFailure},
	{Name: "merge", Category: "object", Params: []Param{{Name: "a", Type: TObject}, {Name: "b", Type: TObject}}, Result: TObject, Fallibility: NullOnFailure, Doc: "Right side wins on key collision."},
	{Name: "has_key", Category: "object", Params: []Param{{Name: "obj", Type: TObject}, {Name: "key", Type: TString}}, Result: TBool, Fallibility: NullOnFailure},
	{Name: "to_json", Category: "object", Params: []Param{{Name: "x", Type: TAny}}, Result: TString, Fallibility: NullOnFailure},
	{Name: "from_json", Category: "object", Params: []Param{{Name: "s", Type: TString}}, Result: TAny, Fallibility: NullOnFailure, StrictVariant: true, Doc: "Null on invalid JSON, never the original string."},
}
