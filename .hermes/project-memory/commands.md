# Commands

## Backend
- Build: `cd backend && go build ./...`
- Targeted tests: `cd backend && go test ./internal/handler ./internal/repository ./cmd/server`

## Frontend
- Build: `pnpm run build` (also runs `vue-tsc -b`); host profile may lack pnpm, so the Dockerfile build is the canonical fallback.
- Targeted tests/typecheck: `pnpm exec vitest run src/i18n/__tests__/localeRegistry.spec.ts src/i18n/__tests__/localesMessageCompile.spec.ts` and `pnpm exec vue-tsc --noEmit`.

## Docker production deploy
- GitHub Actions workflow: `.github/workflows/docker-build-deploy.yml`; pushes `main` build `ghcr.io/${{ github.repository_owner }}/sub2api:sha-${{ github.sha }}` then deploys over SSH to the production VM's `${SSH_APP_DIR:-/home/ubuntu/apps/sub2api}/deploy` and verifies `/health`.
- Production `docker-compose.prod.yml` and `.env` are VM-side files; they are intentionally not tracked in this minimal source checkout.

## Docker token2
- Render: export `IMAGE_REPO`, `IMAGE_TAG`, and `TOKEN2_NETWORK=deploy_sub2api-network`, then run `docker compose --env-file /home/ubuntu/apps/sub2api/deploy/.env -f docker-compose.token2.yml config --quiet` from `/home/ubuntu/apps/sub2api/deploy`.
- App-only rollout: `docker compose ... up -d --no-deps sub2api-token2` after the fixed-name container replacement procedure documented in `gotchas.md`.

## Last validated
- 2026-08-21: backend build and targeted tests passed; token2 image build/render and health/public auth route smoke passed.
