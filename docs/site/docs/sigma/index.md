# Sigma rules on LynxDB

Sigma is a rule format for security detections. LynxDB runs Sigma detections
as [LynxFlow](../lynxflow/functions.md) queries — LynxFlow v2 is LynxDB's only
query language since it replaced SPL2. rsigma is an external CLI that can
compile Sigma YAML for the LynxDB backend; LynxDB does not ship rsigma or parse
Sigma YAML itself.

:::warning rsigma output is not directly executable today

The pinned rsigma version (v0.9.0, see `mise.toml`) has no LynxFlow output
format. `rsigma convert -t lynxdb` still emits the **legacy SPL2 dialect**,
which LynxDB no longer parses or executes. Piping rsigma output straight into
`lynxdb query` fails until rsigma ships a LynxFlow emitter. Hand-migrate the
converted output to LynxFlow first — the
[legacy SPL2 mapping](spl2-mapping.md) covers every construct rsigma emits.

:::

The working flow today:

```bash
cargo install rsigma

# 1. Convert. rsigma emits the legacy SPL2 dialect (reference only):
rsigma convert -t lynxdb rule.yml
#   FROM main | search CommandLine="whoami"     <- legacy SPL2, not executable

# 2. Hand-migrate the output to LynxFlow (see the legacy SPL2 mapping):
echo 'from main | where CommandLine == "whoami"' > rule.lynxflow

# 3. Execute the LynxFlow query:
lynxdb query "$(cat rule.lynxflow)"
```

Against a running server, the unassisted REST path takes the migrated LynxFlow
query:

```bash
QUERY='from main | where CommandLine == "whoami"'
curl -sS http://localhost:3100/api/v1/query \
  -H 'content-type: application/json' \
  -d "{\"query\":$(printf '%s' "$QUERY" | jq -Rs .)}"
```

For larger rule sets, pass a query file with
`lynxdb query --queries-file rules.lynxflow` or import it as saved queries with
`lynxdb saved import rules.lynxflow`.

## Curated LynxFlow corpus

LynxDB ships a hand-migrated, conformance-tested LynxFlow corpus covering the
Sigma constructs that rsigma's LynxDB target produces. The fixtures live in the
LynxDB repository at `pkg/sigmaqueries/testdata/golden/*.lynxflow`, for
example:

```
from main | where CommandLine == "whoami"
from main | where cidr_match("10.0.0.0/8", SourceIP)
from main | where has(_raw, "error") or has(_raw, "timeout") or has(_raw, "refused")
```

Use these goldens as migration templates. They are validated by:

```bash
go test -run 'TestParseLynxFlowGoldens|TestLynxFlowConformance' ./pkg/sigmaqueries/...
```

See the [compatibility contract](compat.md) for what the corpus guarantees.

Reference:

- [Compatibility contract](compat.md)
- [Legacy SPL2 mapping (with LynxFlow equivalents)](spl2-mapping.md)
- [Pipelines](pipelines.md)
- [Cookbook](cookbook.md)
- [Troubleshooting](troubleshooting.md)
- [Limitations](limitations.md)
- [Drift runbook](drift-runbook.md)

Tutorials:

- [01: Detect Whoami in 60 seconds](tutorials/01-quickstart.md)
- [02: Bulk conversion](tutorials/02-bulk-conversion.md)
- [03: Windows EVTX](tutorials/03-windows-evtx.md)
- [04: CloudTrail](tutorials/04-cloudtrail.md)
- [05: Pipelines](tutorials/05-pipelines.md)
- [06: Scheduled detection](tutorials/06-scheduled-detection.md)

Next: [tutorial 01](tutorials/01-quickstart.md).
