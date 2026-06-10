# Changelog

All notable changes to LynxDB will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- **LynxFlow v2 query language** ([RFC-002](docs/grammar/RFC-002.md)): Clean-break language redesign replacing the SPL2 surface. One expression grammar with standard boolean precedence (AND before OR), typed values (int/float/string/bool/timestamp/duration/array/object/null/missing), and a registry-driven operator/function/aggregate catalog.
  - **18 core operators**: `from`, `where`, `parse`, `extend`, `keep`, `drop`, `rename`, `stats`, `eventstats`, `streamstats`, `sort`, `head`, `tail`, `dedup`, `join`, `union`, `explode`, `describe`.
  - **12 sugar operators** with mechanical desugaring (`--show-rewritten`): `top`, `rare`, `every`, `rate`, `latency`, `percentiles`, `proportion`, `facets`, `impact`, `baseline`, `changes`, `exemplars`.
  - **13 helper operators**: `patterns`, `compare`, `outliers`, `sessionize`, `transaction`, `trace`, `topology`, `correlate`, `rollup`, `xyseries`, `materialize`, `tee`, `use`.
  - **78 scalar functions** across 10 categories (conversion, conditional, string, search, regex, math, time, hash, network, array, object) with `name!` strict variants for fallible functions.
  - **26 aggregate functions** including `perc(x, p)` and the `p50/p75/p90/p95/p99` alias family, plus 5 window-only functions (`lag`, `lead`, `row_number`, `running_sum`, `moving_avg`).
  - **Unified `parse` stage** replacing 16 `unpack_*` commands: `parse json`, `parse logfmt`, `parse regex r"..."`, `parse first_of(json, logfmt)`, etc.
  - **Conditional aggregates**: `count(where status >= 500)` replaces `count(eval(status>=500))`.
  - **Array/object literals and lambdas**: `[1, 2, 3]`, `{key: value}`, `any(tags, t -> t.name == "vip")`.
  - **Null/missing distinction**: `exists(f)` (non-null present), `is_null(f)` (explicit null), `is_missing(f)` (never extracted), `f ?? default` coalesce.
  - **Registry-generated documentation**: `go run ./internal/docgen` generates operator pages, function/aggregate tables, and EBNF grammar from the single source of truth (`pkg/lynxflow/registry`). CI drift-guard test prevents stale docs.
- **Dual-runtime migration path**: Both SPL2 and LynxFlow parsers are active. Query routing: explicit `language=lynxflow` or `language=spl2` always wins; ambiguous queries auto-detect and route to LynxFlow; killed SPL2 spellings (`mean`, `percentile95`, etc.) fall through to SPL2 for backward compatibility. Use `lynxdb mv migrate` to convert saved queries and materialized views.

### Changed

- **LLM cookbook** (`docs/grammar/llm-cookbook.md`): Rewritten for LynxFlow with updated system prompt, few-shot examples, error correction patterns, and SPL2-to-LynxFlow migration hints.
- **EBNF grammar**: New `docs/grammar/lynxflow.ebnf` generated from registry; the old `docs/grammar/spl2.ebnf` is retained during the dual-runtime window.
- **Docs site sidebar**: New "LynxFlow v2 Reference" section with registry-generated operator pages and function/aggregate tables. Old SPL2 pages moved to "Legacy SPL2" collapsed section.

### Breaking Changes (LynxFlow vs SPL2)

