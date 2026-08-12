# Fork Release Sync

This fork follows stable releases from `Wei-Shaw/sub2api` while preserving the
small set of custom commits on `main`.

## How it works

`.github/workflows/sync-upstream-release.yml` runs every six hours and can also
be started manually. It calls `scripts/sync-upstream-release.sh`, which:

1. Reads the latest non-prerelease upstream GitHub Release.
2. Exits when the same fork release already exists.
3. Fetches the official tag into a namespaced ref so upstream and fork tags do
   not collide locally.
4. Merges that official tag into fork `main` in a temporary worktree.
5. Rejects new fork-only migrations outside the reserved `fork_` namespace,
   then runs backend and frontend checks. A conflict or failed check exits without
   changing fork `main` or creating a tag.
6. Atomically pushes fork `main` and the same version tag.
7. Dispatches the existing `release.yml` workflow to build release archives and
   `ghcr.io/dextok/sub2api` images.

If a tag exists but its release build failed, the next run dispatches the
release workflow again. No personal access token or external scheduler is
required.

## One-time repository setup

1. Enable GitHub Actions for `dextok/sub2api`.
2. In **Settings > Actions > General**, confirm repository or organization
   policy allows the workflows' declared `contents: write`, `actions: write`,
   and `packages: write` permissions.
3. Ensure branch rules for `main` allow GitHub Actions to push the automated
   merge commit. Conflicts still fail closed and require a manual resolution.
4. Push this customization commit, then manually run **Sync Upstream Release**
   once to bootstrap the current fork release.
5. After the first image is published, open the `sub2api` package settings and
   make the GHCR package public so production hosts can pull it anonymously.

No repository secrets are needed unless existing release notifications or
Docker Hub publishing are also desired.

## Fork migration policy

Upstream owns numeric migration names such as `221_description.sql`. New
fork-only migrations must instead use
`fork_YYYYMMDDHHMM_description.sql` (UTC timestamp, lowercase description).
This keeps the fork out of upstream's growing numeric namespace and gives fork
migrations deterministic ordering after numeric upstream migrations.

The two previously released fork migrations beginning with `194_` and `195_`
are grandfathered. Never rename or edit them: deployed databases identify a
migration by its complete filename and verify its content checksum. The sync
script runs `scripts/check-fork-migrations.sh` after every upstream merge and
before tests, tagging, or push, so a future namespace violation fails closed.

## Version model

Fork release `vX.Y.Z` means "official `vX.Y.Z` plus the fork commits already on
`main`". The fork intentionally uses the same tag name so the existing semantic
version comparison and rollback list continue to work.

The application update source defaults to `dextok/sub2api` and can be overridden
with `UPDATE_REPOSITORY`. Compose defaults to
`ghcr.io/dextok/sub2api:latest` and can be pinned or overridden with
`SUB2API_IMAGE`.

## Production upgrade

Back up the database and persistent data before an upgrade. For a Compose
deployment, keep the existing `.env`, volumes, bind mounts, PostgreSQL, and Redis
services unchanged; only switch the application image and recreate `sub2api`:

```bash
export SUB2API_IMAGE=ghcr.io/dextok/sub2api:vX.Y.Z
docker compose pull sub2api
docker compose up -d --no-deps sub2api
docker compose ps
docker compose logs --tail=100 sub2api
```

Pinning the release tag during deployment makes rollback deterministic. After
health checks pass, `latest` may be used for later automated pulls if desired.
