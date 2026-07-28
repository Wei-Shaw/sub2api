# Sub2API production deploy and strict 100G policy

## Authority and entrypoint

This runbook documents the approved production mechanism; it does not itself
authorize a deploy, paid smoke, credential access, or destructive recovery. The
only production entrypoint is `production-deploy-guard.sh`. Before invoking it,
the approved host secret mechanism injects the paid-smoke credential into the
process environment. Do not show, assign, or copy that credential into a shell
transcript, manifest, unit, or command line.

Compose invocation shape:

```bash
./deploy/production-deploy-guard.sh \
  --runner compose \
  --commit '<40-hex-commit>' \
  --app-image 'registry.example/sub2api@sha256:<64-hex>'
```

The host mechanism must also provide digest-pinned PostgreSQL and Redis image
references for the Compose path. They are operational configuration, not
arguments accepted by the guard.

Systemd invocation shape:

```bash
./deploy/production-deploy-guard.sh \
  --runner systemd \
  --commit '<40-hex-commit>' \
  --app-binary '/approved/staging/sub2api' \
  --app-sha256 '<64-hex>'
```

The script never accepts `$@` or another operator-supplied deploy command. The
lock is fixed at `/run/lock/sub2api-prod-deploy.lock`; the Compose project and
systemd unit names are fixed by the strict non-secret manifest. Read-only
detection must find exactly one active application runner, and it must match
`--runner`. Zero runners, dual runners, a second Compose project, or a mismatch
fails closed.

Install an approved copy of `production-deploy.conf.example` as
`/etc/sub2api/production-deploy.conf`. Replace the example mount values with the
approved target, source, and filesystem UUID. The guard verifies all three with
`findmnt`, `readlink`, and `blkid`; missing or mismatched identity fails before
backup or replacement. The manifest contains paths and service identifiers
only and is parsed as `KEY=value` data rather than shell-sourced.

## Strict 100G allocation

The budgets sum to the whole approved 100G filesystem and are not fungible:

| Category | Hard budget |
| --- | ---: |
| OS and container runtime | 20G |
| PostgreSQL plus Redis | 25G |
| Immutable releases/images | 8G |
| Media temporary space | 20G |
| Application logs | 5G |
| journald | 4G |
| Rollback backups | 8G |
| Forced free space | 10G |

Media temporary space has no automatic category reclaim. Crossing another
category's limit never authorizes deleting database files, Redis data, media,
Docker volumes, or broad directory trees. Preserve the 10G forced-free reserve
by capacity expansion or an independently approved exact-target retention
action.

Filesystem states use hysteresis:

- `70%`: alert, investigate category growth, and block every regular deploy.
- `80%`: block and stop the entire application runner, which stops new API
  intake and all media creates; remain quiesced until usage is below `75%`.
- `90%`: incident and stop the entire application runner; remain quiesced until
  usage is below `70%`.

`production-disk-readback.sh` enforces the measured category budgets,
thresholds, recovery boundaries, and forced-free reserve. It uses the same
production lock as deployment and writes the readable bounded artifact
`/run/sub2api-prod-quiesce.state`. At block/incident it stops the detected
Compose or systemd application runner. Only the matching hysteresis recovery
starts that same runner. A restart failure records `STATE=recovery_failed`,
keeps `QUIESCED=1`, and exits nonzero. The guard never deletes data.

## Log and timer policy

- Sub2API Compose JSON logs: `100MB × 10`, compression enabled, operational
  retention ceiling 7 days. The timer alerts on drift rather than deleting.
- The systemd runner uses journald and has no parallel Docker log stream.
- Caddy: `50MB × 10` in `deploy/Caddyfile`.
- journald persistent maximum: `4G`; runtime maximum: `512M`; retention ceiling:
  7 days; compression enabled.

Install and read back host policy through the approved host-change procedure:

```bash
install -D -m 0644 deploy/journald-sub2api.conf /etc/systemd/journald.conf.d/sub2api.conf
install -D -m 0644 deploy/production-deploy.conf.example /etc/sub2api/production-deploy.conf
install -D -m 0644 deploy/sub2api.service /etc/systemd/system/sub2api.service
install -D -m 0644 deploy/sub2api-disk-guard.service /etc/systemd/system/sub2api-disk-guard.service
install -D -m 0644 deploy/sub2api-disk-guard.timer /etc/systemd/system/sub2api-disk-guard.timer
install -D -m 0755 deploy/production-disk-readback.sh /opt/sub2api/deploy/production-disk-readback.sh
systemctl daemon-reload
systemctl enable --now sub2api-disk-guard.timer
systemctl restart systemd-journald
systemd-analyze cat-config systemd/journald.conf
systemctl show sub2api-disk-guard.timer -p ActiveState -p LastTriggerUSec -p NextElapseUSecRealtime
journalctl --disk-usage
df -hP /opt/sub2api
du -xhd1 /opt/sub2api
```

