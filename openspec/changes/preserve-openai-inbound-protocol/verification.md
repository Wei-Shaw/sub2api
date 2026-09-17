## Verification

- `go test -tags=unit ./internal/pkg/openai_compat ./internal/service`
- `pnpm exec vitest run src/components/account/__tests__/BulkEditAccountModal.spec.ts src/components/account/__tests__/EditAccountModal.spec.ts`
- `pnpm exec vitest run src/i18n/__tests__/localeKeyCompleteness.spec.ts`
- `pnpm exec vue-tsc -b`
- `pnpm exec vite build`

All commands passed locally with Go 1.27 selected by `GOTOOLCHAIN=auto`, Node 24, and pnpm 9.15.9. Production deployment is outside this change and was not performed.
