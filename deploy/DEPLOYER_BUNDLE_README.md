# Sub2API deployer bundle

This bundle installs or upgrades only the host deployer. It does not update,
stop, or recreate the running Sub2API application container.

Verify the bundle before installation:

```bash
sha256sum --check MANIFEST.sha256
```

Then run the installer with the binary included in this directory:

```bash
sudo ./install-sub2api-deployer.sh \
  --deployer-binary ./sub2api-deployer-linux-amd64 \
  --deployer-checksums ./MANIFEST.sha256 \
  --install-dir /opt/sub2api \
  --container sub2api \
  --nginx-site /etc/nginx/sites-enabled/sub2api \
  --nginx-probe-url http://127.0.0.1/health \
  --nginx-probe-host your.public.host
```

Use the `arm64` binary and bundle on ARM64 hosts. Existing installations retain
their configured socket GID. Fresh installations create a dedicated
`sub2api-deployer` system group unless `--socket-gid` is explicitly supplied.

This host-only installation is the mandatory one-time bootstrap when the
current `/v1/health` response does not report
`"control_plane_upgrade_ready": true`. Do not start an application update until
the installer succeeds and this check returns `true`:

```bash
curl --fail --silent \
  --unix-socket /run/sub2api-deployer/deployer.sock \
  http://localhost/v1/health \
  | jq -e '.status == "ok" and .degraded == false and .job_running == false and .control_plane_upgrade_ready == true'
```

The installer verifies that the active application container ID, version,
port, Nginx route, and socket-directory inode remain unchanged. Once this
bootstrap is complete, successful managed application updates also upgrade the
host deployer from the immutable active image.
