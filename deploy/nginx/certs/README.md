# TLS Certificates

Nginx reads its certificate from this directory. Both files are required and
must use exactly these names:

```text
fullchain.pem   # server certificate + intermediate chain
privkey.pem     # private key (PEM, unencrypted — Nginx cannot prompt for a passphrase)
```

**Nginx will not start if these files are missing.** Put them in place before
running `docker compose -f docker-compose.local.yml up -d`.

Restrict the key so only root can read it:

```bash
chmod 600 privkey.pem
chmod 644 fullchain.pem
```

The certificate must cover the domain configured as `DOMAIN` in `.env`.

`*.pem` in this directory is gitignored — keys are never committed.

## Self-signed certificate (testing only)

To bring the stack up before a real certificate is available. Browsers and
`curl` will reject it unless you pass `-k`:

```bash
openssl req -x509 -newkey rsa:2048 -nodes -days 365 \
  -keyout privkey.pem -out fullchain.pem \
  -subj "/CN=your-domain.example.com" \
  -addext "subjectAltName=DNS:your-domain.example.com"
chmod 600 privkey.pem
```

## Renewing

Copy the new files over the old ones, then validate and reload without
dropping connections:

```bash
cd ..                # back to the deploy directory
docker compose -f docker-compose.local.yml exec nginx nginx -t
docker compose -f docker-compose.local.yml exec nginx nginx -s reload
```
