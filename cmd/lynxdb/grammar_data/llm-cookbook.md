# LynxFlow LLM Cookbook

Prompt patterns and examples for translating natural language to LynxDB LynxFlow queries.

LynxFlow is the v2 query language defined by [RFC-002](RFC-002.md). It replaces
the SPL2 surface with a single expression grammar, typed values, and a
registry-driven operator/function catalog. The 18 spec examples in RFC-002 S13
are canonical; this cookbook adds NL-to-query patterns for LLM integrations.

---

## 1. System Prompt Template

Use this system prompt for NL-to-LynxFlow translation tasks:

```
You are an expert at translating natural language questions into LynxDB LynxFlow queries.

## Rules
- Output ONLY the LynxFlow query. No explanation unless asked.
- Pipelines start with `from <source>` (implicit `from main` when omitted).
- Use `where <expr>` for filtering; `has(_raw, "term")` for full-text search.
- Use pipe (|) to chain stages. Stages: from, where, parse, extend, keep, drop,
  rename, stats, eventstats, streamstats, sort, head, tail, dedup, join, union,
  explode, describe, top, rare, every, rate, latency, percentiles, proportion,
  facets, impact, baseline, changes, exemplars, patterns, compare, outliers,
  sessionize, transaction, trace, topology, correlate, rollup, xyseries,
  materialize, tee, use.
- `==` for comparison, `=` for assignment/options only.
- Strings use double quotes. Raw-string regex: r"...".
- count() requires parentheses. Conditional: count(where status >= 500).
- Aggregation functions: count, sum, avg, min, max, dc, estdc, perc, p50, p75,
  p90, p95, p99, stdev, var, mode, first, last, earliest, latest, values, list,
  rate, per_second.
- Scalar functions: if, case, coalesce, nullif, exists, is_null, is_missing,
  typeof, int, float, string, bool, timestamp, duration, len, lower, upper,
  trim, substr, replace, split, join, starts_with, ends_with, printf, has,
  contains, glob, matches, extract, abs, round, floor, ceil, sqrt, ln, log,
  pow, bin, now, strftime, strptime, md5, sha256, cidr_match, from_json,
  to_json, keys, values, merge, slice, filter, map, any, all.
- `eval` is now `extend`; `table`/`fields` is now `keep`/`drop`.
- `rex` is now `parse regex r"..."`.
- `timechart` is now `every <dur> stats ...` or `stats ... by bin(_time, dur)`.
- `fillnull` is now `extend f = f ?? default_value`.
- CTEs use `let`: `let $x = <pipeline>;`.
```

---

## 2. Schema Injection Pattern

Inject your field catalog into the prompt so the LLM generates valid field names.

```
## Available Fields
| Field         | Type      | Coverage | Example Values              |
|---------------|-----------|----------|-----------------------------|
| _time         | timestamp | 100%     | 2026-03-23T14:30:00Z       |
| _raw          | string    | 100%     | Full log line text          |
| _source       | string    | 100%     | nginx, api-gw, redis        |
| level         | string    | 100%     | INFO, WARN, ERROR, DEBUG    |
| status        | int       | 50%      | 200, 404, 500              |
| duration_ms   | float     | 50%      | 0.1 to 30001.0             |
| host          | string    | 80%      | web-01, web-02, api-01     |
| endpoint      | string    | 60%      | /api/users, /health         |
| method        | string    | 40%      | GET, POST, PUT, DELETE      |
| client_ip     | string    | 70%      | 192.168.1.1                |
| user_id       | int       | 30%      | 1 to 99999                 |
| message       | string    | 95%      | Human-readable log message  |
| service       | string    | 85%      | auth, billing, gateway      |
```

### Dynamic injection example

```
## Available Fields
{{#each fields}}
| {{name}} | {{type}} | {{coverage}}% | {{top_values}} |
{{/each}}
```

---

## 3. Few-Shot Examples

Include 5-10 examples covering common LynxFlow patterns:

