# DEPLOYMENT KNOWLEDGE BASE

## OVERVIEW
`deploy/` is the operator-facing subtree for Docker Compose, systemd installs, env templates, and deployment scripts.

## STRUCTURE
```text
deploy/
├── docker-compose.local.yml      # Preferred local-directory deployment
├── docker-compose.yml            # Named-volume deployment
├── docker-compose.dev.yml        # Build-from-source local development
├── docker-compose.standalone.yml # App-only deployment with external Postgres/Redis
├── docker-deploy.sh              # One-click Docker preparation
├── install.sh                    # Binary install / upgrade / uninstall
├── docker-entrypoint.sh          # Container permission fix + exec wrapper
├── .env.example                  # Docker env template
├── config.example.yaml           # Binary/systemd config template
└── sub2api.service               # Systemd unit
```

## WHERE TO LOOK
| Task | Location | Notes |
|------|----------|-------|
| Choose Docker variant | `docker-compose.local.yml`, `docker-compose.yml`, `docker-compose.standalone.yml` | Local directories are preferred; standalone expects external services |
| One-click Docker setup | `docker-deploy.sh` | Downloads local compose/env templates and generates secrets |
| Binary install / upgrade | `install.sh`, `sub2api.service` | Systemd path is `/opt/sub2api`; setup wizard remains relevant here |
| Container startup behavior | `docker-entrypoint.sh` | Fixes `/app/data` ownership, then re-execs as `sub2api` |
| Environment variables | `.env.example`, `config.example.yaml` | Docker favors env vars; binary install can use config YAML |
| Data management daemon | `DATAMANAGEMENTD_CN.md`, `install-datamanagementd.sh` | Socket path is fixed at `/tmp/sub2api-datamanagement.sock` |

## CONVENTIONS
- Prefer `docker-compose.local.yml` for self-hosted deployments because data lives in local directories and migrates cleanly by archiving the deploy folder.
- Docker deployments rely on `AUTO_SETUP=true`; binary/systemd deployments still expect the normal setup wizard flow.
- Keep `JWT_SECRET` and `TOTP_ENCRYPTION_KEY` fixed across restarts; empty values trigger random regeneration and break sessions or 2FA.
- `docker-compose.standalone.yml` is only for environments with externally managed PostgreSQL and Redis.
- Container and service units run as `sub2api`; permission fixes happen in entrypoint/service setup, not in app code.

## ANTI-PATTERNS
- Using named-volume compose when you actually need easy backup/migration behavior.
- Removing or changing `PGDATA=/var/lib/postgresql/data` in `docker-compose.yml`; Postgres 18 otherwise falls back to the image default path and can appear to lose data after recreate.
- Assuming PostgreSQL or Redis are exposed on the host by default; the compose files keep them on the internal network unless you add debug ports yourself.
- Treating `JWT_SECRET` or `TOTP_ENCRYPTION_KEY` as optional in long-lived deployments.
- Moving the datamanagementd socket to a different path without updating the host/container mount strategy.

## COMMANDS
```bash
cd deploy && cp .env.example .env
cd deploy && docker compose -f docker-compose.local.yml up -d
cd deploy && docker compose -f docker-compose.local.yml logs -f sub2api
cd deploy && docker compose -f docker-compose.local.yml down
cd deploy && sudo ./install.sh
cd deploy && sudo ./install.sh upgrade
```

## NOTES
- `docker-deploy.sh` downloads `docker-compose.local.yml` but saves it as `docker-compose.yml`; read the script before assuming the filename tells you which variant is in use.
- `install.sh` is bilingual and defaults to Chinese in non-interactive `curl | bash` flows.
- `sub2api.service` uses hardening flags such as `NoNewPrivileges=true` and `ProtectSystem=strict`; preserve those unless deployment requirements truly change.
- Keep root deployment guidance pointer-oriented; deploy-specific gotchas should live here.
