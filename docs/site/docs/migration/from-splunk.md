---
title: Migrating from Splunk
description: Migrate from Splunk to LynxDB -- SPL to LynxFlow syntax mapping, HEC endpoint setup, data export, and forwarder configuration.
---

# Migrating from Splunk

LynxDB speaks LynxFlow, a pipeline query language in the same tradition as Splunk's SPL. The pipe-stage model and most stage names (`stats`, `sort`, `dedup`, `top`) carry over directly, so most SPL knowledge transfers. This guide covers the key differences and provides a step-by-step migration path.

:::note SPL2 history
LynxDB originally shipped an SPL2 dialect; it was replaced by LynxFlow v2 as a clean-break redesign. If you are migrating from an older LynxDB version rather than from Splunk, see the full capability mapping in `docs/grammar/RFC-002.md` §15 and the CHANGELOG section "Breaking Changes (LynxFlow vs SPL2)".
:::

## SPL vs LynxFlow Differences

### Index Selection

```
# Splunk SPL
index=main sourcetype=nginx

# LynxDB LynxFlow
from main sourcetype=nginx
```

The `from` stage accepts search sugar -- bare terms, quoted phrases, and `field=value` comparisons -- so Splunk-style search expressions mostly carry over after replacing `index=main` with `from main`:

```
from main _source=nginx status>=500 | stats count() by uri
```

### The Big Four Differences

1. **`==` compares, `=` binds**: In expressions, write `where status == 500`, not `where status = 500`. The from-stage search sugar (`level=error`) is the single exception.
2. **`count()` requires parentheses**: `stats count() by host`, not `stats count by host`.
3. **Renamed stages**: `eval` is now `extend`, `table`/`fields` are now `keep`/`drop`, `rex` is now `parse regex`, and `timechart` is now `every ... stats`.
4. **`search` is from-stage only**: Mid-pipeline, use `where has(_raw, "term")` or `where contains(_raw, "substring")` instead of `| search term`.

### Quick Syntax Reference

| Operation | Splunk SPL | LynxDB LynxFlow |
|-----------|-----------|-----------------|
| Select index | `index=main` | `from main` |
| Search | `search error` | `from main error` |
| Filter | `where status>500` | `where status > 500` |
| Aggregate | `stats count by host` | `stats count() by host` |
| Time chart | `timechart count span=5m` | `every 5m stats count()` |
| Time chart split by field | `timechart count by host span=5m` | `every 5m by host stats count()` |
| Top values | `top limit=10 uri` | `top 10 uri` |
| Field extraction | `rex field=_raw "(?<ip>\d+\.\d+\.\d+\.\d+)"` | `parse regex r"(?P<ip>\d+\.\d+\.\d+\.\d+)"` |
| Select columns | `table _time, level, message` | `keep _time, level, message` |
| Compute fields | `eval duration_sec=duration_ms/1000` | `extend duration_sec = duration_ms / 1000` |
| Fill nulls | `fillnull value=0 status` | `extend status = status ?? 0` |
| Rename | `rename src AS source_ip` | `rename src as source_ip` |
| Dedup | `dedup host` | `dedup host` |
| Subsearch / CTE | `[search index=threats \| fields ip]` | `let $threats = from threats \| keep ip;` |
| Macro | `` `my_macro` `` | Not supported |

### Stage Mapping

Stages that keep their Splunk names: `stats`, `sort`, `head`, `tail`, `dedup`, `rename`, `top`, `rare`, `streamstats`, `eventstats`, `join`, `union`, `transaction`, `where`.

Stages that were renamed:

| Splunk SPL | LynxFlow |
|-----------|----------|
| `eval` | `extend` |
| `table`, `fields` | `keep`, `drop` |
| `rex` | `parse regex` (plus `parse json`, `parse logfmt`) |
| `timechart` | `every <span> stats ...` or `stats ... by bin(_time, <span>)` |
| `bin` | `bin(_time, <span>)` as a grouping function |
| `fillnull` | `extend f = f ?? value` |
| `search` (mid-pipeline) | `where has(...)` / `where contains(...)` |