- **Expression grammar**: SEARCH precedence killed; standard boolean precedence everywhere (`and` before `or`).
- **`==` for comparison, `=` for assignment**: `status == 500` not `status = 500` in expressions.
- **`count()` requires parentheses**: `stats count()` not `stats count`.
- **Renamed commands**: `eval` -> `extend`, `table`/`fields +/-` -> `keep`/`drop`, `rex` -> `parse regex`, `timechart` -> `every ... stats` or `stats ... by bin(_time, dur)`, `fillnull` -> `extend f = f ?? v`, `unpack_*` -> `parse <format>`.
- **Killed commands**: `search` (mid-pipeline; use `where has(...)`), `chart`, `untable`, `reverse`, `appendcols`, `appendpipe`, `fieldformat`, `makemv`, `mvcombine`, `mvexpand`, `nomv`, `makeresults`, `addinfo`, `convert`, `fieldsummary`, `flatten`, `iplocation`, `lookup`, `regex` command, `replace` command, `tags`, `thru`, `timewrap`, `tstats`, `mstats`.
- **Killed functions/aggregates**: `mean` (use `avg`), `percentile95`/`exactperc95`/`upperperc95`/`median` zoo (use `p95`/`perc(x, 95)`/`p50`), `tonumber`/`toint`/`todouble` (use `int`/`float`), `isnull`/`isnotnull` (use `is_null`/`exists`), `mv*` family (use native array functions), `json_*` string family (use `parse json` + object access).
- **CTE syntax**: `$x = query;` -> `let $x = query;`.
- **Comments**: `--` killed; use `//` or `/* */`.
- **Quoted identifiers**: Single-quote identifiers killed; use backticks.
- **F-strings**: Killed; use `printf` or string concatenation.

### Removed

- **Dashboards**: Entire dashboards feature removed — REST API (`/api/v1/dashboards`), CLI (`lynxdb dashboards`), Go client library, persistent store, and Web UI views/components.
- **Alerts**: Entire alerts feature removed — REST API (`/api/v1/alerts`), CLI (`lynxdb alerts`), Go client library, persistent store, notification channels (webhook, Slack, Telegram), scheduler, cluster-mode assignment (Raft commands and gRPC RPCs), and the `http.alert_shutdown_timeout` config parameter. **Breaking change**: `meta.CommandType` enum values after `CmdUpdateSourceRegistry` have shifted — clusters with existing Raft state must be re-bootstrapped.

### Added

- **Storage engine**: Columnar segment format (`.lsg` format-major v1 with `LSG1` magic) with delta-varint timestamps, LZ4/ZSTD compression, dictionary-encoded strings, Gorilla-encoded floats, region magics, and a `FORMAT` marker. Existing pre-v1 `.lsg` files are not readable and must be deleted before upgrade.
- **Full-text search**: FST-based inverted index with roaring bitmap posting lists and bloom filters for segment skipping.
- **Direct-to-part ingest**: `AsyncBatcher` buffers events in memory and flushes immutable `.lsg` parts via atomic rename; configurable `fsync` policy per part write.
- **Compaction**: Size-tiered compaction (L0 -> L1 -> L2) with rate limiting.
- **Tiered storage**: Hot (SSD) -> Warm (S3) -> Cold (Glacier) with automatic policy-driven tiering and local segment cache.
- **SPL2 query language**: Full parser with 20+ commands, 15+ aggregation functions, 20+ eval functions, CTEs, and subsearches.
- **Query engine**: Volcano iterator model with 18 streaming operators, stack-based bytecode VM (22ns/op, 0 allocs), and 23-rule optimizer.
- **REST API**: Ingest (JSON/NDJSON/plain text), query (sync/async/streaming), live tail (SSE), field catalog, and management endpoints.
- **Compatibility layer**: Elasticsearch `_bulk` API, OpenTelemetry OTLP/HTTP, and Splunk HEC receivers.
- **Pipe mode**: Query local files and stdin with the full SPL2 engine — no server required.
- **Materialized views**: Precomputed aggregations with automatic backfill, versioned rebuilds, retention policies, and cascading views.
- **Live tail**: Real-time SSE streaming with historical catchup and full SPL2 pipeline support.
- **Field catalog**: Automatic field discovery with types, coverage stats, and top values.
- **CLI**: `server`, `query`, `ingest`, `status`, `mv`, `config`, `bench`, `demo`, and shell completion.
- **Interactive TUI**: Colorized JSON output, progress tracking, and query statistics when stdout is a TTY.
- **Benchmark command**: Built-in `lynxdb bench` for self-testing ingest and query performance.
- **Demo mode**: `lynxdb demo` generates realistic log traffic from nginx, api-gateway, postgres, and redis.
- **Install script**: `curl -fsSL https://lynxdb.org/install.sh | sh` with platform auto-detection and checksum verification.
- **Docker images**: Multi-arch (`amd64`/`arm64`) scratch-based images on Docker Hub.
- **Homebrew tap**: `brew install lynxbase/tap/lynxdb`.
