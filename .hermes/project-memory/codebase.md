# Codebase

## Key paths detected
- `backend`
- `frontend`
- `docs`
- `.github/workflows`
- `Dockerfile`

## Entry points
- TBD after source inspection

## Where to add things
- Frontend locale source modules live under `frontend/src/i18n/locales/{zh,en}`; `zh/index.ts` is the parity source for the complete message tree.
- Vietnamese base messages remain in `frontend/src/i18n/locales/vi.ts`; incremental domain additions are split across `vi-additions-a.ts` through `vi-additions-d.ts` and deep-merged by `vi.ts`.
- Locale regression checks belong under `frontend/src/i18n/__tests__`; `localeKeyParity.spec.ts` guards zh→vi key coverage and en→vi placeholder parity.
