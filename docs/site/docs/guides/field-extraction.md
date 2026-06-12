---
title: Extract Fields at Query Time
description: How to extract fields from unstructured logs at query time using parse regex, extend, and LynxDB's schema-on-read approach.
---

# Extract Fields at Query Time

LynxDB follows a schema-on-read philosophy: you do not need to define a schema before ingesting data. JSON and text logs remain queryable without an upfront schema, but each ingest transport has its own endpoint contract. Fields from JSON events are indexed automatically. For unstructured text logs, use [`parse regex`](/docs/lynxflow/operators/parse) (formerly `rex` in SPL2) and [`extend`](/docs/lynxflow/operators/extend) (formerly `eval`) to extract and compute fields at query time.

## How schema-on-read works

When you ingest data, LynxDB automatically discovers fields:

- **JSON events**: All top-level keys become searchable fields with their types preserved.
- **Raw text**: The full line is stored in the `_raw` field. You can extract structure from it at query time.

Check what fields LynxDB has discovered:

```bash
lynxdb fields
```

```
FIELD                     TYPE       COVERAGE   TOP VALUES
--------------------------------------------------------------------------------
_timestamp                datetime      100%
level                     string        100%    INFO(72%), ERROR(17%), WARN(11%)
status                    integer        50%    200(90%), 404(5%), 500(3%)
duration_ms               float          50%    min=0.1, max=30001.0, avg=145.3
source                    string        100%    nginx(50%), api-gw(37%), redis(13%)
```

See the [`lynxdb fields`](/docs/cli/shortcuts) command reference for details. Inside a query, the [`describe`](/docs/lynxflow/operators/describe) stage gives the same view of the *current* stream — one row per field with type, coverage, and top values:

```bash
lynxdb query 'from main | parse json | describe'
```

---

## Extract fields with `parse regex`

The [`parse regex`](/docs/lynxflow/operators/parse) stage extracts fields from a text field using named capture groups in a regular expression.

### Basic extraction

Given raw log lines like:

```
2026-01-15 14:23:01 host=web-01 service=api duration=245ms status=200
```

Extract `host`, `service`, `duration`, and `status`:

```bash
lynxdb query 'from main "duration"
  | parse regex r"host=(?P<host>\S+) service=(?P<service>\S+) duration=(?P<duration>\d+)ms status=(?P<status>\d+)"
  | keep _time, host, service, duration, status'
```

### Named capture group syntax

`parse regex` uses Go-style named capture groups: `(?P<field_name>pattern)`.

| Pattern | Matches |
|---------|---------|
| `(?P<ip>\d+\.\d+\.\d+\.\d+)` | An IPv4 address |
| `(?P<host>\S+)` | A non-whitespace token |
| `(?P<code>\d{3})` | A 3-digit status code |
| `(?P<path>[^ ]+)` | A path (no spaces) |
| `(?P<msg>.+)` | Everything to end of line |

:::note
Patterns are raw strings (`r"..."`): backslashes are literal, so `\d` and `\S` need no double-escaping. A raw string cannot contain a literal `"` — match a quote character with `\x22` instead.
:::

### Extract from Apache/Nginx access logs

