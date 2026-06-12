# Sigma cookbook

[Back to Sigma docs](index.md)

The recipes below run hand-migrated LynxFlow queries. rsigma v0.9.0's
`lynxdb` target still emits the legacy SPL2 dialect, which LynxDB does not
execute, so each rule is converted once and migrated by hand (see the
[legacy SPL2 mapping](spl2-mapping.md)). The recipes share this query file:

```bash
# rsigma convert -t lynxdb whoami.yml emits legacy SPL2 such as
#   FROM main | search CommandLine=*"whoami"*
# Hand-migrate it to LynxFlow once:
cat > whoami.lynxflow <<'EOF'
from main | where contains(CommandLine, "whoami")
EOF
```

## Send matches to syslog

Run a migrated rule on a schedule and forward matches:

```bash
lynxdb query "$(cat whoami.lynxflow)" --since 5m --format ndjson \
  | xargs -I{} logger -t lynxdb-sigma '{}'
```

## Send matches to Slack

This example posts each match to a Slack incoming webhook endpoint:

```bash
lynxdb query "$(cat whoami.lynxflow)" --since 5m --format ndjson \
  | xargs -I{} curl -sS -X POST "$SLACK_WEBHOOK_URL" \
      -H 'content-type: application/json' \
      -d '{"text":"LynxDB Sigma match: {}"}'
```

## Send matches to PagerDuty

This example posts each match to the PagerDuty Events API:

```bash
lynxdb query "$(cat whoami.lynxflow)" --since 5m --format ndjson \
  | xargs -I{} curl -sS -X POST https://events.pagerduty.com/v2/enqueue \
      -H 'content-type: application/json' \
      -d '{"routing_key":"00000000000000000000000000000000","event_action":"trigger","payload":{"summary":"LynxDB Sigma match","source":"lynxdb","severity":"warning","custom_details":{}}}'
```

## Embed a rule predicate in a larger pipeline

rsigma's `format=minimal` output omits the `FROM main | search` prefix, which
makes the remaining predicate easy to migrate and embed:

```bash
# Legacy SPL2 predicate emitted by rsigma (reference only, not executable):
rsigma convert -t lynxdb -f minimal whoami.yml
#   CommandLine=*"whoami"*

# Hand-migrate the predicate and place it in a where stage of your pipeline:
lynxdb query 'from security | where contains(CommandLine, "whoami") | stats count() by user'
```

Measure this query against your own data before adding alerts or budgets.

## GitHub Action for scheduled rule runs

Because rsigma cannot emit LynxFlow yet, CI cannot convert rules on the fly.
Commit the hand-migrated `.lynxflow` queries next to the rule sources — the
same layout LynxDB uses for its own corpus in
`pkg/sigmaqueries/testdata/golden/` — and run the committed query file:

```yaml
name: sigma-to-lynxdb
on:
  push:
    paths:
      - "rules/**/*.yml"
      - "rules/**/*.lynxflow"

jobs:
  run-detections:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - run: lynxdb query --queries-file rules/all.lynxflow --since 15m --format ndjson
        env:
          LYNXDB_SERVER: http://localhost:3100
```

When a rule source changes, regenerate the legacy SPL2 reference with
`rsigma convert -t lynxdb`, re-migrate the affected line in `all.lynxflow`,
and commit both files together.
