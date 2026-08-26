# Sub2API Docker Image

Sub2API is an AI API Gateway Platform for distributing and managing AI product subscription API quotas.

## Quick Start

```bash
docker run -d \
  --name sub2api \
  -p 8080:8080 \
  -e DATABASE_URL="postgres://user:pass@host:5432/sub2api" \
  -e REDIS_URL="redis://host:6379" \
  ghcr.io/a515642/sub2api:latest
```

## Docker Compose

```yaml
version: '3.8'

services:
  sub2api:
    image: ghcr.io/a515642/sub2api:latest
    ports:
      - "8080:8080"
    environment:
      - DATABASE_URL=postgres://postgres:postgres@db:5432/sub2api?sslmode=disable
      - REDIS_URL=redis://redis:6379
    depends_on:
      - db
      - redis

  db:
    image: postgres:15-alpine
    environment:
      - POSTGRES_USER=postgres
      - POSTGRES_PASSWORD=postgres
      - POSTGRES_DB=sub2api
    volumes:
      - postgres_data:/var/lib/postgresql/data

  redis:
    image: redis:7-alpine
    volumes:
      - redis_data:/data

volumes:
  postgres_data:
  redis_data:
```

## Environment Variables

| Variable | Description | Required | Default |
|----------|-------------|----------|---------|
| `DATABASE_URL` | PostgreSQL connection string | Yes | - |
| `REDIS_URL` | Redis connection string | Yes | - |
| `PORT` | Server port | No | `8080` |
| `GIN_MODE` | Gin framework mode (`debug`/`release`) | No | `release` |

## Supported Architectures

- `linux/amd64`
- `linux/arm64`

## Tags

- `latest` - Latest stable release
- `x.y.z` - Specific version
- `x.y` - Latest patch of minor version
- `x` - Latest minor of major version

## Upgrade and Automatic Migrations

Database migrations run automatically when the application starts. The image applies pending embedded SQL migrations before serving requests, records them in `schema_migrations`, and serializes concurrent migration attempts with a PostgreSQL advisory lock. A migration failure is reported in the container logs and intentionally blocks startup.

For a production upgrade, back up PostgreSQL first, then pull and recreate only the application container:

```bash
cd /path/to/sub2api-deploy
docker compose exec -T postgres pg_dump -U "$POSTGRES_USER" -d "$POSTGRES_DB" > "backup-$(date +%Y%m%d-%H%M%S).sql"
docker compose pull sub2api
docker compose up -d sub2api
docker compose ps
docker compose logs --since=10m sub2api
```

Do not run `docker compose down -v` or delete database/data volumes during an upgrade. Migrations are forward-only; if a rollback is required after a schema migration has run, restore a compatible database backup before starting the older image.

## Links

- [GitHub Repository](https://github.com/a515642/sub2api)
- [Documentation](https://github.com/a515642/sub2api#readme)
