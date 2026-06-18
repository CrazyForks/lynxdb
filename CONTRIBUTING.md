# Contributing to LynxDB

## Setup

LynxDB pins every local and CI toolchain in `mise.toml`: Go, Node, Python,
Rust, Bun, GoReleaser, golangci-lint, rclone, GitHub CLI, and `rsigma`.

```bash
git clone https://github.com/lynxbase/lynxdb.git
cd lynxdb
curl https://mise.run | sh
mise install
mise run build
mise run test
```

If you already have the required toolchains installed, the plain `make` targets
are equivalent:

```bash
make build
make test
make vet
```

For quick local Go-only checks:

```bash
go build -o lynxdb ./cmd/lynxdb/
go test ./...
```

## Branches

Branch off `main`. Name format: `<type>/<short-description>`.

| Prefix    | When to use                          |
|-----------|--------------------------------------|
| `feat/`   | New feature                          |
| `fix/`    | Bug fix                              |
| `chore/`  | Build, CI, deps, cleanup             |
| `refactor/` | Code change that doesn't fix a bug or add a feature |
| `docs/`   | Documentation only                   |
| `test/`   | Adding or fixing tests               |
| `perf/`   | Performance improvement              |

Examples: `feat/streaming-join`, `fix/wal-replay-crash`, `chore/bump-go-version`.

## Commits

Follow the same prefixes for commit messages:

```
feat: add JOIN command to pipeline
fix: batcher flush recovers gracefully from interrupted part writes
chore: update roaring bitmap to v2.4
```

One logical change per commit. Keep commits small and reviewable.

## Pull Requests

1. One PR = one logical change.
2. PR title follows the same `type: description` format.
3. All checks must pass: `make build`, `make test`, `make vet`.
4. Add tests for new code. No exceptions.
5. If you're adding a new LynxFlow operator or query-language behavior, add parser,
   planner, runtime, and CLI coverage as appropriate.

## Code Style

- `go vet ./...` must pass.
- `golangci-lint` via `make lint` if configured.
- Exported symbols require godoc comments.
- Errors are wrapped with context: `fmt.Errorf("component.Op: %w", err)`.
- `context.Context` is the first parameter for any I/O or blocking function.
- No `init()`, no global mutable state, no `panic` for control flow.

## Testing

```bash
make test          # all tests
make test-unit     # Go unit tests with the race detector
make test-e2e      # end-to-end server harness
make test-cli      # CLI integration tests
go test ./pkg/lynxflow/parser -run TestParse   # specific package/test
go test ./... -bench .                   # benchmarks
```

Test locations:
- Unit tests: next to the code (`*_test.go` in the same package).
- Acceptance tests: `test/acceptance/`.
- Integration tests: `test/integration/`.
- E2E tests: `test/e2e/`.
- Regression tests: `test/regression/`.

## Common Tasks

```bash
mise run build          # make build
mise run test           # make test
mise run lint           # make lint
mise run web            # build embedded Web UI
mise run bench          # macro benchmark suite
mise run sync-rsigma    # refresh pinned rsigma golden corpus
make docs-gen           # regenerate registry-driven LynxFlow docs
make clean              # remove local build artifacts
```

## Project Structure

Code lives in three top-level directories:

- `cmd/lynxdb/` -- CLI entry point.
- `pkg/` -- Public packages (storage, query engine, API, LynxFlow parser, etc.).
- `internal/` -- Internal packages not intended for external use.

## Documentation

- User-facing docs site: `docs/site/` (Docusaurus).
- Changelog: `CHANGELOG.md`

## Reporting Issues

Open a GitHub issue. Include:
- What you did (query, config, input data).
- What you expected.
- What happened instead.
- LynxDB version (`lynxdb --version`).