```
Q: Count errors by source
A: from main | where level == "error" | stats count() by _source

Q: Average latency per endpoint over the last hour
A: from main[-1h] | stats avg(duration_ms) by endpoint

Q: Show error rate over time in 5-minute buckets
A: from main | where level == "error" | every 5m stats count()

Q: Top 10 slowest endpoints
A: from main | stats avg(duration_ms) as avg_dur by endpoint | sort -avg_dur | head 10

Q: Find all 500 errors from nginx
A: from nginx | where status == 500

Q: Extract user agent and count requests
A: from main | parse regex r"user_agent\":\"(?<ua>[^\"]+)\"" | stats count() by ua | sort -count

Q: Compare error counts across services
A: from main | where level == "error" | stats count() by _source | sort -count

Q: Create a 5-minute error summary view
A: from main | where level == "error" | stats count() by _source, bin(_time, 5m) | materialize "mv_errors_5m" retention=90d

Q: Find IP addresses hitting the server most
A: from main | stats count() by client_ip | sort -count | head 20

Q: Show percentile latencies by endpoint
A: from main | latency duration_ms every 5m by endpoint

Q: What proportion of requests are errors per service?
A: from main | proportion status >= 500 as error_rate by service

Q: Show rolling baseline and anomalies for error rate
A: from main | every 1h stats count() as errors | baseline errors window=24

Q: Parse JSON logs and filter on nested fields
A: from app | parse json | where user.role == "admin" and any(tags, t -> t.name == "vip")

Q: Handle null fields with defaults
A: from main | where exists(amount) | extend amount = amount ?? 0

Q: Compare current hour to previous hour
A: from nginx[-1h] | stats count() by host | compare previous 1h

Q: Show field coverage and types
A: from main | parse json | describe

Q: Find session patterns
A: from main | sessionize maxpause=30m by user_id

Q: Auto-detect log format and parse
A: from main | parse first_of(json, logfmt) | keep _time, service, status, duration_ms | sort -duration_ms | head 50
```

---

## 4. Error Correction Loop

When the LLM generates invalid LynxFlow, use this correction pattern:

### Step 1: Parse and validate

```
lynxdb query --explain '<generated_query>'
```

### Step 2: Inject error feedback

```
The previous query had an error:

  Error: unknown command "eval" at position 12
  Hint: did you mean "extend"?

Available stages: from, where, parse, extend, keep, drop, rename, stats,
eventstats, streamstats, sort, head, tail, dedup, join, union, explode,
describe, top, rare, every, rate, latency, percentiles, proportion, facets,
impact, baseline, changes, exemplars, patterns, compare, outliers, sessionize,
transaction, trace, topology, correlate, rollup, xyseries, materialize, tee, use.

Please correct the query.
```

### Step 3: Retry prompt template

```
Your previous LynxFlow query was invalid:

  Query: {original_query}
  Error: {error_message}
  Suggestion: {hint_if_available}

Common mistakes (SPL2 -> LynxFlow):
- eval -> extend:          `| extend is_err = status >= 500`
- table/fields -> keep/drop: `| keep host, status` or `| drop _raw`
- rex -> parse regex:      `| parse regex r"user=(?<user>\w+)"`
- timechart -> every:      `| every 5m stats count()`
- fillnull -> extend+??: `| extend f = f ?? 0`
- count without parens:    `count()` not `count`
- = vs ==:                 `== ` for comparison, `=` for assignment/options
- index=x:                 `from x` (not `index=x`)
- search "term":           `where has(_raw, "term")`
- isnotnull(f):            `exists(f)`
- perc95(f):               `p95(f)` or `perc(f, 95)`
- percentile95/exactperc95: killed; use `p95(f)` or `perc(f, 95)`
- mean(f):                  killed; use `avg(f)`
- dc/distinct_count:        use `dc(f)`
- $x = query;:             `let $x = query;`

Generate a corrected query:
```

### Automated correction pipeline

```python
def generate_and_correct(nl_query, max_retries=3):
    system_prompt = LYNXFLOW_SYSTEM_PROMPT
    schema = load_field_catalog()

    for attempt in range(max_retries):
        query = llm.generate(system_prompt, schema, nl_query)
        result = subprocess.run(
            ["lynxdb", "query", "--explain", query],
            capture_output=True, text=True
        )

        if result.returncode == 0:
            return query

        error = result.stderr
        hint = extract_hint(error)
        nl_query = f"""Previous attempt failed:
Query: {query}
Error: {error}
Hint: {hint}
Please fix the query."""

    raise MaxRetriesExceeded()
```

---

## 5. Common Patterns

### 5.1 Filter patterns

| Natural language | LynxFlow |
|---|---|
| "show errors" | `from main \| where level == "error"` |
| "from nginx" | `from nginx` |
| "status 500" | `from main \| where status == 500` |
| "slow requests" | `from main \| where duration_ms > 1000` |
| "search for X" | `from main \| where has(_raw, "X")` |
| "not debug" | `from main \| where level != "debug"` |
| "errors from nginx" | `from nginx \| where level == "error"` |
| "500 or 502 errors" | `from main \| where status in [500, 502]` |

### 5.2 Aggregation patterns

