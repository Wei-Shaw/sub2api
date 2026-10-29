# AICodex Prompt Audit source freeze manifest

- Frozen at: `2026-07-16 20:21:19 CST (+0800)`
- Source repository: `/Users/mt/code/mt-ai/aicodex/aicodex-api`
- Source branch at capture: `yjb`
- Base commit: `7a50378851a80650cb0c086260b23abeb3469e6b`
- Freeze method: immutable tracked patch plus untracked tar archive
- Restored verification worktree: detached from the base commit, then populated only from the two artifacts below

## Artifacts

| Artifact | Size | SHA-256 |
| --- | ---: | --- |
| `aicodex-prompt-audit-tracked.patch` | 124674 bytes | `f751a13cce3f3a73cd60cae3aececcef6e1e76dcec8c551a7a4747f032234d2b` |
| `aicodex-prompt-audit-untracked.tar.gz` | 39342 bytes | `1536e2781703b7620e26f2d08b249431fa5846ad9e32b2e8b0d547c3fa3b3632` |

The tracked patch contains 38 files with 1306 insertions and 227 deletions. It is applied to the base commit above using `git apply`.

## Untracked archive entries

- `ai-gateway/internal/gatewaycore/prompt_guard.go`
- `ai-gateway/internal/relay/ws_responses_prompt_guard_order_test.go`
- `ai-gateway/internal/router/prompt_guard_order_test.go`
- `ai-gateway/internal/service/promptaudit/outbound_security.go`
- `ai-gateway/internal/service/promptaudit/synchronous_guard.go`
- `ai-gateway/internal/service/promptaudit/synchronous_guard_test.go`
- `openspec/changes/add-prompt-audit-synchronous-blocking/.openspec.yaml`
- `openspec/changes/add-prompt-audit-synchronous-blocking/design.md`
- `openspec/changes/add-prompt-audit-synchronous-blocking/proposal.md`
- `openspec/changes/add-prompt-audit-synchronous-blocking/specs/prompt-input-audit/spec.md`
- `openspec/changes/add-prompt-audit-synchronous-blocking/specs/prompt-input-guard/spec.md`
- `openspec/changes/add-prompt-audit-synchronous-blocking/tasks.md`

## Restore and verification result

The artifacts were restored into `/tmp/aicodex-prompt-audit-freeze-7a503788`, a detached worktree at the base commit. `git diff --check` passed.

The following commands passed against the restored copy:

```text
cd ai-gateway
go test ./internal/service/promptaudit -count=1
ok github.com/mt21625457/aicodex/internal/service/promptaudit 2.081s

go test ./internal/router ./internal/relay ./internal/gatewayadapter/transport \
  -run 'PromptGuard|PromptAudit|ConcurrencyOrder' -count=1
ok github.com/mt21625457/aicodex/internal/router 1.184s
ok github.com/mt21625457/aicodex/internal/relay 2.201s
ok github.com/mt21625457/aicodex/internal/gatewayadapter/transport 3.233s
```

The source worktree remains untouched. The target OpenSpec specs remain authoritative if this frozen implementation differs from the target architecture.
