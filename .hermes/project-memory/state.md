# State

## Current focus
- Vietnamese locale content is complete and has been reorganized into the same module-folder layout used by English and Chinese.
- Local verification, commit, push, and fresh CI/Security checks for the folder refactor are green on the current PR head.

## Repo
- Branch: `feat/vietnamese-locale-coverage-followup`
- Pull request: `https://github.com/cong91/sub2api/pull/176`
- Base: `main`

## Active changes
- `frontend/src/i18n/locales/vi/` now mirrors the `en/` and `zh/` topology: root locale modules plus `admin/` modules and index aggregators.
- The former root-level `vi.ts` and `vi-additions-a.ts` through `vi-additions-d.ts` are removed.
- `vi/merge.ts` preserves the previous ordered deep-merge behavior for keys that originated in additions files.
- No translation value, key path, placeholder, component behavior, or backend code is changed by the structural refactor.

## Verification
- Runtime equivalence against `HEAD` passed: old/new both contain `8,709` leaf keys; missing `0`; extra `0`; changed values `0`.
- Focused i18n contract suite passed: `npm run test:run -- src/i18n/__tests__/localeKeyParity.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts src/i18n/__tests__/localeRegistry.spec.ts src/i18n/__tests__/localesNoKeyCollision.spec.ts` (`4` files / `13` tests).
- Frontend lint: `npm run lint:check` passed.
- Frontend typecheck: `npm run typecheck` passed.
- Full frontend suite: `npm run test:run` passed (`255` files / `1,795` tests).
- Production build: `npm run build` passed; Vite emitted only the existing large-chunk warning.
- `git diff --check` passed.

## Model Plaza triage
- Both previous failures were test regressions introduced by commit `2838efa81` in `PlazaModelPricingTable.spec.ts`; the component contract was not changed.
- The restored targeted suite is included in the green full frontend run (`PlazaModelPricingTable.spec.ts`: `25` tests).

## Release blockers / risks
- CI and Security Scan are green on the current PR head; latest check-runs have no annotations.
- Independent review remains unavailable; local verification and self-review do not replace formal approval.
- No merge, deployment, release, or production-impacting action is authorized by this state.

## Beads
- `br`/beads bootstrap is unavailable in this environment; `.beads` is not initialized.

## Bootstrap metadata
- mode: smart
- cwd: /home/ubuntu/workspaces/projects/sub2api
