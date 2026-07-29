# Production Docker Updates

This document describes the managed blue-green update path used by the admin
version badge. It is intentionally separate from the application container:
the application never receives Docker's root-equivalent control socket.

## Safety model

The application sends a version and an idempotency key over a permission-limited
Unix socket. The host deployer accepts only `update` and `rollback` operations,
and only versions matching the configured GHCR repository. It does not accept
commands, file paths, Compose service names, ports, or Nginx paths from the web
request.

Before a candidate can receive traffic, the deployer:

1. Pulls the exact release tag and resolves it to an immutable image digest.
2. Verifies the configured OCI source and update-protocol labels.
3. Starts the inactive slot under the installer-discovered Compose project with
   the existing environment and volumes. The immutable Docker container ID and
   Compose project/service labels are persisted and verified before lifecycle
   commands; a same-name replacement fails closed.
4. Keeps singleton-sensitive schedulers and shared queue consumers in `standby`,
   while request-support and horizontally safe workers remain available.
5. Requires Docker health, HTTP `/health`, the embedded application version,
   and the standby runtime state to agree with the requested release.
6. Atomically rewrites the managed Nginx upstream, runs `nginx -t`, verifies
   the active route from `nginx -T`, performs a graceful reload, and probes the
   candidate through the real Nginx virtual host before accepting the switch.
7. Fsyncs a handoff journal bound to the old container's immutable ID, waits
   until `/health` reports zero ordinary requests and hijacked connections
   continuously for `drain_duration`, revalidates the candidate route and
   health, stops the old container, then activates the candidate background
   runtime with a host-only signal and verifies `/health` reports `active`.
8. Persists the immutable digest and completes the deployment.

Until step 6 succeeds, all traffic stays on the old container. During candidate
stabilization, the old container remains the sole owner of singleton-sensitive jobs.
If health, stabilization, or background activation fails after the switch, the
deployer starts and reactivates the old container before restoring its Nginx
upstream. State is fsynced around each transition, so a deployer or host restart
recovers deterministically. During handoff recovery it checks the journal-bound
old container's Docker running state: a running container is drained and stopped,
while an already stopped container skips the dead drain endpoint and proceeds
directly to candidate activation.

The previous container is kept stopped rather than deleted. The next deployment
reuses that inactive slot only after a new image has been pulled. A rollback to
that retained version starts it directly without pulling or requiring a label
that may not exist on the initial pre-deployer image.

## Database contract

The candidate and active container share PostgreSQL. Migrations use a PostgreSQL
advisory lock, but an image rollback cannot undo a destructive schema migration.
For this reason, managed releases follow an expand/contract rule. The release
gate permits only:

- New tables and PostgreSQL types.
- Non-unique indexes on tables created in the same release. Indexes on tables
  that already exist at the release baseline must use
  `CREATE INDEX CONCURRENTLY IF NOT EXISTS` in a dedicated `*_notx.sql`
  migration so candidate startup does not block writes from the active slot.
  A unique index is allowed only on a table created after the managed-update
  schema baseline.
- A plain nullable column on an existing table using a known nullable built-in
  PostgreSQL type. `serial` variants are rejected because they implicitly add a
  sequence, default, and `NOT NULL`; custom and qualified domain types are
  rejected because the gate cannot prove that the domain is nullable.
- `INSERT INTO ... VALUES` with literal value rows, optionally ending in
  `ON CONFLICT ... DO NOTHING`. Query-form inserts and `CREATE TABLE AS` are
  rejected because writable CTEs can hide data updates or deletes inside them.

`CREATE TABLE` is limited to a standalone parenthesized column definition;
partition children, `INHERITS`, typed `OF` tables, `AS`, and trailing table
clauses fail closed. PostgreSQL Unicode escaped identifiers (`U&"..."`) also
fail closed rather than being normalized ambiguously. For every
`CREATE INDEX CONCURRENTLY IF NOT EXISTS`, the runtime migration runner resolves
the parent table schema and persists a pending step before execution. It removes
a same-name invalid index and internally executes a strict create without
`IF NOT EXISTS`, so a successful command cannot silently reuse an unrelated
object. It then stores the created index OID and `pg_get_indexdef` result. A
restart reuses the step only when that proof still matches the same target and
the index is valid, ready, and live. A healthy index with only a pending proof
fails closed instead of opening a live uniqueness or performance gap by dropping
an unproven object.

