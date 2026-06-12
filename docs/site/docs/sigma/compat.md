# Sigma compatibility

[Back to Sigma docs](index.md)

## Contract

LynxDB executes LynxFlow v2 only. rsigma v0.9.0's LynxDB target still emits
the legacy SPL2 dialect, so its output is not executed directly. Compatibility
is therefore defined through a curated corpus: for every Sigma construct that
`rsigma convert -t lynxdb` v0.9.0 produces a non-error output for, LynxDB
ships a hand-migrated LynxFlow golden in
`pkg/sigmaqueries/testdata/golden/*.lynxflow` that must:

1. Parse through `pkg/lynxflow/parser` without error.
2. Plan and execute against the deterministic conformance datasets.
3. Match the same set of events that rsigma's own `eval` engine would match on
   identical input, recorded as `expected_match_count` in the embedded
   `pkg/sigmaqueries/compat_manifest.json`.

LynxDB pins rsigma v0.9.0 for v1 of this contract. A future LynxDB release may
extend the supported rsigma tag range, but must not narrow it for the same
contract version.

## Supported construct corpus

Each row pairs the Sigma construct with the curated LynxFlow golden that
covers it (fixture names refer to `pkg/sigmaqueries/testdata/golden/`):

| Sigma construct | Curated LynxFlow golden | Fixture |
|---|---|---|
| `CommandLine: whoami` | `from main \| where CommandLine == "whoami"` | `simple_eq` |
| `contains` / `startswith` / `endswith` modifiers | `from main \| where contains(CommandLine, "whoami") and ends_with(Image, ".exe") and starts_with(ParentImage, "C:\\Windows")` | `wildcards` |
| `(sel and not filter) or extra` | `from main \| where (FieldA == "val1" and not FieldB == "val2") or FieldC == "val3"` | `and_or_not` |
| Selection with negated filter | `from main \| where EventID == 4625 and not SubStatus == "0xC0000064"` | `brute_force` |
| Status range from 400 through 499 | `from main \| where status >= 400 and status < 500` | `numeric_compare` |
| `CommandLine\|re: '.*whoami.*'` | `from main \| where matches(CommandLine, r".*whoami.*")` | `regex` |
| `SourceIP\|cidr: '10.0.0.0/8'` | `from main \| where cidr_match("10.0.0.0/8", SourceIP)` | `cidr` |
| `exists: true` / `field: null` / boolean literal | `from main \| where exists(FieldA) and not exists(FieldB) and Enabled == true` | `exists_null_bool` |
| `keywords: [error, timeout, refused]` | `from main \| where has(_raw, "error") or has(_raw, "timeout") or has(_raw, "refused")` | `keywords` |
| logsource mapped to a custom index | `from security_logs \| where CommandLine == "whoami"` | `simple_eq_index` |

Each canonical fixture also has a `*_minimal` variant covering rsigma's
`format=minimal` output, migrated to the same LynxFlow predicate.

## Validating the corpus

The corpus is conformance-tested in CI and can be checked locally from the
LynxDB repository:

```bash
# Parse + execute the canonical fixtures against deterministic datasets,
# asserting expected_match_count from the embedded compat manifest:
go test -run 'TestParseLynxFlowGoldens|TestLynxFlowConformance' ./pkg/sigmaqueries/...

# Parse-check every .lynxflow golden with the LynxFlow parser:
scripts/check_rsigma_golden_parses.sh
```

`lynxdb query --queries-file` and `lynxdb saved import` accept any LynxFlow
query file, including hand-migrated rule sets. They do not call rsigma and do
not accept the legacy SPL2 dialect.

## Known limitations

The compatibility contract only covers Sigma constructs that rsigma converts
without error for the LynxDB backend. Unsupported Sigma constructs remain
upstream rsigma errors, and rsigma's missing LynxFlow output format means
every converted rule needs the hand-migration step. See
[limitations](limitations.md) for the current list.

## Provenance header

Clients that submit Sigma-derived queries over REST may send:

```http
Sigma-Source: rsigma/0.9.0
```

The header is informational. LynxDB records it in request logs and does not
change query parsing, planning, or execution behavior.

## Embedded manifest

Every LynxDB release carries `pkg/sigmaqueries/compat_manifest.json` as a
release artifact and embeds the same manifest into the binary.

The manifest still contains a legacy `spl2` field per fixture. It records the
rsigma v0.9.0 output that each `expected_match_count` was derived from and is
retained only for that bookkeeping; LynxDB does not parse or execute it.

Print the embedded summary:

```bash
lynxdb sigma compat-check
```

Check a specific rsigma version:

```bash
lynxdb sigma compat-check --rsigma-version 0.9.0
```

Export the full manifest:

```bash
lynxdb sigma compat-check --json
```

## Drift policy

Nightly drift checks sync the Sigma rule sources (`*.yml`) and `manifest.json`
from the pinned rsigma tag and diff them against LynxDB's committed corpus;
the hand-migrated `.lynxflow` goldens are validated by the conformance tests
on every run. Triage follows the [drift runbook](drift-runbook.md): bump the
supported rsigma range and re-sync, update the hand-migrated goldens, or
document the shape as unsupported.
