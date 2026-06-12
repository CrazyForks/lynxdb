package registry

// operators is the frozen v1 stage-operator surface (RFC-002 §8, §9).
var operators = []Operator{
	// source
	{
		Name: "from", Class: ClassSource, Streaming: StreamingRow,
		Positionals: []Positional{
			{Name: "sources", Type: ArgFieldPatterns, Required: true, Doc: "source names, globs, !-excludes, *, or $cte refs; optional [range] suffix; optional trailing search-sugar terms"},
		},
		Doc:      "Scan stage. Only valid first in a pipeline. Accepts bracket time ranges and search-sugar terms (RFC-002 §3.1).",
		Examples: []string{`from nginx[-1h] timeout status>=500`, `from logs*,!logs-debug*[-7d..-1d]`, `from $errs`},
	},

	// core
	{
		Name: "where", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "predicate", Type: ArgPredicate, Required: true}},
		Doc:         "Typed predicate filter.",
		Examples:    []string{`where status >= 500 and has(_raw, "timeout")`},
	},
	{
		Name: "parse", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "format", Type: ArgFormat, Required: true, Doc: "json, logfmt, kv(...), pattern \"...\", regex r\"...\", a named format, or first_of(f1, f2, ...)"}},
		Options: []Option{
			{Name: "from", Type: ArgField, Default: "_raw", Doc: "input field"},
			{Name: "into", Type: ArgCaptures, Doc: "typed captures: into (status as int, dur as duration)"},
			{Name: "prefix", Type: ArgString, Doc: "namespace prefix for extracted fields"},
			{Name: "on_error", Type: ArgEnum, Default: "propagate", Enum: []string{"propagate", "null", "drop", "strict"}},
		},
		Doc:      "Schema-on-read extraction stage (RFC-002 §7). Never deletes columns; never silently overwrites non-null fields.",
		Examples: []string{`parse json`, `parse first_of(json, logfmt)`, `parse regex r"user=(?<user>\w+)" into (user as string)`},
	},
	{
		Name: "extend", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "assignments", Type: ArgAssignList, Required: true}},
		Doc:         "Add or replace computed columns, evaluated left to right.",
		Examples:    []string{`extend is_err = status >= 500, amount = amount ?? 0`},
	},
	{
		Name: "keep", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "fields", Type: ArgFieldPatterns, Required: true}},
		Doc:         "Projection, order-preserving. Supports globs and `* except f1, f2`.",
		Examples:    []string{`keep _time, service, status`, `keep * except _raw`},
	},
	{
		Name: "drop", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "fields", Type: ArgFieldPatterns, Required: true}},
		Doc:         "Remove columns. Supports globs.",
		Examples:    []string{`drop _raw, trace_*`},
	},
	{
		Name: "rename", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "renames", Type: ArgAssignList, Required: true, Doc: "old as new, ..."}},
		Doc:         "Rename columns.",
		Examples:    []string{`rename duration_ms as latency`},
	},
	{
		Name: "stats", Class: ClassCore, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "aggs", Type: ArgAggList, Required: true}},
		Options:     []Option{{Name: "by", Type: ArgFieldList, Doc: "group keys; bin(_time, dur) allowed and emits as _time"}},
		Doc:         "Grouped aggregation. count() requires parens; conditional aggregates via count(where p) / sum(x, where p).",
		Examples:    []string{`stats count(), avg(dur) by service`, `stats count(where status >= 500) as errors, count() as total by service`},
	},
	{
		Name: "eventstats", Class: ClassCore, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "aggs", Type: ArgAggList, Required: true}},
		Options:     []Option{{Name: "by", Type: ArgFieldList}},
		Doc:         "Aggregates appended to every row without collapsing.",
		Examples:    []string{`eventstats avg(duration_ms) as global_avg`},
	},
	{
		Name: "streamstats", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "aggs", Type: ArgAggList, Required: true, Doc: "aggregates or window functions (lag, lead, row_number, ...)"}},
		Options: []Option{
			{Name: "window", Type: ArgInt, Doc: "sliding window size in rows; 0 = all preceding"},
			{Name: "current", Type: ArgBool, Default: "true", Doc: "include the current row"},
			{Name: "by", Type: ArgFieldList},
		},
		Doc:      "Running/windowed values in row order.",
		Examples: []string{`streamstats window=3 avg(duration_ms) as rolling_avg`, `streamstats row_number() as rk by host`},
	},
	{
		Name: "sort", Class: ClassCore, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "keys", Type: ArgSortList, Required: true, Doc: "-f desc, +f or f asc"}},
		Doc:         "External merge sort, spill-capable. Nulls last ascending, first descending.",
		Examples:    []string{`sort -count, service`},
	},
	{
		Name: "head", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "n", Type: ArgInt, Required: true}},
		Doc:         "First N rows (TopK pushdown after sort).",
		Examples:    []string{`head 10`},
	},
	{
		Name: "tail", Class: ClassCore, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "n", Type: ArgInt, Required: true}},
		Doc:         "Last N rows.",
		Examples:    []string{`tail 5`},
	},
	{
		Name: "dedup", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{
			{Name: "n", Type: ArgInt, Doc: "rows kept per key (default 1)"},
			{Name: "fields", Type: ArgFieldList, Required: true},
		},
		Doc:      "Keep first N (default 1) rows per key.",
		Examples: []string{`dedup service`, `dedup 3 service, host`},
	},
	{
		Name: "join", Class: ClassCore, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "right", Type: ArgSubPipeline, Required: true, Doc: "with $cte or with [ <pipeline> ]"}},
		Options: []Option{
			{Name: "type", Type: ArgEnum, Default: "inner", Enum: []string{"inner", "left", "outer"}},
			{Name: "on", Type: ArgFieldList, Required: true},
		},
		Doc:      "Hash join. Default type=inner is a plain inner join (never innerunique).",
		Examples: []string{`join type=left on user_id with [from users]`},
	},
	{
		Name: "union", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "pipelines", Type: ArgSubPipeline, Required: true, Variadic: true}},
		Doc:         "Append rows from sub-pipelines; schemas merge by name with null-padding.",
		Examples:    []string{`union [from audit[-1h] | where res == "failed"]`},
	},
	{
		Name: "explode", Class: ClassCore, Streaming: StreamingRow,
		Positionals: []Positional{
			{Name: "array", Type: ArgField, Required: true},
			{Name: "as", Type: ArgField, Doc: "element output field (default: the array field name)"},
		},
		Doc:      "One row per array element; rows with missing/empty arrays are dropped.",
		Examples: []string{`explode tags as tag`},
	},
	{
		Name: "describe", Class: ClassCore, Streaming: StreamingAcc,
		Doc:      "Stream schema/coverage summary: field, type, coverage, distinct_est, top_values (RFC-002 §7.4).",
		Examples: []string{`parse json | describe`},
	},

	// sugar (mechanical desugar, RFC-002 §9.1)
	{
		Name: "top", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{
			{Name: "n", Type: ArgInt, Doc: "default 10"},
			{Name: "field", Type: ArgField, Required: true},
		},
		DesugarsTo: "stats count() as count by <field> | sort -count | head <n>",
		Doc:        "Top-N frequent values.",
		Examples:   []string{`top 10 uri`},
	},
	{
		Name: "rare", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{
			{Name: "n", Type: ArgInt, Doc: "default 10"},
			{Name: "field", Type: ArgField, Required: true},
		},
		DesugarsTo: "stats count() as count by <field> | sort +count | head <n>",
		Doc:        "Bottom-N frequent values.",
		Examples:   []string{`rare 3 service`},
	},
	{
		Name: "count", Class: ClassSugar, Streaming: StreamingAcc,
		Options:    []Option{{Name: "by", Type: ArgFieldList}},
		DesugarsTo: "stats count() as count [by <fields>]",
		Doc:        "Row count, optionally per group.",
		Examples:   []string{`count`, `count by host`},
	},
	{
		Name: "every", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{
			{Name: "span", Type: ArgDuration, Required: true},
			{Name: "aggs", Type: ArgAggList, Required: true, Doc: "introduced by the stats keyword"},
		},
		Options:    []Option{{Name: "by", Type: ArgFieldList}},
		DesugarsTo: "stats <aggs> by [<keys>,] bin(_time, <span>)",
		Doc:        "Time-bucketed aggregation.",
		Examples:   []string{`every 5m by service stats count()`},
	},
	{
		Name: "rate", Class: ClassSugar, Streaming: StreamingAcc,
		Options: []Option{
			{Name: "per", Type: ArgDuration, Default: "1m"},
			{Name: "by", Type: ArgFieldList},
		},
		DesugarsTo: "every <per> [by <keys>] stats count() as rate",
		Doc:        "Event count per time bucket.",
		Examples:   []string{`rate per 5m by service`},
	},
	{
		Name: "latency", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "field", Type: ArgField, Required: true}},
		Options: []Option{
			{Name: "every", Type: ArgDuration},
			{Name: "by", Type: ArgFieldList},
		},
		DesugarsTo: "stats p50(<f>), p95(<f>), p99(<f>), count() [by <keys>, bin(_time, <every>)]",
		Doc:        "Latency percentile summary. The metric field is required (no guessed defaults).",
		Examples:   []string{`latency duration_ms every 5m by endpoint`},
	},
	{
		Name: "percentiles", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "field", Type: ArgField, Required: true}},
		Options:     []Option{{Name: "by", Type: ArgFieldList}},
		DesugarsTo:  "stats p50(<f>) as p50_<f>, p75(<f>) as p75_<f>, p90(<f>) as p90_<f>, p95(<f>) as p95_<f>, p99(<f>) as p99_<f> [by <keys>]",
		Doc:         "Five-point percentile summary.",
		Examples:    []string{`percentiles duration_ms by service`},
	},
	{
		Name: "proportion", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{
			{Name: "predicate", Type: ArgPredicate, Required: true},
			{Name: "as", Type: ArgField, Required: true, Doc: "alias is mandatory"},
		},
		Options: []Option{
			{Name: "every", Type: ArgDuration},
			{Name: "by", Type: ArgFieldList},
		},
		DesugarsTo: "stats count(where <pred>) as <name>_num, count() as <name>_den [by ...] | extend <name> = <name>_num / <name>_den",
		Doc:        "Matching events divided by all events, denominator visible.",
		Examples:   []string{`proportion status >= 500 as error_rate by service`},
	},
	{
		Name: "facets", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "fields", Type: ArgFieldList, Required: true}},
		Options:     []Option{{Name: "limit", Type: ArgInt, Default: "10"}},
		DesugarsTo:  "union of per-field `stats count() as count by <f> | sort -count | head <limit> | extend _facet = \"<f>\", _value = string(<f>) | keep _facet, _value, count`",
		Doc:         "Top values per requested field in one result (_facet/_value/count columns).",
		Examples:    []string{`facets service, host limit=5`},
	},
	{
		Name: "impact", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "agg", Type: ArgAggList, Doc: "default count()"}},
		Options:     []Option{{Name: "by", Type: ArgFieldList, Required: true}},
		DesugarsTo:  "stats <agg> as v by <keys> | eventstats sum(v) as total_v | extend pct_v = v / total_v | sort -pct_v",
		Doc:         "Contribution percentage per group.",
		Examples:    []string{`impact sum(bytes) by host`},
	},
	{
		Name: "baseline", Class: ClassSugar, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "field", Type: ArgField, Required: true}},
		Options: []Option{
			{Name: "window", Type: ArgInt, Required: true},
			{Name: "by", Type: ArgFieldList},
		},
		DesugarsTo: "streamstats current=false window=<n> avg(<f>) as baseline_<f>, stdev(<f>) as stdev_<f> [by <keys>] | extend delta_<f> = <f> - baseline_<f>, z_<f> = if(stdev_<f> > 0, delta_<f> / stdev_<f>, null)",
		Doc:        "Rolling baseline, delta, and z-score from previous rows.",
		Examples:   []string{`baseline error_rate window=12 by service`},
	},
	{
		Name: "changes", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "field", Type: ArgField, Required: true}},
		Options:     []Option{{Name: "by", Type: ArgFieldList}},
		DesugarsTo:  "sort +_time | streamstats current=false last(<f>) as previous_<f> [by <keys>] | where exists(previous_<f>) and <f> != previous_<f>",
		Doc:         "Rows where a field changed relative to the previous row in the same group.",
		Examples:    []string{`changes version by service`},
	},
	{
		Name: "exemplars", Class: ClassSugar, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "n", Type: ArgInt, Doc: "default 3"}},
		Options:     []Option{{Name: "by", Type: ArgFieldList}},
		DesugarsTo:  "sort -_time | dedup <n> <keys>  (global: sort -_time | head <n>)",
		Doc:         "Newest representative rows, globally or per group.",
		Examples:    []string{`exemplars 5 by endpoint`},
	},

	// helpers (runtime operators, RFC-002 §9.2)
	{
		Name: "patterns", Class: ClassHelper, Streaming: StreamingAcc,
		Options: []Option{
			{Name: "field", Type: ArgField, Default: "_raw"},
			{Name: "max_templates", Type: ArgInt},
			{Name: "similarity", Type: ArgString},
		},
		Doc:      "Group similar messages into Drain templates. Logical-IR node.",
		Examples: []string{`patterns field=message`},
	},
	{
		Name: "compare", Class: ClassHelper, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "shift", Type: ArgDuration, Required: true, Doc: "optionally preceded by the keyword previous"}},
		Doc:         "Re-run the pipeline prefix over the previous window; adds previous_*/change_* columns. Logical-IR node.",
		Examples:    []string{`compare previous 1h`},
	},
	{
		Name: "outliers", Class: ClassHelper, Streaming: StreamingAcc,
		Options: []Option{
			{Name: "field", Type: ArgField, Required: true},
			{Name: "method", Type: ArgEnum, Default: "iqr", Enum: []string{"iqr", "zscore", "mad"}},
			{Name: "threshold", Type: ArgString},
		},
		Doc:      "Mark statistical outliers using the selected method.",
		Examples: []string{`outliers field=duration_ms method=zscore threshold=2.0`},
	},
	{
		Name: "sessionize", Class: ClassHelper, Streaming: StreamingAcc,
		Options: []Option{
			{Name: "maxpause", Type: ArgDuration, Default: "30m"},
			{Name: "by", Type: ArgFieldList},
		},
		Doc:      "Add session id/start/end fields based on time gaps within each group.",
		Examples: []string{`sessionize maxpause=30m by user_id`},
	},
	{
		Name: "transaction", Class: ClassHelper, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "fields", Type: ArgFieldList, Required: true}},
		Options: []Option{
			{Name: "maxspan", Type: ArgDuration},
			{Name: "startswith", Type: ArgPredicate},
			{Name: "endswith", Type: ArgPredicate},
		},
		Doc:      "Group events into transactions keyed by fields.",
		Examples: []string{`transaction user_id maxspan=30m`},
	},
	{
		Name: "trace", Class: ClassHelper, Streaming: StreamingAcc,
		Options: []Option{
			{Name: "trace_id", Type: ArgField, Default: "trace_id"},
			{Name: "span_id", Type: ArgField, Default: "span_id"},
			{Name: "parent_id", Type: ArgField, Default: "parent_id"},
		},
		Doc:      "Build a span tree from trace/span fields; adds depth/tree fields.",
		Examples: []string{`where trace_id == "req-abc-123" | trace`},
	},
	{
		Name: "topology", Class: ClassHelper, Streaming: StreamingAcc,
		Options: []Option{
			{Name: "source_field", Type: ArgField, Default: "service"},
			{Name: "dest_field", Type: ArgField, Default: "downstream"},
			{Name: "weight_field", Type: ArgField},
			{Name: "max_nodes", Type: ArgInt},
		},
		Doc: "Build edge/node summaries from source/destination fields.",
	},
	{
		Name: "correlate", Class: ClassHelper, Streaming: StreamingAcc,
		Positionals: []Positional{
			{Name: "field1", Type: ArgField, Required: true},
			{Name: "field2", Type: ArgField, Required: true},
		},
		Options:  []Option{{Name: "method", Type: ArgEnum, Default: "pearson", Enum: []string{"pearson", "spearman"}}},
		Doc:      "Correlation between two numeric fields.",
		Examples: []string{`correlate duration_ms cpu_pct method=pearson`},
	},
	{
		Name: "rollup", Class: ClassHelper, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "resolutions", Type: ArgDuration, Required: true, Variadic: true}},
		Options:     []Option{{Name: "by", Type: ArgFieldList}},
		Doc:         "Multiple time resolutions in one stream; adds _resolution.",
		Examples:    []string{`rollup 1m, 1h by service`},
	},
	{
		Name: "xyseries", Class: ClassHelper, Streaming: StreamingAcc,
		Positionals: []Positional{
			{Name: "x", Type: ArgField, Required: true},
			{Name: "y", Type: ArgField, Required: true},
			{Name: "value", Type: ArgField, Required: true},
		},
		Doc:      "Pivot rows into a matrix (x rows, y columns).",
		Examples: []string{`stats count() by service, level | xyseries service level count`},
	},

	// management
	{
		Name: "materialize", Class: ClassManagement, Streaming: StreamingAcc,
		Positionals: []Positional{{Name: "name", Type: ArgString, Required: true}},
		Options: []Option{
			{Name: "retention", Type: ArgDuration},
			{Name: "partition_by", Type: ArgFieldList},
		},
		Doc:      "Terminal stage: create a materialized view from the current pipeline.",
		Examples: []string{`stats count() by service, bin(_time, 5m) | materialize "mv_errors_5m" retention=90d`},
	},
	{
		Name: "tee", Class: ClassManagement, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "sink", Type: ArgString, Required: true}},
		Doc:         "Side effect: additionally send JSON rows to a sink without interrupting the stream.",
	},
	{
		Name: "use", Class: ClassManagement, Streaming: StreamingRow,
		Positionals: []Positional{{Name: "fragment", Type: ArgString, Required: true}},
		Doc:         "Expand a named pipeline fragment at parse time. Missing fragments are explicit errors.",
		Examples:    []string{`use @ops/error-filter`},
	},
}