PostgreSQL backups acquire the same migration advisory lock on a dedicated
database session before `pg_dump` starts. The lock remains held until the dump
stream is closed and the process has exited, so a backup cannot capture a
partially finalized non-transactional migration journal or index. An unlock
failure discards the database session instead of returning a possibly
lock-owning connection to the pool.

Existing migration files are immutable. Drops (including constraints and
defaults), renames, type changes, constraints on existing tables, truncation,
and data updates/deletes/merges/copies require a maintenance release after all
rollback candidates no longer depend on the old schema. Unknown SQL forms fail
closed. A new migration filename must also sort strictly after the last migration
in the prior release, matching the runner's ordering and preventing fresh and
already-upgraded databases from applying the same file at different positions.

The checker compares every release cumulatively to the immutable commit for
`v0.1.164-ts.1` (`0572569c6e0187fd02655bec2d9439e30d9edc04`). The base must be
an ancestor of the release, and SQL is read from the release commit rather than
the runner worktree. Semantic fingerprints also reject a new migration that
replays a statement already present at the baseline, even when comments,
whitespace, or keyword case differ.

The gate also compares the release with every valid `v*-ts.*` tag whose commit
is between the immutable baseline and the release commit. This includes
annotated and lightweight tags left by failed workflows, while excluding
side-branch and future tags. Therefore every migration ever associated with a
reachable version tag remains immutable even when that version never became a
completed GitHub Release. The preceding completed release is separately
authenticated by its public, non-prerelease completion marker, tag object,
commit, GHCR and optional DockerHub manifest digests, and deployer checksum
manifest.

The Release workflow requires the tagged commit to be reachable from
`origin/main`; a side-branch tag cannot publish an image. Every build job uses
the validated commit ID. Immediately before publication, the workflow fetches
with tag pruning and verifies the annotated tag object, target commit, main
ancestry, release ordering, committed `VERSION`, and absence of same-version
GitHub Releases and registry images again. GoReleaser creates a draft release.
The workflow binds each registry version tag to the digest recorded by that
exact GoReleaser build, then verifies the amd64 and arm64 manifests, deployer
checksums, and schema-3 completion ledger. Before mutating a registry, its
current `latest` must equal the digest bound to the preceding completed release,
or the pinned one-time bootstrap digest; missing or drifted tags fail closed.
It promotes only those verified `latest` tags and restores the same authenticated
digests if publication is confirmed to remain a draft. Registry copies disable
automatic index wrapping so the pinned bootstrap's single-platform manifest and
later multi-platform indexes retain their exact digests. Enabling a new registry
requires a reviewed ledger transition that deliberately seeds and pins its
bootstrap image first. Only after
every promoted tag
resolves to the build digest does it publish the GitHub Release as latest. A
partial release must never
be deleted, overwritten, or resumed with `--clobber`; fix the cause and use a new
immutable fork tag. Build base images are pinned by digest, and the release job
does not mutate `go.mod`, `go.sum`, or the tagged source tree.

Fork release tags are a required immutable boundary, not an advisory control.
Configure two active repository tag rulesets for `refs/tags/v*-ts.*`: an
immutable layer that forbids updates and deletion with no bypass actors, and a
creation layer that forbids creation with a single Deploy Key bypass. The
repository must retain exactly one writable deploy key named
`Sub2API release tag promoter`; its private key is stored only in the
`RELEASE_TAG_DEPLOY_KEY` Actions secret. Set `RELEASE_TAG_RULESET_ID` and
`RELEASE_TAG_CREATION_RULESET_ID` to the corresponding numeric ruleset IDs. The
workflows fail closed if either variable, ruleset, or the dedicated credential
is missing or inconsistent.

The version file is part of that immutable source boundary. Update and commit
`backend/cmd/server/VERSION`, merge the release PR, then require the exact merged
`main` SHA to pass CI, Release Preflight, and final independent audit before any
tag exists. Trigger `Promote Release` with that version and full SHA; only this
workflow may create the annotated tag and invoke the reusable Release workflow.
The release gate requires the file to equal the tag without its leading `v` and
never edits the default branch after a release. Do not create or push release
tags manually.

