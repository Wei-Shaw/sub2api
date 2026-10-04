# Shared CI pool onboarding

This repository's validation workflow uses the organization-wide CI pool
router described in the shared onboarding guide. The workflow keeps the
deployment-script checks on `macos-15`; the Linux test, frontend, lint, and
release-helper jobs use the runner selected by the `route` job.

The existing `security-scan.yml` remains GitHub-hosted, and `release.yml`
retains its production release workflow, concurrency, and credential boundary.

## Server allowlist

The CI administrator must add this exact entry to
`/etc/peiwan/ci-router.json`, preserving all existing entries and creating a
timestamped backup first:

```json
{
  "ttk9995/sub2api-ttk": {
    ".github/workflows/backend-ci.yml": {
      "test": 1,
      "frontend": 1,
      "golangci-lint": 1,
      "release-helpers": 1
    },
    ".github/workflows/ci-router-smoke.yml": {
      "Router smoke workload": 1
    }
  }
}
```

The keys are workflow paths and job display names, not job IDs from another
project. The shared Action must remain pinned to the accepted commit SHA in
the workflows.

## Repository settings

Configure the dedicated `CI_ROUTER_SSH_KEY` and verified
`CI_ROUTER_KNOWN_HOSTS` secrets, plus the `CI_ROUTER_HOST` variable. Optional
`CI_ROUTER_PORT` and `CI_ROUTER_USER` default to `22` and `ci-router`.

The organization Runner group must allow `ttk9995/sub2api-ttk`. Backend
integration tests use Docker Testcontainers for isolated PostgreSQL and Redis;
the self-hosted Runner account therefore needs the same Docker capability as
the existing CI jobs. The forced-command `ci-router` account does not need
Docker or sudo access.

## Acceptance

Run `ci-router-smoke.yml` while the pool is idle, then overlap it with a smoke
run from another configured repository. Record both Runner names, the run URLs,
the release result, and the completed-workflow cleanup result before enabling
the workflow as a required branch check.
