# Contributing

Thanks for helping improve agsy. A few project-specific rules keep changes
easy to review:

## Ground rules

- **Zero external dependencies.** Everything builds from the Go standard
  library plus the vendored YAML parser in `internal/yaml/` (leave that
  directory untouched — it tracks upstream go-yaml). Do not add module
  dependencies.
- **Every user-facing string goes through `i18n.T(...)`** and needs a matching
  entry in `i18n/locales/zh-TW.json`. The i18n tests enforce this in both
  directions (missing and orphan keys both fail), including placeholder
  counts.
- **Silent behavior is a bug.** Files skipped, changes discarded, links left
  behind — everything gets reported. Follow the existing pattern.
- **Vendor facts live in one table.** Hook event names, supported handler
  types and registry shapes per tool are in `internal/build/hooks.go`,
  registry file names in `internal/config`; change them together with the
  docs' `hook.yaml` section and the golden tests.
- **Destructive operations need guards.** Anything that deletes or overwrites
  must validate its paths (see `validateOut` / `RemoveOut`) and be covered by
  tests.

## Before opening a PR

```sh
go build ./...
go vet ./...
gofmt -l . | grep -v '^internal/yaml/'   # must print nothing
go test ./...
```

READMEs are generated — edit `docs/en/` and `docs/zh-TW/` instead and run
`go run ./scripts/genreadme`. Documentation changes should update both
languages.
