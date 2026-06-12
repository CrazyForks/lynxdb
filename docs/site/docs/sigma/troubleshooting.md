# Sigma troubleshooting

[Back to Sigma docs](index.md)

| Symptom | Diagnosis | Fix |
|---|---|---|
| `rsigma output fails to parse in lynxdb query` | rsigma v0.9.0 emits the legacy SPL2 dialect (`FROM main \| search ...`); LynxDB executes LynxFlow v2 only. | Hand-migrate the output using the [legacy SPL2 mapping](spl2-mapping.md), e.g. `from main \| where CommandLine == "whoami"`. The direct `rsigma \| lynxdb` pipe stays broken until rsigma ships a LynxFlow output format. |
| `My regex rule is slow` | The migrated query uses `matches(field, r"pattern")`, which may require scanning candidate rows. | Prefer rules that migrate to `has`, `contains`, `starts_with`, or `ends_with` when possible. For `_raw` regex searches, turn on the inverted index for `_raw` so literal extraction can reduce scans. |
| `rsigma says rule X is unsupported` | rsigma could not convert the Sigma construct for the LynxDB backend. | Check the upstream rsigma issue tracker and file a minimal rule if one does not exist: [rsigma issues](https://github.com/timescale/rsigma/issues). |
| `My index isn't main` | rsigma defaults to `FROM main` unless a pipeline sets another index, and the migrated query inherits that source. | Add a pipeline with `set_state index=security`, then migrate to `from security \| where ...`; see [pipelines](pipelines.md). |
| A migrated query returns no rows | Field names in the Sigma rule do not match the ingested event shape. | Use an rsigma field-mapping pipeline before conversion, or adjust ingestion so fields match the rule pack. |
| A CIDR rule misses IPv6 events | IPv6 CIDR edge cases are listed as a current limitation of `cidr_match` coverage. | Track the limitation in [limitations](limitations.md) and keep a rule-specific regression case when support changes. |

The first check is always to inspect the legacy SPL2 that rsigma emitted, as
the reference for what the rule means:

```bash
rsigma convert -t lynxdb rule.yml   # legacy SPL2, reference only
```

Then confirm the hand-migrated LynxFlow query expresses the same predicate and
run it directly through LynxDB:

```bash
lynxdb query 'from main | where contains(CommandLine, "whoami")'
```

To smoke-test that a migrated query parses without touching real data, run it
against an empty input:

```bash
lynxdb query --file /dev/null 'from main | where contains(CommandLine, "whoami")'
```

The conformance-tested goldens in `pkg/sigmaqueries/testdata/golden/` are the
reference migrations for every supported construct.
