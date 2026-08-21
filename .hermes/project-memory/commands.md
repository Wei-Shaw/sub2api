# Commands

## Backend
- Build: `cd backend && go build ./...`
- Targeted tests: `cd backend && go test ./internal/handler ./internal/repository ./cmd/server`

## Docker token2
- Render: export `IMAGE_REPO`, `IMAGE_TAG`, and `TOKEN2_NETWORK=deploy_sub2api-network`, then run `docker compose --env-file /home/ubuntu/apps/sub2api/deploy/.env -f docker-compose.token2.yml config --quiet` from `/home/ubuntu/apps/sub2api/deploy`.
- App-only rollout: `docker compose ... up -d --no-deps sub2api-token2` after the fixed-name container replacement procedure documented in `gotchas.md`.

## Last validated
- 2026-08-21: backend build and targeted tests passed; token2 image build/render and health/public auth route smoke passed.
