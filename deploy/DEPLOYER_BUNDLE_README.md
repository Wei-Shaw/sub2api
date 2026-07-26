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
