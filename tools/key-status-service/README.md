# Sub2API Key Status Service

Small FastAPI service for exact API-key status checks against a Sub2API PostgreSQL database. Its usable for fetching the status of specific api-keys created on the system locally.

## Security Model

This service does not list keys.

Available endpoints:

- `GET /health`
- `GET /api-keys/status?key=sk-...`

The status endpoint requires `X-API-Key: <KEY_STATUS_API_TOKEN>` or `Authorization: Bearer <KEY_STATUS_API_TOKEN>`.

## Deploy

1. Copy this folder to a server that already runs Sub2API.
2. Copy `.env.example` to `.env`.
3. Fill in `KEY_STATUS_API_TOKEN` and the Sub2API database credentials.
4. Make sure the Docker network name in `docker-compose.example.yml` matches the Sub2API compose network.
5. Run:

```bash
docker compose -f docker-compose.example.yml up -d --build
```

## Test

```bash
curl http://SERVER_IP:18081/health

curl -H "X-API-Key: $KEY_STATUS_API_TOKEN" \
  "http://SERVER_IP:18081/api-keys/status?key=sk-your-key"
```

## Portability

The code has no hard-coded IP, domain, server identifier, or deployment-specific signature.

Server-specific values live only in `.env` and compose/network settings:

- `KEY_STATUS_API_TOKEN`
- `DATABASE_HOST`
- `DATABASE_PORT`
- `DATABASE_NAME`
- `DATABASE_USER`
- `DATABASE_PASSWORD`
- `SUB2API_ACCOUNT_EMAIL`
- Docker network name
- published host port, if changed from `18081`

To deploy on a different server, update those values only.
