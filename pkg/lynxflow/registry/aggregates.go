package registry

// aggregates is the frozen v1 aggregate and window-function surface
// (RFC-002 §10). The percN alias family replaces the RFC-001 percentile zoo.
var aggregates = []Aggregate{
	{Name: "count", Params: []Param{{Name: "x", Type: TAny, Optional: true}}, SupportsWhere: true, Result: TInt, Doc: "count() counts rows; count(x) counts non-null x; count(where p) counts matching rows."},
	{Name: "sum", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TNumber, Doc: "Nulls skipped; all-null group yields null."},
	{Name: "avg", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat},
	{Name: "min", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny},
	{Name: "max", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny},
	{Name: "dc", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TInt, Doc: "Distinct count; exact below 10K, HLL above."},
	{Name: "estdc", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TInt, Doc: "Always-HLL distinct count."},
	{Name: "perc", Params: []Param{{Name: "x", Type: TNumber}, {Name: "p", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "T-digest percentile; p in [0, 100]."},
	{Name: "p50", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Alias for perc(x, 50)."},
	{Name: "p75", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Alias for perc(x, 75)."},
	{Name: "p90", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Alias for perc(x, 90)."},
	{Name: "p95", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Alias for perc(x, 95)."},
	{Name: "p99", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Alias for perc(x, 99)."},
	{Name: "stdev", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Sample standard deviation."},
	{Name: "var", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "Sample variance."},
	{Name: "mode", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny},
	{Name: "first", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny, Doc: "First non-null in row order."},
	{Name: "last", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny, Doc: "Last non-null in row order."},
	{Name: "earliest", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny, Doc: "Value from the row with the smallest _time."},
	{Name: "latest", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TAny, Doc: "Value from the row with the largest _time."},
	{Name: "values", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TArray, Doc: "Distinct non-null values as an array."},
	{Name: "list", Params: []Param{{Name: "x", Type: TAny}}, SupportsWhere: true, Result: TArray, Doc: "All non-null values as an array, row order."},
	{Name: "rate", SupportsWhere: true, Result: TFloat, Doc: "Row count divided by the group's time-bucket span."},
	{Name: "per_second", Params: []Param{{Name: "x", Type: TNumber}}, SupportsWhere: true, Result: TFloat, Doc: "sum(x) divided by the group's time-bucket span in seconds."},

	// ---- window (streamstats only) -------------------------------------------
	{Name: "lag", Params: []Param{{Name: "x", Type: TAny}, {Name: "n", Type: TInt, Optional: true}}, WindowOnly: true, Result: TAny, Doc: "Value n rows back (default 1)."},
	{Name: "lead", Params: []Param{{Name: "x", Type: TAny}, {Name: "n", Type: TInt, Optional: true}}, WindowOnly: true, Result: TAny, Doc: "Value n rows ahead (default 1)."},
	{Name: "row_number", WindowOnly: true, Result: TInt, Doc: "1-based row index within the group."},
	{Name: "running_sum", Params: []Param{{Name: "x", Type: TNumber}}, WindowOnly: true, Result: TNumber},
	{Name: "moving_avg", Params: []Param{{Name: "x", Type: TNumber}, {Name: "n", Type: TInt}}, WindowOnly: true, Result: TFloat},
}
