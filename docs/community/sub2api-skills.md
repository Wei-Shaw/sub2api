# Sub2API Assistant Skill, Telegram Operations Bot, and Docker Sidecar

A community-maintained integration is available for users who want to manage Sub2API resources through an assistant skill and an optional self-hosted Telegram operations bot.

Repository: <https://github.com/deltrivx/sub2api-skills>

## What it provides

The integration can help with:

- Listing Sub2API accounts, groups, balance, API tokens, models and channels.
- Managing user API tokens with masked output.
- Copying or applying token values without printing secrets in chat.
- Inspecting local config files with best-effort redaction.
- Answering Sub2API usage questions through the skill help workflow.
- Importing account files from `.json` / `.txt` files and common archive formats, with size and extraction limits.
- Running a self-hosted Telegram operations bot for status checks, diagnostics, backups, account checks, and confirmation-protected maintenance actions.
- Running the Telegram bot either directly on the host or as a Docker sidecar next to a Docker-based Sub2API deployment.

## Installation

```bash
npx skills add https://github.com/deltrivx/sub2api-skills --skill sub2api
```

## Configuration

Use environment variables or local runtime secrets. Do not commit real credentials.

```bash
export SUB2API_BASE_URL="https://<your-sub2api-host>"
export SUB2API_ACCESS_TOKEN="<your-sub2api-access-token>"
export SUB2API_USER_ID="<your-user-id>"
```

## Telegram bot template

The repository includes a Telegram bot template under `skills/sub2api/templates/telegram-bot.py`.

The template is optional and self-hosted. It is intended for deployments that want chat-based operational checks and low-risk maintenance controls, including:

- `/status`, `/accounts`, `/models`, `/channels`, `/tokens`, `/debug`
- account-file import guidance and local backups
- confirmation-protected `/restart` and `/update` operations
- account availability checks with soft-delete / hard-delete choices and a second confirmation step

`/restart` can show Bot / Sub2API selection buttons first, then a confirm/cancel step. `/update` sends an immediate “checking update” notice before it runs longer checks so users do not mistake network or registry checks for a stalled bot.

Before using the bot template, restrict allowed chat IDs, store secrets in environment files or runtime secrets, and review the code against your own deployment paths, service names, database schema and permission model.

## Host-level deployment

Host-level deployment is suitable when Sub2API runs as a binary, systemd service, or another host-managed service.

At a high level:

1. Copy `skills/sub2api/templates/telegram-bot.py` to the host.
2. Provide local environment variables or a local secrets file.
3. Run it under systemd or another service manager.
4. Configure `SUB2API_UPDATER_SCRIPT` if you want `/update` to call a local update helper.

For host-level deployments, `/update` calls the configured updater and returns “already up to date” when no update is available.

## Docker sidecar deployment

Docker sidecar deployment is suitable when Sub2API itself runs in Docker. The bot runs as a separate container and does not modify the official Sub2API image or write files into the Sub2API container.

Published sidecar image:

```text
ghcr.io/deltrivx/sub2api-skill:latest
```

Recommended layout:

```text
<docker-root>/sub2api/
  docker-compose.yml
  data/
<docker-root>/sub2api-skill/
  docker-compose.yml
  .env
  config/sub2api-bot-secrets.json
  data/
```

The sidecar uses environment variables for Sub2API URL, Telegram bot token, allowed chat IDs, admin credentials, database connection, optional proxy settings, and Docker update settings.

Important mounts for the sidecar include:

- `./config:/config` for bot secrets.
- `./data:/data` for offsets, pending operations, import cache and backups.
- The Sub2API data directory mounted read-only.
- The Sub2API compose directory mounted read-only, so `/update` can recreate the Sub2API service using the same compose file.
- `/var/run/docker.sock:/var/run/docker.sock` when confirmation-protected Docker restart/update actions are desired.

Docker `/update` behavior:

1. Immediately replies that it is checking the Docker image update.
2. Compares the local `weishaw/sub2api:latest` image with the remote official image digest.
3. Replies “already up to date” if they match.
4. Shows confirm/cancel buttons if an update is available.
5. On confirmation, pulls the latest Sub2API image.
6. Recreates the `sub2api` service using the existing Sub2API `docker-compose.yml`.
7. Waits for the health check and prunes dangling old images.

This keeps the official Sub2API image and container clean while still allowing a chat-based maintenance workflow.

## Security notes

- Do not commit real tokens, passwords, JWTs, API keys, chat IDs, database addresses, or private network addresses.
- Pass sensitive values through environment variables or local secrets files.
- Restrict bot access with allowed chat IDs.
- Require confirmation codes or buttons for control commands.
- Keep imported accounts unscheduled by default until reviewed.
- Redact secrets in logs and replies.
- Review Docker socket access carefully; mounting the Docker socket gives the sidecar the ability to control local Docker containers.

## Disclaimer

This is an independent community integration and is not an official Sub2API component unless explicitly adopted by Sub2API maintainers.

Review the code before use, test it in a non-production environment, and make sure any SQL queries, service names, file paths, permissions, routing rules and automation behavior match your own deployment.

Users are responsible for protecting credentials, complying with upstream provider terms and local laws, and ensuring that account sharing, quota distribution, API forwarding, billing, imports and administrative operations are authorized in their environment.
