# Managed deployment updates

Container images set `SUB2API_UPDATE_MODE=managed`. Other immutable hosting
environments can opt in with the same environment variable. The running version
and latest official release remain visible, including release notes, but the
administrator UI directs updates and rollbacks to the deployment lifecycle.

The update and rollback API endpoints return HTTP 409 with reason
`DEPLOYMENT_MANAGED_UPDATE` before fetching/downloading releases or modifying
files. This avoids non-root directory permission failures, updates lost when a
container is recreated, and accidental replacement of custom builds.

For Docker Compose, pull the desired image tag and recreate the application
service. For Railway or another hosting platform, deploy the desired image or
source revision. Use the previous known-good image/revision for rollback. Keep
database and data volumes, and take backups before upgrades.

Native release binaries keep their existing in-place update workflow when the
environment variable is absent. Setting the environment variable to an empty
value preserves the embedded source/release build identity; operators who do
this in a container are responsible for writable paths and persistence.
