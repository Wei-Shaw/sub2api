# Issue #8 repository gate record

This file is the tracked record format for repository-wide verification. Record results only after running each command against the code-and-assets commit from a clean worktree. The evidence-only commit that fills in this record may follow without changing the verified code or runner. Keep raw command output under `output/issue-8/` and link its relative path in the evidence column.

## Run metadata

- Verified code/assets commit: `32cd1e123ab17c64a1e2e98ce0053d498b1ff445`
- Worktree state: clean detached linked worktree; `git status --short` was empty before and after all gates
- Started UTC: `2026-07-12T11:06:12Z`
- Finished UTC: `2026-07-12T11:29:51Z`
- Operator/environment: Codex on Windows; PowerShell `7.6.1`; Node.js `24.15.0`; Docker Desktop server `29.5.3`; Go images `s2a-go-integration:1.26.5` and `s2a-issue8-race:go1.26.5`

Allowed statuses are `PASS`, `FAIL`, and `NOT RUN`. Do not mark a gate `PASS` from an earlier code/assets commit or a dirty-worktree run.

| Gate | Status | Command or scope | Evidence |
| --- | --- | --- | --- |
| Issue #8 runner self-test | PASS | `verification/issue-8/run.tests.ps1` | `output/issue-8/runner-self-test.txt` (8 scenarios passed) |
| Issue #8 Full acceptance | PASS | `verification/issue-8/run.ps1 -Mode Full` | `output/issue-8/summary.md`, `output/issue-8/benchmark.txt`, `output/issue-8/contention.txt` |
| Issue #8 Docker cleanup | PASS | `resource_cleanup.passed` and all `resource_cleanup.remaining` arrays are empty | `output/issue-8/run-status.json`; run `5c2eeb37d11c4f2188ca0de97c19b1cd` |
| Post-suite Docker resource audit | PASS | Remove only run-labeled/test-owned resources; verify pre-existing running container IDs remain running and healthy | `output/issue-8/docker-final-state.txt`; SHA-256 `AC3C5A90F820BCC3DCC9746722446026ABFB5C78B4E1DDC20C575FF416B9A901` |
| Backend unit tests | PASS | `go test -tags=unit ./...` | `output/issue-8/backend-unit.txt` |
| Backend integration tests | PASS | `SUB2API_TEST_RUN_ID=<unique> go test -tags=integration ./...` | `output/issue-8/backend-integration.txt`; run `de1b9b8ffd5c4d43a276cf30fa92f726` |
| Backend vet | PASS | `go vet ./...` | `output/issue-8/backend-vet.txt` |
| Wire generation drift | PASS | `go generate ./cmd/server` then `git diff --exit-code -- backend/cmd/server/wire_gen.go` | `output/issue-8/backend-wire-generate.txt`, `output/issue-8/backend-wire-diff.txt` |
| Issue #8 integration race | PASS | `SUB2API_TEST_RUN_ID=<same unique run> go test -race -tags=integration ./internal/repository -run '^TestIssue8GatewayAdmission' -count=1` using `Dockerfile.race` | `output/issue-8/backend-race.txt`; run `de1b9b8ffd5c4d43a276cf30fa92f726` |
| Frontend typecheck | PASS | `vue-tsc --noEmit` | `output/issue-8/frontend-typecheck.txt` |
| Frontend lint | PASS | `eslint` | `output/issue-8/frontend-lint.txt` |
| Frontend tests | PASS | `vitest run` | `output/issue-8/frontend-tests.txt` (147 files, 995 tests) |
| Frontend production build | PASS | `vue-tsc -b` and `vite build` | `output/issue-8/frontend-build-typecheck.txt`, `output/issue-8/frontend-build.txt` |

## Failures and follow-up

The first sandboxed Vitest launch could not spawn esbuild (`EPERM`). Re-running the same command with process-launch permission produced the recorded PASS result: 147 files and 995 tests. No product-code failure was observed on the verified commit.