Do not print unit environments, Compose environments, API keys, or database
passwords.

## Fail-closed deployment transaction

The fixed entrypoint performs this sequence:

1. Acquire the shared production lock, verify mount target/source/UUID, detect
   exactly one active Compose or systemd runner, and require it to match the
   explicit requested runner.
2. Verify the clean immutable Git commit, runner-specific image/binary digest,
   100G baseline, `<70%` usage, normal quiesce state, 10G free reserve, and 8G
   backup budget. Before any Compose quiesce or app change, resolve the final
   service images and require PostgreSQL and Redis digests to exactly equal their
   currently running container digests; drift fails immediately.
3. Stop the application to quiesce all intake, then take PostgreSQL
   custom-format and Redis RDB backups with checksums and the previous immutable
   app version/digest manifest.
4. For Compose, pull and force-recreate only digest-pinned `sub2api`. PostgreSQL
   and Redis are never pulled, recreated, restarted, or targeted by `up -d`;
   their live health is read only. For systemd, install the verified binary under
   `releases/<commit>/`, atomically replace `/opt/sub2api/current`, and restart
   the fixed service. Both paths run migrations through normal application
   startup.
5. Read back the application revision/digest (and systemd `--version`),
   PostgreSQL readiness and both migration tables, Redis `PONG`, and `/health`.
6. With explicit paid-smoke authorization and the host-only smoke credential,
   create one minimal video using `Idempotency-Key: deploy-<commit>`, poll its
   status, and request one content byte. The key and response bodies are not
   logged.
7. Read back Caddy, journald, filesystem usage, directory usage, and the
   disk-guard timer.
8. On any failure after quiescence, the ERR/signal trap retains the PostgreSQL,
   Redis, release-manifest, checksum, and log evidence without restoring or
   mutating live data. Compose recovery recreates the prior immutable app
   artifact/config as a stopped container; systemd recovery atomically restores
   the prior binary symlink. Both verify the prior digest/version, leave the app
   stopped, and only read PostgreSQL readiness plus Redis `PONG`. Automatic
   recovery never runs `dropdb`, `createdb`, `pg_restore`, installs a backup RDB,
   or restarts the old app. Even when artifact rollback succeeds, schema/data
   compatibility remains unproven: the guard writes `STATE=recovery_failed`,
   keeps `QUIESCED=1`, reports `compatibility/data-safety unproven`, and exits
   nonzero for operator-led recovery. No recovery command is hidden with
   `|| true`.

Failure injection is available only in the test-mode branch and never executes
Docker, systemd, paid, or production paths.

## Video idempotency and rollback compatibility

Migration `187_grok_video_request_owners.sql` adds two authoritative tables:

- `grok_video_create_idempotency`: caller scope, hashed `Idempotency-Key`,
  canonical request hash, stable derived upstream key, account pre-binding, and
  replay response.
- `grok_video_request_owners`: immutable external request owner for
  status/content routing. Successful lookups renew a 7-day recovery window;
  terminal status/content retains the binding for 7 days before bounded cleanup.

The create intent and selected account commit before the upstream call. If the
upstream accepts but the response/process is lost, a retry after 15 minutes
reuses the same account and derived upstream key. A provider honoring
`Idempotency-Key` returns the same external task without a second paid create.
Completion stores the replay response and owner in one PostgreSQL transaction.
Redis `grok-video-owner:v2` remains derived only.

Rollback is fail-closed. Stop creates and drain or restore the forward binary
while non-expired rows exist. Never drop either table automatically.
Status/content must never switch to another account when the stored owner is
busy or unavailable.

Read-only migration checks:

```sql
SELECT to_regclass('public.grok_video_create_idempotency');
SELECT to_regclass('public.grok_video_request_owners');
SELECT COUNT(*) FROM grok_video_request_owners WHERE expires_at > NOW();
```

Generated media files must not be persisted on this host. A future persistence
requirement belongs in object storage with lifecycle expiry, not the 100G local
filesystem.
