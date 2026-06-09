# Sub2API Assistant Skill and Telegram Operations Bot

A community-maintained integration is available for users who want to manage Sub2API resources through an assistant skill and an optional self-hosted Telegram operations bot template.

Repository: <https://github.com/deltrivx/sub2api-skills>

## What it provides

The integration can help with:

- Listing Sub2API accounts, groups, balance and API tokens.
- Managing user API tokens with masked output.
- Copying or applying token values without printing secrets in chat.
- Inspecting local config files with best-effort redaction.
- Answering Sub2API usage questions through the skill help workflow.
- Optional Telegram operations bot templates for status checks, diagnostics, account-file import, local backups, notification mute/watch, and confirmation-protected maintenance actions.

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

The repository also includes a Telegram bot template under `skills/sub2api/templates/telegram-bot.py`.

The template is optional and self-hosted. It is intended for deployments that want chat-based operational checks and low-risk maintenance controls, including:

- `/status`, `/accounts`, `/models`, `/channels`, `/tokens`, `/debug`
- account-file import guidance and local backup
- confirmation-protected `/restart` and `/update` operations

Before using the bot template, restrict allowed chat IDs, store secrets in environment files or runtime secrets, and review the code against your own deployment paths, service names, database schema and permission model.

## Disclaimer

This is an independent community integration and is not an official Sub2API component unless explicitly adopted by Sub2API maintainers.

Review the code before use, test it in a non-production environment, and make sure any SQL queries, service names, file paths, permissions, routing rules and automation behavior match your own deployment.

Users are responsible for protecting credentials, complying with upstream provider terms and local laws, and ensuring that account sharing, quota distribution, API forwarding, billing, imports and administrative operations are authorized in their environment.
