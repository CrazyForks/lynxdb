# Sigma limitations

[Back to Sigma docs](index.md)

This page lists known limits of running Sigma detections on LynxDB. LynxDB
executes LynxFlow v2 only; rules that rsigma rejects, and the dialect gap
described below, are tracked here so the rest of the docs can stay concrete.

## Current limits

| Area | Limit | Where to track |
|---|---|---|
| No LynxFlow output in rsigma | rsigma v0.9.0's `lynxdb` target emits the legacy SPL2 dialect, which LynxDB no longer executes. Converted output must be hand-migrated to LynxFlow; the direct `rsigma \| lynxdb` pipe does not work. | [rsigma issues](https://github.com/timescale/rsigma/issues) and the [legacy SPL2 mapping](spl2-mapping.md) |
| Unsupported rsigma rules | Some Sigma constructs may not convert for the LynxDB backend at all. | [rsigma issues](https://github.com/timescale/rsigma/issues) |
| Rare correlation forms | Correlation rules only work when rsigma can lower them for the LynxDB backend and the result is hand-migrated to LynxFlow. | [rsigma issues](https://github.com/timescale/rsigma/issues) |
| IPv6 CIDR edge cases | IPv4 CIDR is covered by `cidr_match`; IPv6 edge cases need rule-specific validation before being called supported. | LynxDB issue tracker and rsigma issue tracker |
| Field naming | Sigma packs assume a schema such as ECS, OCSF, or Windows event fields. LynxDB does not rename fields unless the query tells it to. | [Pipelines](pipelines.md) |
| Helper commands | `lynxdb query --queries-file` and `lynxdb saved import` consume LynxFlow query files only; they do not convert Sigma YAML or the legacy SPL2 dialect. | Hand-migrate rsigma output before calling LynxDB helpers. |

## What LynxDB does not provide

LynxDB does not provide a Sigma rule editor, rule scheduler, or alerting
system. Use cron, GitHub Actions, Airflow, or another runner to execute the
LynxFlow queries. See [tutorial 06](tutorials/06-scheduled-detection.md).

LynxDB does not vendor or run rsigma. Install rsigma separately, convert the
rule, and hand-migrate the generated legacy SPL2 to LynxFlow before passing it
to LynxDB. The curated migrations in `pkg/sigmaqueries/testdata/golden/` cover
every supported construct.
