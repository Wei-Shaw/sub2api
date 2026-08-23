# State

## Current focus
- Vietnamese locale coverage and translation quality are complete on a dedicated branch; local verification is complete, while push/PR, independent review, and CI remain pending.

## Repo
- Branch: `feat/vietnamese-locale-coverage`
- Remote: `https://github.com/cong91/sub2api.git`

## Active changes
- Vietnamese messages are split between `frontend/src/i18n/locales/vi.ts` and `vi-additions-a.ts` through `vi-additions-d.ts`.
- Locale regression tests cover zh-to-vi key coverage, message compilation, and en-to-vi placeholder parity.
- Technical literals such as endpoints, field names, model identifiers, environment variables, and code placeholders are preserved during translation cleanup.

## Verification
- Frontend lint: `npm run lint:check` passed.
- Frontend typecheck: `npm run typecheck` passed.
- Full Vitest suite: `npm run test:run` passed (`243` files / `1706` tests).
- Production build: `npm run build` passed; Vite emitted existing dynamic-import and large-chunk warnings only.
- Locale parity script passed: en/zh each have 7,916 keys; vi has 8,567 keys; missing zh-to-vi keys `0`; placeholder mismatches `0`.
- `git diff --check` passed.

## Release blockers / risks
- No push, PR, or production deployment has occurred for this locale branch.
- Independent delegated review was unavailable because the configured reviewer model returned HTTP 404; self-review and local verification do not replace formal review/CI.

## Beads
- br-missing: install beads_rust; `.beads` not initialized.

## Bootstrap metadata
- mode: smart
- cwd: /home/ubuntu/workspaces/projects/sub2api