| Natural language | LynxFlow |
|---|---|
| "count events" | `\| stats count()` |
| "count by source" | `\| stats count() by _source` |
| "average latency" | `\| stats avg(duration_ms)` |
| "max duration" | `\| stats max(duration_ms)` |
| "unique users" | `\| stats dc(user_id)` |
| "95th percentile" | `\| stats p95(duration_ms)` |
| "top 10 URIs" | `\| top 10 uri` |
| "error count only" | `\| stats count(where level == "error") as errors` |

### 5.3 Time series patterns

| Natural language | LynxFlow |
|---|---|
| "errors per minute" | `\| where level == "error" \| every 1m stats count()` |
| "hourly rate" | `\| every 1h stats count()` |
| "by service over time" | `\| every 5m by service stats count()` |
| "daily aggregation" | `\| stats count() by bin(_time, 1d)` |

### 5.4 Transform patterns

| Natural language | LynxFlow |
|---|---|
| "extract IP" | `\| parse regex r"(?<ip>\d+\.\d+\.\d+\.\d+)"` |
| "add flag field" | `\| extend is_error = status >= 500` |
| "rename column" | `\| rename duration_ms as latency` |
| "select columns" | `\| keep _time, _source, level, message` |
| "fill missing values" | `\| extend duration_ms = duration_ms ?? 0` |
| "parse JSON logs" | `\| parse json` |
| "parse mixed formats" | `\| parse first_of(json, logfmt)` |

### 5.5 Advanced patterns

| Natural language | LynxFlow |
|---|---|
| "join with user data" | `\| join type=left on user_id with [from users]` |
| "group into sessions" | `\| sessionize maxpause=30m by user_id` |
| "create a view" | `\| materialize "name" retention=90d` |
| "rolling average" | `\| streamstats window=10 avg(duration_ms)` |
| "cumulative sum" | `\| streamstats running_sum(count) as total` |
| "latency summary" | `\| latency duration_ms every 5m by endpoint` |
| "error proportion" | `\| proportion status >= 500 as error_rate by service` |
| "field distribution" | `\| facets service, host limit=5` |
| "detect anomalies" | `\| baseline error_rate window=12 by service` |
| "track changes" | `\| changes version by service` |

### 5.6 Edge cases to handle

1. **`==` for comparison**: `status == 500` (never `status = 500` outside search sugar)
2. **`=` for assignment/options**: `extend x = 1`, `type=inner`, `maxpause=30m`
3. **count() requires parens**: `stats count()` not `stats count`
4. **Conditional aggregates**: `count(where status >= 500)` not `count(eval(...))`
5. **CTE syntax**: `let $name = pipeline;` then `from $name`
6. **Duration units**: `5m`, `1h`, `30s`, `1d`, `1w` — no month/year
7. **Standard boolean precedence**: `and` binds tighter than `or` (unlike SPL2 search)
8. **Raw-string regex**: `r"pattern"` not `"pattern"` in parse regex
9. **Null handling**: `exists(f)` for non-null check, `f ?? default` for coalesce
10. **Array literals**: `[1, 2, 3]` not `(1, 2, 3)` for `in` expressions

---

## 6. Token Budget Optimization

For long prompts, prioritize:

1. **System prompt** (~250 tokens): Always include
2. **Schema fields** (~50-200 tokens): Include top 15-20 fields by coverage
3. **Few-shot examples** (~400-600 tokens): 5-10 examples covering key patterns
4. **Error context** (~100 tokens): Only on retry

Total budget: ~800-1150 tokens for the prompt, leaving room for query generation.

### Minimal prompt (for token-constrained models)

```
Translate to LynxFlow. Rules: from <src> starts pipeline, | where <expr> filters,
| stats func() by group, | every 5m stats count(), | extend x = expr, | sort -f,
| head N, | keep f1, f2, | drop f1, | parse json, | parse regex r"...",
| join type=left on k with [...], let $cte = ...; from $cte.
Aggregations: count(), sum, avg, min, max, dc, p50-p99, perc(x,p).
Comparison: ==, !=, <, >, <=, >=, in [...], between x and y.
Functions: if, case, coalesce, exists, has, contains, glob, matches, int, float,
string, bin, now, len, lower, upper.
No explanation. Query only.
```

---

## 7. Validation Checklist

Before deploying an NL-to-LynxFlow system:

- [ ] System prompt covers all stages your users need
- [ ] Schema includes fields with >10% coverage
- [ ] Few-shot examples match your data domain
- [ ] Error correction loop is wired up
- [ ] Edge cases (`==` vs `=`, `count()` parens, duration units) are documented
- [ ] Token budget fits your model's context window
- [ ] Generated queries are validated with `lynxdb query --explain`
- [ ] Output format is constrained (query only, no explanation)
- [ ] SPL2 migration terms are in the correction prompt (eval->extend, etc.)
