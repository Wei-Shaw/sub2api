# Upgrading Sub2API

Choose the update method that matches how Sub2API is deployed.

| Deployment | Update method |
| --- | --- |
| Binary installation with a writable executable directory | Admin Dashboard binary update, followed by a supported service restart |
| Docker Compose | Pull the selected application image and recreate the Sub2API service |
| Kubernetes or another multi-replica orchestrator | Change the workload's image reference and use the orchestrator's rollout process |

## What the Admin Dashboard updates

The update endpoint downloads a release archive, verifies its checksum when one is available, and replaces the executable next to its backup file. It operates on the filesystem of the instance that handles the request. A successful response asks for a service restart; the running process has not yet changed versions.

This operation does not pull a container image, change a Compose configuration or Kubernetes Deployment, drain other replicas, or verify their versions. Shared PostgreSQL and Redis do not distribute the downloaded executable. A shared operation lock coordinates update requests, but does not perform a multi-replica rollout.

Some container supervisors support restarting a binary updated through the UI, including the documented Apple-container workflow. Such an update still changes the container's writable layer, not its image. Recreating the container uses the configured image again. An immutable or read-only container may reject the binary replacement entirely.

## Container image updates

Select the intended official release tag or immutable image digest in the existing service configuration. Record the previous image reference before changing it. A moving tag such as `latest` does not preserve the exact artifact needed for recovery.

For the local-directory Compose configuration, with PostgreSQL and Redis already running:

```sh
docker compose -f docker-compose.local.yml pull sub2api
docker compose -f docker-compose.local.yml up -d --no-deps sub2api
```

For the named-volume configuration, omit `-f docker-compose.local.yml`. These commands target only Sub2API; they do not upgrade the database or Redis services.

For Kubernetes, update the image in the workload's source of truth and follow its normal rollout process. Check the actual running version of every replica after replacement. The Admin Dashboard update endpoint is not an image rollout controller.

## Shared state and recovery

Before upgrading, review the release notes and SQL migrations, take a consistent backup of the affected PostgreSQL data and protected configuration, and verify that the backup can be restored separately. Preserve the existing persistent volumes, Redis database, JWT secret and encryption keys. Never publish these values in an image or build configuration.

Replicas belonging to the same installation must use the same authoritative PostgreSQL database and intended Redis database, with consistent authentication configuration. Redis is not a substitute for the PostgreSQL account and billing ledger. Independent installations should keep their state isolated.

Test old and new application versions against the post-migration schema before allowing them to overlap. A rolling deployment can run both versions at once, and a failed release may need the previous application version to continue using the migrated database. Reverting an image does not reverse SQL migrations. Do not overwrite transactions accepted after an upgrade by restoring an older backup as an ordinary application rollback.

Plan request admission and termination around the deployment's actual stream durations and supervisor behavior. Readiness alone does not show that existing SSE or WebSocket requests have drained. Verify authentication, revocation, routing and representative usage/balance changes as well as health endpoints before completing an upgrade.
