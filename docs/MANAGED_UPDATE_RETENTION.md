# Managed update backup and retention

This document defines the data and application retention contract for the
TokenSupply managed Docker deployer.

## Update order

For an application update, the deployer must perform these operations in this
order:

1. Confirm that the currently routed application is healthy.
2. Create a PostgreSQL custom-format dump and copy the managed configuration.
3. Write a manifest and SHA-256 checksum file, verify every checksum, and
   atomically commit the backup directory.
4. Pull and verify the immutable target image.
5. Start the candidate in standby mode and verify its version and health.
6. Switch Nginx, observe the candidate, drain and stop the old application,
   then activate candidate background work.
7. Persist the successful deployment and mark the job complete.
8. Prune old automatic backups and managed images. Cleanup failure is recorded
   as a warning and must not roll back a healthy new deployment.

Any backup failure stops the update before an image is pulled or a container is
changed.

## Backup layout

The default root is `/var/lib/sub2api-deployer/backups`:

```text
backups/
  automatic/  # managed by the deployer; newest two completed backups remain
  manual/     # operator-owned; never enumerated or removed by automatic cleanup
```

Each completed automatic backup contains:

- `database.dump`: complete PostgreSQL custom-format dump.
- `application-config`: the host-side application `config.yaml` configured by
  the installer (normally `/opt/sub2api/data/config.yaml`).
- Compose environment files, including `.env` and the managed image state.
- Base Compose and deployer override files.
- Deployer configuration, state, and installed binary.
- Private registry Docker credentials when they are configured for the
  deployer.
- Nginx site and managed upstream configuration.
- `manifest.json`: source/target versions, image digest, deployer build, file
  sizes, and hashes.
- `checksums.sha256`: checksums verified before the temporary directory is
  renamed into place.

Files and directories use private permissions. A crash can leave a
`.pending-*` directory; the next update removes pending directories before it
creates a new backup.

The application config and private registry credentials are optional for
compatibility with older layouts. When either path does not exist, the
manifest records it under `skipped`; any other read or copy error blocks the
update.

## Application retention

After a successful deployment the host retains:

- The current running version.
- The immediately previous version as a complete stopped container and image.
- The version before that as an immutable Docker image only.

Rollback to the immediately previous version starts the stopped container.
Rollback to the second previous version recreates the inactive slot from the
retained local image without requiring registry access. A third or older
managed image may be removed only after deployment success. Images without the
configured repository and ownership labels are never removed.

## Database recovery boundary

Application rollback never calls `pg_restore`, rewinds PostgreSQL, or replaces
the live database. Automatically restoring a pre-update database could erase
orders, users, and usage written after the update.

Database restore is a disaster-recovery operation. It requires an operator to
stop writers, choose and verify a backup, assess schema compatibility, and
explicitly run the restore procedure. It is not exposed through the managed
update or rollback API.

## Transition from an older deployer

The deployer control plane is upgraded only after an application update has
successfully switched traffic. Therefore, the update that first installs this
retention-capable deployer still runs under the previous deployer behavior.
Starting with the following update, the pre-update backup and bounded retention
contract is enforced automatically.

Operators who require the new backup contract for that first application
update must install the matching host deployer bundle before starting the
application update. The host-only bundle does not stop or recreate the running
application.