GitHub may redact ruleset bypass actors from the workflow token's API response.
The workflow therefore validates all runtime-visible rule structure and rejects
visible unexpected actors, while the pre-promotion operator audit must use the
administrator view to verify the two rulesets and the sole writable deploy key.
Changing the fixed baseline requires a reviewed code change after a maintenance
migration has completed and old images are no longer rollback candidates.

Behavior-defining statements supported by the checker (`CREATE FUNCTION`,
`CREATE PROCEDURE`, and `CREATE TRIGGER`, including supported `OR REPLACE`
forms) require explicit statement-level review. After verifying that both the
old and new application images remain compatible with the behavior, place this
exact line immediately before that one statement:

```sql
-- sub2api-managed-update: reviewed-compatible
CREATE OR REPLACE FUNCTION example_function() ...;
```

The annotation cannot override destructive/data-rewrite statements, unsafe
`ALTER TABLE`, a unique index on an existing table, or an unknown SQL form. The
Release workflow will not publish an automatically deployable image when any
part of the cumulative check fails.

## Initial installation

Prerequisites:

- Docker Engine and Docker Compose v2
- Nginx managed by systemd
- `jq`, `curl`, and the standard GNU/Linux administration tools
- A running `sub2api` Compose service container, with its project/service labels,
  exposing container port 8080 on loopback
- The self-contained deployer bundle for the host architecture

Run:

```bash
tar -xzf sub2api-deployer-linux-amd64.tar.gz
cd sub2api-deployer-linux-amd64
sha256sum --check MANIFEST.sha256
sudo ./install-sub2api-deployer.sh \
  --nginx-site /etc/nginx/sites-enabled/tokensupply.conf \
  --nginx-probe-url http://127.0.0.1/health \
  --nginx-probe-host tokensupply.net \
  --deployer-binary ./sub2api-deployer-linux-amd64 \
  --deployer-checksums ./MANIFEST.sha256 \
  --docker-config /root/.docker/config.json \
  --activation-version VERSION_WITH_PROTOCOL_V2
```

`--docker-config` is optional for public images. For a private registry it copies
only the selected Docker credential file into
`/etc/sub2api-deployer/docker/config.json` with mode `0600`; later installer
upgrades preserve that dedicated credential unless a replacement is supplied.
The Compose working directory must be on a service-readable persistent path such
as `/opt/sub2api`. Paths under `/root`, `/home`, or `/run/user` are rejected
because the hardened systemd service cannot read them reliably.

The probe URL is mandatory and must reach the actual Nginx `/health` route.
Supply the Host header when a loopback request would otherwise select the wrong
virtual host. The installer performs read-only discovery and body validation
first. It then installs the deployer, creates the blue/green config, changes
exactly one active Nginx `proxy_pass`, validates the effective `nginx -T`
configuration, reloads Nginx, and explicitly restarts and verifies
`sub2api-deployer.service`. If a step fails, the installer makes a best-effort
transactional rollback of replaced files, Nginx, directory metadata, and the
previous service state. If any rollback step itself fails, it leaves the deployer
stopped when possible, prints a critical error, and preserves its recovery backup
directory for manual repair. It leaves the running app container untouched.
On an upgrade, the installer first requires an idle, healthy deployer, verifies
that persisted state and the active-slot marker agree, and then stops only that
update control plane before taking the migration snapshot. Host-only state can
be migrated to the systemd-managed state directory. The socket and marker paths
cannot change in-place because running and retained containers still mount them;
the installer fails before mutation if those paths differ. Rollback normally
restores the old paths and service while application traffic continues through
Nginx; heed the critical error and preserved backup path if rollback is incomplete.

The application container receives the deployer socket group as a supplementary
group through the Compose override. Fresh installations create a dedicated
`sub2api-deployer` system group. Existing installations retain their configured
GID so a deployer-only upgrade cannot revoke access from the running container.
`--socket-gid` remains available for an explicit operator-selected GID.

The first protocol-v2 release is deployed through the same blue-green state
machine over the host Unix socket. Do not bootstrap it with `docker compose up`,
which would recreate the only active container:

```bash
curl --fail --show-error --max-time 10 \
  --unix-socket /run/sub2api-deployer/deployer.sock \
  -H 'Content-Type: application/json' \
  -d '{"action":"update","target_version":"VERSION_WITH_PROTOCOL_V2","expected_current_version":"CURRENT_VERSION","request_id":"bootstrap-VERSION_WITH_PROTOCOL_V2"}' \
  http://localhost/v1/deployments
```

