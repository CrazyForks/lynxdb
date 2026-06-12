---
sidebar_position: 2
title: query
description: Execute LynxFlow queries against a LynxDB server, local files, or stdin.
---

# query

Execute a LynxFlow query against a running server, a local file, or stdin.

```
lynxdb query [LynxFlow query] [flags]
```

**Alias:** `q`

## Flags

| Flag | Short | Default | Description |
|------|-------|---------|-------------|
| `--since` | `-s` | | Relative time range (e.g., `15m`, `1h`, `7d`) |
| `--from` | | | Absolute start time (ISO 8601) |
| `--to` | | | Absolute end time (ISO 8601) |
| `--file` | `-f` | | Query a local file instead of server |
| `--sourcetype` | | | Override source type detection (file/stdin mode) |
| `--source` | | | Set source metadata (file/stdin mode) |
| `--output` | `-o` | | Write results to file |
| `--timeout` | | | Query timeout (e.g., `10s`, `5m`) |
| `--analyze` | | | Profile query execution (`basic`, `full`, `trace`) |
| `--max-memory` | | | Max memory for ephemeral query (e.g., `512mb`, `1gb`) |
| `--fail-on-empty` | | `false` | Exit with code 6 if no results |
| `--copy` | | `false` | Copy results to clipboard as TSV |
| `--explain` | | `false` | Show query plan without executing |
| `--raw` | | `false` | Disable auto-format detection |
| `--no-lint` | | `false` | Disable advisory query lints |
| `--no-suggestions` | | `false` | Disable advisory query suggestions |
| `--show-rewritten` | | `false` | Show normalized query rewrites |
| `--queries-file` | | | Run one LynxFlow query per non-empty line from a file, or `-` for stdin |
| `--language` | | | Query language. Only `lynxflow` is accepted; `spl2` is rejected with a migration error |
| `--param` | `-D` | | Set query parameter (`--param name=value`); see [Query parameters](#query-parameters) |

The query argument is required unless `--queries-file` is set. `from main` is prepended automatically when the query starts with `|`. A query that starts with a stage name (such as `where`) is also valid — LynxFlow scans the default source implicitly.

`--analyze` accepts an optional value; bare `--analyze` defaults to `basic`.

## Three Execution Modes

### 1. Server mode (default)

Sends the query to a running LynxDB server via HTTP. This is the default when no `--file` flag is set and stdin is a terminal.

```bash
lynxdb query 'from main level=error | stats count() by source'
lynxdb q 'from main | where status >= 500 | top 10 path' --since 24h
```

### 2. File mode (`--file`)

Ingests file(s) into an ephemeral in-memory engine and queries locally. No server required. Glob patterns are supported.

```bash
lynxdb query --file access.log '| stats count() by status'
lynxdb query --file '/var/log/*.log' '| where level == "ERROR" | stats count() by source'
```

### 3. Stdin mode (pipe detected)

Reads stdin into an ephemeral engine and queries locally. Detected automatically when stdin is not a terminal. No server required.

```bash
cat app.json | lynxdb query '| stats count() by level'
kubectl logs deploy/api | lynxdb query '| where duration_ms > 1000 | stats avg(duration_ms) by endpoint'
```

You cannot use `--file` and stdin simultaneously. `--copy` is only supported in server mode.

## Examples

### Basic queries

```bash
# Query a running server
lynxdb query 'from main level=error | stats count() by source'
lynxdb q 'from main | where status >= 500 | top 10 path' --since 24h

# Short form (from main is auto-prepended)
lynxdb query '| stats count() by source'
lynxdb query 'where level == "error" | stats count()'
```

### Time ranges

```bash
# Relative time range
lynxdb query 'from main level=error | stats count()' --since 1h

# Absolute time range
lynxdb query 'from main | stats count() by host' \
  --from 2026-01-15T00:00:00Z --to 2026-01-15T23:59:59Z
```

### Local files and stdin

```bash
# Query local files (no server needed)
lynxdb query --file access.log '| stats count() by status'
lynxdb query --file '/var/log/*.log' '| where level == "ERROR" | stats count() by source'

# Pipe from stdin (no server needed)
cat app.json | lynxdb query '| stats count() by level'
kubectl logs deploy/api | lynxdb query '| where duration_ms > 1000 | stats avg(duration_ms) by endpoint'
```

### Profiling and debugging

```bash
# Profile query performance
lynxdb query 'from main level=error | stats count()' --analyze
lynxdb query 'from main level=error | stats count()' --analyze full

# Show query plan without executing
lynxdb query 'from main level=error | stats count() by source' --explain

# Query with timeout
lynxdb query 'from main level=error | stats count()' --timeout 30s
```

### Query parameters

The `--param` flag is accepted for backward compatibility, but `${name}` placeholder substitution was removed together with SPL2. For parameterized pipelines, use LynxFlow `let` bindings instead:

```bash
lynxdb query --file events.ndjson 'let $errors = from main | where level == "ERROR"; from $errors | stats count()' --format ndjson --no-stats
```

Expected output:

```json
{"count":2}
```

### Output control

```bash
# Force output format
lynxdb query 'from main | stats count() by host' --format csv
lynxdb query 'from main | stats count() by host' --format table

# Write results to file
lynxdb query 'from main level=error' --output errors.json

# Copy results to clipboard
lynxdb query 'from main level=error | stats count() by source' --copy
```

### Scripting

```bash
# Exit code 6 when empty (for CI/scripting)
lynxdb query 'from main level=FATAL' --fail-on-empty || echo "No fatal errors found"

# Pipe query results to jq
lynxdb query 'from main | stats count() by host' | jq '.host'

# Export to file
lynxdb query 'from main | where level == "ERROR"' --since 24h > errors.json

# Chain queries (query server, then local post-process)
lynxdb query 'from main | where status >= 500' --since 1h \
  | lynxdb query '| stats count() by path'

# Export as CSV
lynxdb query 'from main | stats count() by host' --format csv > report.csv

# Run one query per non-empty line
lynxdb query --queries-file rules.lynxflow --since 24h --format ndjson
```

## Console Output

### Server mode, TTY

When stdout is a terminal and format is `auto`, the query prints formatted results with stats:

```
  . Scanning segments...  1.23s

    Memtable:  4,200 events
    Segments:  3 scanned / 5 to scan / 12 total
    Skipped:   7 (bloom:3, time:2, index:1, stats:1)
    Rows read: 156,000
```

On completion, results are shown as colorized JSON with stats:

```
  #1 {
    "host": "web-01",
    "count": 42530
  }

  --------------------------------------------------
  Results:      2 results
  Scanned:      150,000 events  (149,998 filtered, 100.0%)
  Segments:     5 scanned / 12 total  (7 skipped)
  Query time:   45ms
```

### Server mode, pipe (non-TTY)

When stdout is not a terminal (pipe), output is newline-delimited JSON:

```json
{"host":"web-01","count":42530}
{"host":"api-01","count":31204}
```

Stats and metadata are written to stderr so they do not interfere with piped data.

To force machine-readable output in a terminal:

```bash
lynxdb query 'from main | stats count()' --format json
```

## See Also

- [Output Formats](/docs/cli/output-formats) for details on `--format` options
- [Shortcuts](/docs/cli/shortcuts) for `count`, `sample`, `last`, and `explain` shortcuts
- [Interactive Shell](/docs/cli/shell) for the REPL interface
- [LynxFlow v2 Reference](/docs/lynxflow/overview) for the query language reference
