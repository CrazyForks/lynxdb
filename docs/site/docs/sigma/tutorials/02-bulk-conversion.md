# Bulk conversion

[Back to Sigma docs](../index.md)

This tutorial converts a SigmaHQ rules checkout and builds a LynxFlow query
file from it. rsigma v0.9.0 emits the legacy SPL2 dialect that LynxDB no
longer executes, so the rsigma output serves as a migration worksheet: the
file you actually run is the hand-migrated `.lynxflow` file you maintain next
to it. (LynxDB maintains its own corpus the same way, in
`pkg/sigmaqueries/testdata/golden/`.)

Clone SigmaHQ rules:

```bash
git clone https://github.com/SigmaHQ/sigma.git sigma
```

Convert the rules into a legacy SPL2 reference file:

```bash
rsigma convert -t lynxdb -r sigma/rules > all.spl2   # legacy SPL2, not executable
```

Inspect the output to see what each rule means:

```bash
head -20 all.spl2
```

Hand-migrate each line to LynxFlow using the
[legacy SPL2 mapping](../spl2-mapping.md), keeping one query per line. For
example, a worksheet line:

```spl
FROM main | search CommandLine="whoami"
```

becomes this line in `all.lynxflow`:

```
from main | where CommandLine == "whoami"
```

Smoke-test that every migrated query parses by running the file against an
empty input — no server or data needed:

```bash
while IFS= read -r q; do
  lynxdb query --file /dev/null "$q" </dev/null >/dev/null
done < all.lynxflow
```

Run one query manually:

```bash
sed -n '1p' all.lynxflow > first.lynxflow
lynxdb query "$(cat first.lynxflow)" --since 24h
```

Import the migrated file as saved queries:

```bash
lynxdb saved import all.lynxflow --update-existing
lynxdb saved
```

If you keep a sidecar manifest for rule metadata, pass it during import:

```bash
lynxdb saved import all.lynxflow --manifest manifest.json --update-existing
```

For one-off runs, skip saved queries and run the file directly:

```bash
lynxdb query --queries-file all.lynxflow --since 24h --format ndjson
```

When the rule pack updates, regenerate `all.spl2`, diff it against the
previous version, and re-migrate only the changed lines.