### Aggregation Functions

All common Splunk aggregation functions are supported (note that `count` requires parentheses):

`count()`, `sum`, `avg`, `min`, `max`, `dc` (distinct count), `estdc`, `values`, `list`, `stdev`, `var`, `mode`, `first`, `last`, `earliest`, `latest`, `perc(x, p)`, `p50`, `p75`, `p90`, `p95`, `p99`

Conditional aggregation replaces `eval`-wrapped tricks: `stats count(where status >= 500)`.

### Diagnostics

The parser reports errors with stable codes, caret spans, and suggestions, which catches most Splunk-isms immediately:

```
$ lynxdb query 'from main | stats count by host'
error[E013] at 1:19: count requires parentheses: count()
  from main | stats count by host
                    ^~~~~
  suggestion: count()
```

## Migration Steps

### Step 1: Set Up LynxDB

```bash
# Install
curl -fsSL https://lynxdb.org/install.sh | sh

# Start server
lynxdb server --data-dir /var/lib/lynxdb
```

### Step 2: Forward New Data via HEC

LynxDB includes a Splunk HTTP Event Collector (HEC) compatible endpoint. Point your existing Splunk forwarders at LynxDB with minimal configuration changes.

**Universal Forwarder (`outputs.conf`):**

```ini
# outputs.conf
[httpout]
httpEventCollectorToken = your-lynxdb-token
uri = https://lynxdb.company.com/api/v1/ingest/hec

[httpout:lynxdb]
uri = https://lynxdb.company.com/api/v1/ingest/hec
token = your-lynxdb-token
```

**Heavy Forwarder (`outputs.conf`):**

```ini
[tcpout]
disabled = true

[httpout]
disabled = false

[httpout:lynxdb]
uri = https://lynxdb.company.com/api/v1/ingest/hec
token = your-lynxdb-token
```

### Step 3: Export Historical Data from Splunk

Export data from Splunk for import into LynxDB:

```bash
# Export from Splunk as CSV
splunk search 'index=main earliest=-30d' -output csv > splunk_export.csv

# Or export as JSON
splunk search 'index=main earliest=-7d' -output json > splunk_export.json
```

Import into LynxDB:

```bash
# Import CSV export
lynxdb import splunk_export.csv --source splunk-migration

# Import JSON export
lynxdb import splunk_export.json --format ndjson

# Validate before importing
lynxdb import splunk_export.csv --dry-run

# Import into a dedicated index
lynxdb import splunk_export.csv --index splunk
```

### Step 4: Convert Saved Searches

Convert your Splunk saved searches to LynxDB saved queries:

```bash
# Splunk: index=main sourcetype=nginx status>=500 | stats count by uri | sort -count | head 10
# LynxDB:
lynxdb save "5xx-by-uri" 'from main _source=nginx status>=500 | stats count() by uri | sort -count | head 10'

# Run saved query
lynxdb run 5xx-by-uri --since 24h
```

## Cost Comparison

| | Splunk Enterprise | LynxDB |
|---|---|---|
| License | $2,000+/GB/day ingested | Free (Apache 2.0) |
| Infrastructure | 6+ components (indexer, search head, deployer, license server, etc.) | Single binary |
| Memory | ~8GB minimum per component | ~50MB idle |
| Scaling | Complex deployment server setup | Config flag change |

## Feature Comparison

| Feature | Splunk | LynxDB |
|---------|--------|--------|
| Query language | SPL | LynxFlow (SPL-inspired pipeline language) |
| Full-text search | tsidx | FST + roaring bitmaps |
| Schema | On-read | On-read |
| Materialized views | Data model acceleration | Materialized views (~400x) |
| Pipe mode | No | Yes |
| REST API | Yes | Yes (streaming-first) |
| HEC compatibility | Native | Compatible endpoint |

## Next Steps

- [LynxFlow Reference](/docs/lynxflow/overview) -- learn the full query language
- [Quick Start](/docs/getting-started/quickstart) -- get started with LynxDB
