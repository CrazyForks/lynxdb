---
title: Migrating from Grafana Loki
description: Migrate from Grafana Loki to LynxDB -- LogQL to LynxFlow conversion, Promtail/Alloy reconfiguration, and feature comparison.
---

# Migrating from Grafana Loki

LynxDB provides a fundamentally different approach to log analytics compared to Loki. While Loki indexes only labels and greps through log lines at query time, LynxDB builds a full-text inverted index with roaring bitmap posting lists, enabling sub-second full-text search without label cardinality constraints.

## Why Switch from Loki

- **Full-text search**: LynxDB indexes all content with FST + roaring bitmaps. Loki greps through unindexed log lines.
- **No label cardinality limits**: In Loki, high-cardinality labels (like `user_id` or `request_id`) cause performance degradation. LynxDB indexes all fields automatically.
- **Richer query language**: LynxFlow provides `let` bindings, joins, subqueries, and dozens of transformation stages. LogQL is limited to label matching and line filtering.
- **Schema-on-read**: LynxDB auto-discovers fields from JSON logs. Loki requires explicit label extraction at ingest time.
- **Simpler operations**: Single binary, no separate ingester/distributor/querier/compactor components.

## LogQL to LynxFlow Conversion

### Basic Queries

```
# Loki LogQL
{job="nginx"} |= "error"

# LynxDB LynxFlow
from main _source=nginx "error"
```

### Label Matching

```
# Loki LogQL
{job="nginx", level="error"}

# LynxDB LynxFlow
from main _source=nginx level=error
```

### Regex Filtering

```
# Loki LogQL
{job="nginx"} |~ "status=(4|5)\\d{2}"

# LynxDB LynxFlow
from main _source=nginx | parse regex r"status=(?P<status_code>\d{3})" | where int(status_code) >= 400
# Or simply:
from main _source=nginx | where status >= 400
```

### Aggregations

```
# Loki LogQL
sum(rate({job="nginx"} |= "error" [5m])) by (host)

# LynxDB LynxFlow
from main _source=nginx level=error | every 5m by host stats count()
```

```
# Loki LogQL
topk(10, sum(rate({job="nginx"} [1h])) by (uri))

# LynxDB LynxFlow
from main _source=nginx | stats count() by uri | sort -count | head 10
```

### JSON Extraction

```
# Loki LogQL
{job="api"} | json | duration_ms > 1000

# LynxDB LynxFlow (JSON fields are auto-extracted)
from main _source=api | where duration_ms > 1000
```

### Quick Reference

| LogQL | LynxFlow |
|-------|----------|
| `{job="nginx"}` | `from main _source=nginx` |
| `\|= "error"` | `"error"` in the `from` stage, or `\| where has(_raw, "error")` |
| `\|~ "pattern"` | `\| parse regex r"pattern"` or `\| where matches(_raw, r"pattern")` |
| `\| json` | (automatic for JSON logs; explicit: `\| parse json`) |
| `\| label_format` | `\| extend` or `\| rename` |
| `\| line_format` | `\| extend _raw = ...` |
| `sum(rate(...[5m]))` | `\| every 5m stats count()` |
| `count_over_time(...[1h])` | `\| stats count()` with `--since 1h` |
| `topk(10, ...)` | `\| sort -count \| head 10` |

## Migration Steps

### Step 1: Set Up LynxDB

```bash
curl -fsSL https://lynxdb.org/install.sh | sh
lynxdb server --data-dir /var/lib/lynxdb
```

### Step 2: Repoint Log Shippers

#### Promtail / Grafana Alloy

Promtail and Alloy can forward logs to LynxDB via the Elasticsearch-compatible `_bulk` endpoint or the native ingest API.

**Option A: Use Alloy with Elasticsearch output**

```hcl
// alloy config
loki.source.file "logs" {
  targets = [
    {__path__ = "/var/log/*.log"},
  ]
  forward_to = [otelcol.receiver.loki.default.receiver]
}

otelcol.receiver.loki "default" {
  output {
    logs = [otelcol.exporter.otlphttp.lynxdb.input]
  }
}

otelcol.exporter.otlphttp "lynxdb" {
  client {
    endpoint = "http://lynxdb:3100"
  }
}
```

**Option B: Use a Fluentd/Vector sidecar**

```toml
# Vector config
[sources.files]
type = "file"
include = ["/var/log/*.log"]

[sinks.lynxdb]
type = "elasticsearch"
inputs = ["files"]
endpoints = ["http://lynxdb:3100"]
api_version = "v7"
bulk.index = "logs"
```

#### OpenTelemetry Collector

LynxDB has a native OTLP/HTTP receiver:

```yaml
# otel-collector-config.yaml
exporters:
  otlp_http:
    endpoint: "http://lynxdb:3100/api/v1/otlp"
    encoding: json
    tls:
      insecure: true

service:
  pipelines:
    logs:
      receivers: [otlp]
      exporters: [otlp_http]
```

## Feature Comparison

| Feature | Grafana Loki | LynxDB |
|---------|-------------|--------|
| Full-text search | Line grep (no index) | FST + roaring bitmap inverted index |
| Query language | LogQL | LynxFlow (dozens of stages, `let` bindings, joins) |
| Schema | Labels only (static at ingest) | Schema-on-read (all fields indexed) |
| High cardinality | Degrades performance | Handled natively |
| Deployment | Ingester + Distributor + Querier + Compactor | Single binary |
| Object storage | Required in production | Optional (for tiering) |
| Materialized views | No | Yes (~400x acceleration) |
| Pipe mode (CLI) | No | Yes |
| Dashboards | Via Grafana | Built-in |
| License | AGPL | Apache 2.0 |

## Next Steps

- [LynxFlow Reference](/docs/lynxflow/overview) -- learn the full query language
- [Quick Start](/docs/getting-started/quickstart) -- get started in 5 minutes
- [Pipe Mode](/docs/getting-started/pipe-mode) -- query local files without a server
- [Migration from grep/awk](/docs/migration/from-grep) -- for CLI-first workflows
