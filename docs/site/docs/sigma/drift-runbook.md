# rsigma drift triage

[Back to Sigma docs](index.md)

The nightly drift workflow (`.github/workflows/rsigma-drift.yml`) keeps
LynxDB's pinned rsigma corpus honest. Each run:

1. Runs `scripts/sync_rsigma_golden.sh --output-dir <tmp>`, which clones the
   pinned rsigma tag, syncs the Sigma rule sources (`*.yml`), regenerates
   `manifest.json`, and validates the committed hand-migrated `.lynxflow`
   goldens (parse check plus the `pkg/sigmaqueries` test suite).
2. Runs the conformance tests:
   `go test -run 'TestParseLynxFlowGoldens|TestLynxFlowConformance' ./pkg/sigmaqueries/...`.
3. Diffs the synced `*.yml` and `manifest.json` against the committed corpus
   in `pkg/sigmaqueries/testdata/golden/`. The `.lynxflow` goldens are **not**
   diffed — rsigma has no LynxFlow output format, so they are hand-maintained
   in-repo and covered by the conformance step instead. Reference match sets
   (`*.matches.json`) are also excluded from the diff.
4. On drift, uploads a `drift.patch` artifact, opens or updates an issue
   labeled `area/sigma-compat`, and optionally notifies Slack.

When it opens an issue, the db-expert owner triages it and records the
decision in the issue thread.

## Inputs

Open the latest workflow run linked from the issue and download the
`drift.patch` artifact. It contains the rule-source and manifest diff between
rsigma's pinned-tag corpus and the committed LynxDB corpus.

The issue arrives labeled `area/sigma-compat`. If the drift affects LynxFlow
parsing, planning, or execution, also tag the owning query-language area.

## Reproduce locally

Regenerate the corpus into a temporary directory and diff it by hand:

```bash
scripts/sync_rsigma_golden.sh --output-dir /tmp/rsigma-drift-out

for f in /tmp/rsigma-drift-out/*.yml /tmp/rsigma-drift-out/manifest.json; do
  diff -u "pkg/sigmaqueries/testdata/golden/$(basename "$f")" "$f"
done
```

Then validate the committed goldens the same way the workflow does:

```bash
scripts/check_rsigma_golden_parses.sh
go test -count=1 -run 'TestParseLynxFlowGoldens|TestLynxFlowConformance' -timeout 120s ./pkg/sigmaqueries/...
```

For the full suite (what the sync script itself runs at the end):

```bash
go test -count=1 -timeout 120s ./pkg/sigmaqueries/...
```

## Decision tree

1. The upstream rule or manifest change is intentional and every affected
   construct is already representable in the curated LynxFlow corpus: bump the
   pinned rsigma version (below) and commit the synced corpus.
2. The drift introduces or changes a construct whose hand-migrated `.lynxflow`
   golden is missing or no longer faithful: update the golden by hand, using
   the [legacy SPL2 mapping](spl2-mapping.md), and rerun the validation
   commands above. If LynxFlow cannot express the construct, file a
   query-language issue and keep the drift issue open.
3. rsigma emits a shape LynxDB does not intend to support yet: document it in
   [limitations](limitations.md) and close the drift issue with that link.

## Bump the pinned rsigma

The pin lives in two places: `mise.toml` (the rsigma CLI version) and the
`rsigma_ref` default inside `scripts/sync_rsigma_golden.sh`. The script's
`--rsigma-ref` flag is intentionally ignored (it prints a warning), so update
both files first, then sync the committed corpus in place:

```bash
scripts/sync_rsigma_golden.sh
```

Without `--output-dir`, the script writes the rule sources and manifest into
`pkg/sigmaqueries/testdata/golden/` and finishes by running the parse check
and the full `pkg/sigmaqueries` test suite. If a rule changed, update its
hand-migrated `.lynxflow` golden in the same change.

If the deterministic datasets or expected matches changed, regenerate the
reference match sets with the local reference evaluator:

```bash
scripts/sync_rsigma_golden.sh --with-matches
```

Commit the corpus diff and update [compat](compat.md) with the new supported
range. Keep the `drift.patch` artifact attached to the issue for review
history.

## Update or extend the LynxFlow goldens

When a golden needs LynxFlow work, file a separate issue that includes:

- The failing Sigma rule (`*.yml`).
- The legacy SPL2 that rsigma emits for it (reference only — LynxDB does not
  execute it).
- The proposed or failing hand-migrated LynxFlow query.
- The parse, plan, or conformance error from
  `go test ./pkg/sigmaqueries/...` or
  `scripts/check_rsigma_golden_parses.sh`.
- A link to the drift issue and `drift.patch` artifact.

Do not widen the compatibility range until the goldens parse and the
conformance tests pass.

## Document unsupported output

Update [limitations](limitations.md) with the unsupported shape and link to
the upstream rsigma issue if one exists. Close the drift issue only after the
docs change lands.
