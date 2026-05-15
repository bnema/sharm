# Agent Guide
This file is for coding agents working in `sharm`.
Follow repo conventions first.
Keep changes narrow, concrete, and verifiable.

## Repo Facts
- The project is a Go application using hexagonal architecture.
- The module path is `github.com/bnema/sharm`.
- The Go version in `go.mod` is `1.26.2`.
- Development expects Go 1.26.2+, FFmpeg, `sqlc`, `templ`, and `mockery`.
- The current repo-local agent rules are this file and the README.
- There is no `.cursor/rules/` directory in this repo.
- There is no `.cursorrules` file in this repo.
- There is no `.github/copilot-instructions.md` file in this repo.
- If those files are added later, treat them as additional instructions.
- Start with the Makefile before inventing custom workflows.
- Use repo targets when they already cover the task.
- `make generate` requires `sqlc`, `templ`, and `mockery`.
- `make dev` expects `air` for hot reload.
- CI runs tests with race detection.
- CI lint timeout is 5 minutes.

## Architecture And Boundaries
- Respect hexagonal architecture at all times.
- `internal/domain` holds core types, enums, and domain errors.
- `internal/port` holds interfaces across boundaries.
- `internal/service` holds business logic and depends on ports.
- `internal/adapter/...` holds delivery and infrastructure implementations.
- `cmd/sharm` is the composition root that wires real implementations.
- Dependencies point inward.
- Domain and ports must not import adapters.
- Services should depend on `internal/port` interfaces or narrow local interfaces.
- Adapters may depend on services, ports, and domain types as needed.
- Prefer interfaces at real boundaries.
- Do not add speculative interfaces with only one consumer and no boundary need.
- Follow existing package responsibilities instead of inventing new layers.

## Core Commands
- Setup dependencies: `make deps`
- Generate code: `make generate`
- Start dev server: `make dev`
- Build binary: `make build`
- Run built app: `make run`
- Format Go code: `make fmt`
- Run linters: `make lint`
- Run `go vet` only: `make vet`
- Run full test suite: `make test`
- Run short tests: `make test-short`
- Run race tests: `make test-race`
- Run coverage: `make test-coverage`
- Run benchmarks via Make: `make benchmark`
- Run fmt + vet + test: `make check`
- Run fmt + vet + race tests: `make ci`

## Focused Test Commands
- Test one package: `go test ./internal/service`
- Test one package with verbose output: `go test -v ./internal/service`
- Test one function: `go test ./internal/service -run '^TestAuthService_ValidatePassword$'`
- Test one package with race detector: `go test -race ./internal/service`
- Run benchmarks directly: `go test ./... -bench=. -benchmem`
- During iteration, prefer focused `go test` commands.
- Before handing work back, expand to broader validation.

## When To Run What
- After each major milestone, run at least `make lint` and `make test`.
- Prefer `make check` before you consider work ready to hand back.
- If you touched concurrency, worker pools, storage, or HTTP flows, run `make test-race`.
- If you changed generated sources or templates, run `make generate` before tests.
- If you only changed a small unit, use focused `go test` while iterating.
- If a command cannot run in the environment, say so clearly and why.

## Generated Code And Codegen
- Never edit generated files by hand.
- This is a hard repo rule, especially for mocks.
- Generated mocks live in `internal/port/mocks/`.
- Generated sqlc output lives in `internal/adapter/storage/sqlite/sqlitedb/`.
- Generated templ output lives in files matching `*_templ.go`.
- Regenerate all of these with `make generate`.
- Mock generation is configured in `.mockery.yml`.
- Mockery uses the `goimports` formatter.
- Edit `.templ` source files, not generated `_templ.go` files.
- Put SQL changes in `queries/` and schema changes in `migrations/`.
- After SQL or schema changes, run `make generate`.
- Do not hand-edit generated code to make tests pass.

## Code Style
### Imports And Formatting
- Let `gofmt` and `goimports` manage formatting and imports.
- Keep standard library imports first, then a blank line, then external and internal imports.
- Do not manually align or sort imports in a custom style.
- Keep lines under the repo limit of 140 characters.
- `nolint` comments must be specific and justified.
### Packages, Types, And Naming
- Keep package names short, lowercase, and purpose-driven.
- Use `NewXxx` for constructors.
- Prefer strong domain types over loose strings and ints for important concepts.
- Reuse existing domain enums and sentinel errors when possible.
- Name methods with direct verbs like `Get`, `ListAll`, `Delete`, `Upload`, or `Cleanup`.
- Keep exported names simple and internal helpers local.
### Errors
- Wrap propagated errors with `%w` and useful context.
- Translate infrastructure errors to domain-level errors at boundaries when appropriate.
- Return domain errors from services when callers need stable behavior.
- Avoid swallowing errors unless the code is explicitly best-effort cleanup.
- If you ignore a cleanup error, do it explicitly with `_ = ...` or a justified `//nolint:errcheck`.
### Logging
- Use the shared logger package instead of ad hoc logging styles.
- Log operational context at boundaries.
- Sanitize user-controlled values before logging them.
- Keep user-facing error messages simpler than internal logs when needed.
### HTTP And Adapter Conventions
- Preserve the Go 1.22+ `http.ServeMux` route style using `"METHOD /path"` patterns.
- Follow existing middleware patterns for auth, CSRF, security headers, rate limiting, and backoff.
- Keep HTTP handlers and storage adapters on the adapter side of the boundary.
- Do not move business rules into adapters when they belong in services or domain code.

## Testing Conventions
- Same-package tests are the norm in this repo.
- Prefer table-driven tests with `t.Run(...)` for variations.
- Use `require` for setup and preconditions.
- Use `assert` for result checks and detailed expectations.
- Use `httptest` for HTTP handler tests.
- Use `t.TempDir()` and temp files for filesystem work.
- Prefer mockery-generated mocks for `internal/port` interfaces.
- Use the testify mock `EXPECT()` style already used in the repo.
- Small hand-written fakes are acceptable for narrow local interfaces in tests.
- Keep tests close to the package they verify.

## Security And Operational Notes
- Respect existing security headers, CSRF protection, upload validation, and filename sanitization.
- Do not bypass auth, rate limiting, backoff, or security middleware without a clear reason.
- Preserve existing patterns for sanitizing user-controlled data.
- Keep configuration behavior compatible with the README unless the task explicitly changes it.
- `SECRET_KEY` may be auto-generated and persisted to `$DATA_DIR/.secret_key` if unset.

## Do Not
- Do not break hexagonal boundaries.
- Do not edit generated mocks, generated sqlc code, or generated templ output.
- Do not invent new tooling flows when the Makefile already covers the task.
- Do not silently skip `make lint` and `make test` after a major milestone.
- Do not bypass repo validation just because focused tests passed.

## Useful Paths
- `README.md`
- `Makefile`
- `go.mod`
- `.golangci.yml`
- `.mockery.yml`
- `cmd/sharm/`
- `internal/domain/`
- `internal/port/`
- `internal/service/`
- `internal/adapter/http/`
- `internal/adapter/storage/sqlite/`
- `queries/`
- `migrations/`
- `docs/`
