# Gotchas

- `deploy/docker-compose.token2.yml` requires `TOKEN2_NETWORK`; the production `.env` does not define it. The live external network is `deploy_sub2api-network`; pass it through the deployment process environment rather than writing it into the secret-bearing `.env`.
- The token2 Compose service uses a fixed `container_name`. With the installed Compose v5.3.1, `up --force-recreate` can hit a name conflict instead of removing the old container. For an approved app-only rollout, capture the old container fingerprint, remove only `sub2api-token2`, then run `up -d --no-deps`; never use `--remove-orphans` because the project reports stateful containers as orphans.
- `InviteLogin`/`RedeemLogin` handlers are not sufficient by themselves: both public routes must be registered in `backend/internal/server/routes/auth.go` and protected by the auth rate limiter.
- The host profile does not expose `pnpm` or `corepack`; use the Dockerfile build or an ephemeral Node/Corepack verification container with a separate dependency volume instead of installing package-manager files into the worktree.
