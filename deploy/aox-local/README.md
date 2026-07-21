# Aox Local Sub2API Deployment

Local Docker deployment for AOXToken frontend integration testing.
The default image is pinned to the production-tested Sub2API version:
`weishaw/sub2api:v0.1.144`.

```bash
cd deploy/aox-local
cp .env.example .env
docker compose up -d
```

Sub2API will listen on:

```text
http://127.0.0.1:18080
```

Use the static homepage with the AoxToken web app:

```text
apps/web/index.html
```

Runtime data and secrets are intentionally ignored by Git:

```text
.env
data/
postgres_data/
redis_data/
```