The socket directory is mounted instead of the socket inode. The systemd unit
preserves that directory across service restarts, and tmpfiles.d recreates it
after a host reboot. The installer compares the host and running-container inode
and verifies socket access as the application UID before committing an upgrade.

## Verification

```bash
systemctl --no-pager --full status sub2api-deployer
DEPLOYER_HEALTH=$(curl --fail --show-error --max-time 10 \
  --unix-socket /run/sub2api-deployer/deployer.sock \
  http://localhost/v1/health)
printf '%s\n' "$DEPLOYER_HEALTH" | jq .
docker exec sub2api test -S /run/sub2api-deployer/deployer.sock
nginx -t
ACTIVE_PORT=$(printf '%s\n' "$DEPLOYER_HEALTH" | jq -er '.active_port')
curl --fail --show-error --max-time 10 \
  "http://127.0.0.1:${ACTIVE_PORT}/health"
```

The admin version response must report:

```json
{
  "deployment_mode": "docker-managed",
  "deployment_ready": true
}
```

## Operations and recovery

Job state is stored at `/var/lib/sub2api-deployer/state.json`. The image used for
future Compose commands is pinned in `/var/lib/sub2api-deployer/image.env`.
State records immutable IDs for active, previous, and in-progress containers;
names are retained only for operator readability and must still resolve to the
recorded ID before any lifecycle command.
The mutable Nginx upstream is stored under
`/var/lib/sub2api-deployer/nginx`; `/etc/nginx/conf.d` contains only a static
loader. These writable parent directories permit atomic sibling-file writes
while the rest of the host filesystem remains read-only to the service. None
of these files contains the application's database, Redis, JWT, or administrator
secrets.
The active background slot marker is stored under
`/var/lib/sub2api-deployer/runtime` and mounted read-only into the application.
Only the host deployer can authorize a background slot; the marker also lets the
promoted container restart as active after a Docker or host restart.

Useful commands:

```bash
journalctl -u sub2api-deployer -f
docker ps --filter name=sub2api
cat /etc/nginx/conf.d/sub2api-managed-upstream.conf
cat /var/lib/sub2api-deployer/nginx/managed-upstream.conf
```

If the UI reports `rollback_failed` or `degraded`, stop further deployment
attempts. Keep the healthy container running, point the mutable managed upstream
at its loopback port, and validate before reload. Then stop the deployer, reconcile
its state to the slot that Nginx is actually serving, and start it again:

```bash
nginx -t
systemctl reload nginx
systemctl stop sub2api-deployer
/usr/local/sbin/sub2api-deployer \
  -config /etc/sub2api-deployer/config.json \
  -reconcile-slot sub2api-blue
systemctl start sub2api-deployer
curl --fail --show-error --max-time 10 \
  --unix-socket /run/sub2api-deployer/deployer.sock \
  http://localhost/v1/health
```

Replace `sub2api-blue` with `sub2api-green` when that is the slot selected in
`/var/lib/sub2api-deployer/nginx/managed-upstream.conf`. The reconcile command
fails closed unless the daemon is stopped and the selected route is healthy.

Do not run `docker compose up` without the deployer image env file,
`compose.deployer.yml`, and `SUB2API_DEPLOYER_SOCKET_GID`; doing so bypasses the
pinned image, supplementary socket group, and socket mount.

## Request draining

Nginx reload is graceful and new requests move to the candidate immediately.
The old application exposes drain capability, active ordinary requests, and
hijacked connections through `/health`; idle keepalive sockets are not blockers.
The deployer requires the blocker count to stay at zero continuously for
`drain_duration`. If blockers remain until `drain_timeout`, it leaves both
containers running, keeps the old slot as background owner, keeps the candidate
on the traffic route in standby, and reports `degraded` instead of interrupting
long WebSocket connections. A later stopped-daemon `-reconcile-slot` waits again
and completes once the blockers drain.

Legacy images without drain metrics fail closed during normal managed updates.
The single initial container recorded by a fresh deployer installation is the
only bootstrap exception: after the healthy candidate is continuously confirmed
on the Nginx route, the deployer gracefully stops that initial container using
`stop_timeout`. A recovery that cannot prove this exact initial-container case
still requires the explicit stopped-daemon `-force-unobservable-drain` override.