For standard access-log formats, prefer the purpose-built [`parse combined`](/docs/lynxflow/operators/parse) (see [below](#structured-log-parsing-with-parse-formats)). The regex equivalent looks like this:

```bash
lynxdb query --file access.log '
  | parse regex r"(?P<client_ip>\S+) \S+ \S+ \[(?P<ts>[^\]]+)\] \x22(?P<method>\w+) (?P<uri>\S+) [^\x22]*\x22 (?P<status>\d+) (?P<bytes>\d+)"
  | stats count() as count by method, status
  | sort -count'
```

### Extract from application logs

```bash
lynxdb query 'from main "connection refused"
  | parse regex r"host=(?P<host>\S+)"
  | stats count() as count by host
  | sort -count'
```

### Extract from a non-default field

By default, `parse` operates on `_raw`. Use `from <field>` to extract from any field:

```bash
lynxdb query 'from main
  | parse regex r"user_id=(?P<uid>\d+)" from message
  | stats dc(uid) as unique_users'
```

### Typed captures

`into (...)` coerces captures at extraction time — no separate conversion step needed:

```bash
lynxdb query 'from main
  | parse regex r"duration=(?P<dur>\d+)ms" into (dur as int)
  | stats avg(dur) as avg_ms'
```

---

## Compute fields with `extend`

The [`extend`](/docs/lynxflow/operators/extend) stage (formerly `eval` in SPL2) creates new fields by evaluating expressions. Remember: `=` binds, `==` compares.

### Create a computed field

```bash
lynxdb query 'from nginx
  | extend duration_sec = duration_ms / 1000
  | keep uri, duration_ms, duration_sec'
```

### Conditional fields with if

```bash
lynxdb query 'from nginx
  | extend severity = if(status >= 500, "critical", if(status >= 400, "warning", "ok"))
  | stats count() as count by severity'
```

### Conditional fields with case

A trailing odd argument is the default — no `1=1` sentinel needed:

```bash
lynxdb query 'from nginx
  | extend category = case(
      status >= 500, "5xx",
      status >= 400, "4xx",
      status >= 300, "3xx",
      status >= 200, "2xx",
      "other"
    )
  | stats count() as count by category'
```

### String manipulation

```bash
# Convert to lowercase
lynxdb query 'from main | extend level_lower = lower(level) | stats count() as count by level_lower'

# Extract substring (0-based start; SPL2 substr was 1-based)
lynxdb query 'from main | extend short_path = substr(uri, 0, 20) | stats count() as count by short_path'

# String length
lynxdb query 'from main | extend msg_len = len(message) | where msg_len > 500 | keep _time, msg_len, message'
```

### Type conversion

The SPL2 `tonumber`/`tostring` functions are gone — cast with the type name:

```bash
# Convert a string field to a number
lynxdb query 'from main | extend status_num = int(status) | where status_num >= 500'

# Convert a number to string for display
lynxdb query 'from main | extend status_str = string(status) | keep status_str, uri'
```

`int()` and `float()` return null on failure; the strict variants `int!()` and `float!()` raise a query error instead.

### Coalesce (first non-null)

```bash
lynxdb query 'from main
  | extend display_time = coalesce(timestamp, `@timestamp`, _time)
  | keep display_time, message'
```

For a single fallback, the `??` operator is shorter: `region ?? "unknown"`. Field names with special characters (like `@timestamp`) are quoted with backticks.

### Time formatting

```bash
lynxdb query 'from main
  | extend human_time = strftime(_time, "%Y-%m-%d %H:%M:%S")
  | keep human_time, level, message'
```

See the [scalar functions reference](/docs/lynxflow/functions) for the complete list of available functions.

---

## Combine `parse regex` and `extend`

Extract typed values with `parse regex ... into`, then transform them with `extend`:

```bash
lynxdb query 'from main "request completed"
  | parse regex r"duration=(?P<dur>\d+)ms" into (dur as int)
  | extend is_slow = if(dur > 1000, "slow", "fast")
  | stats count() as count by is_slow'
```

---

## Array operations

LynxFlow arrays are first-class values (SPL2's multivalue `mv*` functions are gone). When a field contains an array — for example, from a `values()` aggregation or parsed JSON — use the native array functions:

```bash
# Join an array into a string (formerly mvjoin)
lynxdb query 'from main level=error | stats values(source) as sources by host | extend src_list = join(sources, ", ")'

# Deduplicate an array (formerly mvdedup)
lynxdb query 'from main | extend unique_tags = array_distinct(tags)'

# Concatenate values into one array (formerly mvappend)
lynxdb query 'from main | extend all_ids = array_concat([primary_id], [secondary_id])'
```

See the array section of the [scalar functions reference](/docs/lynxflow/functions) for `slice`, `flatten`, `filter`, `map`, and more.

---

## Null handling

LynxFlow distinguishes **null** (the field is present with an explicit null) from **missing** (the field was never extracted). SPL2's `isnull`/`isnotnull` are gone:

```bash
# Find events where a field is explicitly null
lynxdb query 'from main | where is_null(user_id) | stats count() as count by source'

# Find events that have a non-null field
lynxdb query 'from main | where exists(duration_ms) | stats avg(duration_ms)'

# Replace null/missing with a default
lynxdb query 'from main | extend region = region ?? "unknown" | stats count() as count by region'
```

`is_missing(f)` is true only when the field was never extracted at all.

---

## Field extraction on local files

All extraction stages work in pipe mode:

```bash
# Extract from a local file
lynxdb query --file /var/log/syslog '
  | parse regex r"(?P<process>\w+)\[(?P<pid>\d+)\]"
  | stats count() as count by process
  | sort -count
  | head 10'

# Extract from piped input
kubectl logs deploy/api | lynxdb query '
  | parse regex r"endpoint=(?P<ep>\S+) status=(?P<code>\d+) duration=(?P<dur>\d+)ms" into (dur as int)
  | stats avg(dur) as avg_ms, p99(dur) as p99_ms by ep'
```

---

## Structured log parsing with parse formats

For structured log formats, the unified [`parse`](/docs/lynxflow/operators/parse) stage has purpose-built named formats (formerly the 16 `unpack_*` commands) that are faster and more accurate than regex extraction:

| Format | Stage | Example input |
|--------|-------|---------------|
| JSON | `parse json` | `{"level":"error","msg":"timeout"}` |
| logfmt | `parse logfmt` | `level=error msg="request failed" duration=245ms` |
| Key=value | `parse kv` | `host=web-01 status=200 duration=45ms` |
| Syslog | `parse syslog` | `<134>Jan 15 14:23:01 web-01 nginx: connection reset` |
| Combined (access log) | `parse combined` | `10.0.1.5 - - [10/Oct/2025:13:55:36 -0700] "GET /api HTTP/1.1" 200 2326 "-" "curl/7.64"` |
| CLF | `parse clf` | `127.0.0.1 - frank [10/Oct/2025:13:55:36 -0700] "GET /api HTTP/1.1" 200 2326` |
| Nginx error | `parse nginx_error` | `2026/02/14 14:52:01 [error] 12345#67: *890 message, client: 10.0.1.5` |
| CEF | `parse cef` | `CEF:0\|Vendor\|Product\|1.0\|100\|Alert\|7\|src=10.0.0.1` |

Additional named formats: `docker`, `redis`, `apache_error`, `postgres`, `mysql_slow`, `haproxy`, `leef`, `w3c`. Fallback chains try formats in order per row: `parse first_of(json, logfmt)`.

```bash
# Parse logfmt and aggregate
cat app.log | lynxdb query '| parse logfmt | stats count() as count by level'

# Parse nginx access logs
lynxdb query --file access.log '| parse combined | where status >= 500 | stats count() as count by uri'
```

For JSON-specific workflows (object access, `from_json`, `explode`), see the [Working with JSON Logs](/docs/guides/json-processing) guide.

---

## Next steps

- [Working with JSON Logs](/docs/guides/json-processing) -- parse json, object access, explode
- [Search and filter logs](/docs/guides/search-and-filter) -- filter before extracting fields
- [Run aggregations](/docs/guides/aggregations) -- aggregate over extracted fields
- [parse reference](/docs/lynxflow/operators/parse) -- full parse syntax, formats, and options
- [extend reference](/docs/lynxflow/operators/extend) -- full extend syntax
- [Scalar functions reference](/docs/lynxflow/functions) -- complete function list
