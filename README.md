<div align="center">

<img src="assets/logo.svg" alt="Sub2API Logo" width="128" />

# Sub2API

[![Go](https://img.shields.io/badge/Go-1.27.0-00ADD8.svg)](https://golang.org/)
[![Vue](https://img.shields.io/badge/Vue-3.4+-4FC08D.svg)](https://vuejs.org/)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-15+-336791.svg)](https://www.postgresql.org/)
[![Redis](https://img.shields.io/badge/Redis-7+-DC382D.svg)](https://redis.io/)
[![Docker](https://img.shields.io/badge/Docker-Ready-2496ED.svg)](https://www.docker.com/)

<a href="https://trendshift.io/repositories/21823" target="_blank"><img src="https://trendshift.io/api/badge/repositories/21823" alt="Wei-Shaw%2Fsub2api | Trendshift" width="250" height="55"/></a>

**AI API Gateway Platform for Subscription Quota Distribution**

English | [中文](README_CN.md) | [日本語](README_JA.md)

</div>

## ⚠️ Important Notice

Please read the following carefully before using this project:

- **🚨 Terms of Service Risk**: Using this project may violate the terms of service of Anthropic and other upstream providers. Please review the relevant providers' user agreements before use; all risks arising from such use are borne solely by the user.
- **⚖️ Compliant Use**: Use this project only in compliance with the laws and regulations of your country or region. Any unlawful use is strictly prohibited.
- **📖 Disclaimer**: This project is provided for technical learning and research purposes only. The authors assume no liability for account bans, service interruptions, data loss, or any other direct or indirect damages resulting from the use of this project.
- **🚫 No Commercial Authorization**: The developers of this project have never authorized any individual or organization to conduct any form of commercial operation based on this project. Any commercial activity conducted in the name of or based on this project is unrelated to this project and its developers, and all resulting disputes, losses, and legal liabilities shall be borne solely by the party conducting such activity.

## Overview

Sub2API is an AI API gateway platform designed to distribute and manage API quotas from AI product subscriptions. Users can access upstream AI services through platform-generated API Keys, while the platform handles authentication, billing, load balancing, and request forwarding. Adobe Firefly Web cookie accounts can serve OpenAI-compatible image generation — see [Adobe Firefly Support](#adobe-firefly-support).

## Features

- **Multi-Account Management** - Support multiple upstream account types (OAuth, API Key)
- **Adobe Firefly Images** - OpenAI-compatible image generation and editing from Firefly Web cookie accounts, with optional relay failover ([Adobe Firefly Support](#adobe-firefly-support))
- **API Key Distribution** - Generate and manage API Keys for users
- **Precise Billing** - Token-level usage tracking and cost calculation
- **Smart Scheduling** - Intelligent account selection with sticky sessions
- **Concurrency Control** - Per-user and per-account concurrency limits
- **Rate Limiting** - Configurable request and token rate limits
- **Built-in Payment System** - Supports EasyPay, Alipay, WeChat Pay, and Stripe for user self-service top-up, no separate payment service needed ([Configuration Guide](docs/PAYMENT.md))
- **Admin Dashboard** - Web interface for monitoring and management
- **Composite Groups** - Admin routing layer that resolves requested models to concrete providers for multi-provider groups ([Operator Guide](docs/COMPOSITE_GROUPS.md))
- **External System Integration** - Embed external systems (e.g. ticketing) via iframe to extend the admin dashboard

---

## Adobe Firefly Support

Sub2API can serve OpenAI-compatible image generation from Adobe Firefly Web subscription accounts (browser cookies), with optional OpenAI-shaped relay accounts in the same Adobe group for failover.

Direct Firefly calls use **Firefly Web** (`firefly.adobe.com` / `clio-playground-web`), not Adobe Express. Official Adobe Firefly Services API keys are not supported.

### Supported Scope

- Platform name: `adobe`
- Account types: Firefly cookie accounts (shown as OAuth in the admin UI) and **API Key + Base URL** relay accounts
- Public image targets: `/v1/images/generations` and `/v1/images/edits`, plus the existing no-prefix aliases, and Gemini-native `POST /v1beta/models/{model}:generateContent` / `streamGenerateContent`
- The API key's group must allow image generation (Adobe groups default this to on)
- `n` defaults to 1 and is capped at 10. The cookie path fans out into n Firefly jobs (upstream `n` stays 1) and bills per image; `n>10` is rejected
- `output_format` follows `png`/`jpeg` with a local transcode (`jpg` is accepted as jpeg). `webp` is not supported. Omit it to keep the upstream format
- `/v1/models` lists public external names only; do not send internal `firefly-*` family IDs
- Out of scope for this provider: the video gateway, asynchronous image tasks (`/v1/images/*/async`), official Firefly Services API keys, and non-image protocols such as chat or TTS

### Public Models

| Public model | Notes |
|--------------|--------|
| `gpt-image-2` | Firefly GPT Image 2 |
| `gpt-image-1.5` | Firefly GPT Image 1.5 |
| `gpt-image-2.5-flare` | Firefly GPT Image 2.5 Flare |
| `gpt-image-2.5-sunburst` | Firefly GPT Image 2.5 Sunburst (upstream version name is `prism`) |
| `gemini-2.5-flash-image` / `gemini-3-pro-image` / `gemini-3.1-flash-image` | Gemini image models on Firefly (same public names as the Gemini channel) |
| `flux-pro`, `flux-ultra` | Firefly FLUX |
| `imagen-4`, `imagen-4-fast` | Firefly Imagen 4 |
| `gpt-4o-image` | Firefly GPT-4o Image |
| `runway-gen4-image` | Firefly Runway Gen-4 Image |

Legacy aliases `gpt-image`, `gpt-image-1`, and `gpt-image-1-mini` map to `gpt-image-2` but are not listed by `/v1/models`. Preview names `gemini-2.5-flash-image-preview`, `gemini-3-pro-image-preview`, and `gemini-3.1-flash-image-preview` are also accepted and map to the same model as their non-preview name, but are not listed by `/v1/models` either.

`gpt-image-*` names are shared with official OpenAI image models. They reach Firefly only when the API key is bound to an **Adobe** group; an OpenAI group still talks to OpenAI. Composite groups do **not** auto-detect `gpt-image-*` (the name is ambiguous) — add an explicit composite route. `gemini-*-image` / `gemini-3-pro-image*` are also shared with the Gemini channel, so composite groups decide by endpoint: `/v1/images/*` goes to Adobe, chat-style endpoints (chat completions, responses, messages) go to Gemini, and `/v1beta` goes to whichever platform in the group can serve the name — if both Gemini/Antigravity and Adobe accounts can, it defaults to Gemini unless an explicit route says otherwise. `flux-*`, `imagen-*`, `runway-gen4*`, and `gpt-4o-image` can be auto-detected as Adobe.

### Cookie Account Setup

1. In the admin dashboard, create an **Adobe** group and add a Firefly cookie account.
2. Sign in to Adobe in a browser, open `https://firefly.adobe.com/generate/image`, and **successfully generate an image on that page** (needed to capture the optional ARP session header).
3. Export a Sub2API account JSON with the Chrome/Edge extension in [`tools/adobe-cookie-exporter`](tools/adobe-cookie-exporter) (recommended), then upload it under **Accounts → Import**. Alternatively, in DevTools → Network find a request to `adobeid-na1.services.adobe.com` `/ims/check/v6/token` and copy that request's **Cookie header**. It must include `ims_sid`.
4. Copying `document.cookie` from `firefly.adobe.com` alone is **not** enough. You can also paste the exporter JSON (or the `cookie` string, a `Cookie:` prefix, or a JSON cookie array) into the Adobe cookie field. The optional `arp_session_id` can be pasted in its own field or taken from the same JSON. Paid accounts can omit ARP; FREE accounts should include it. Import does not bind groups; attach the account afterwards.
5. The short-lived access token is optional; leave it empty to exchange the cookie on first refresh. Sub2API keeps refreshing the token from the cookie.
6. Attach the account to the group, then create a Sub2API API key assigned to that group.

When the cookie is no longer valid, re-export it from the browser. The refresher marks the account as error instead of looping forever.

### Relay Accounts (Optional)

The same Adobe group can also hold **API Key + Base URL** relay accounts. Those accounts do not call Firefly; they forward the OpenAI Images payload to `{base_url}` with `Authorization: Bearer`, keeping public model names such as `gpt-image-2` (they are not rewritten to internal `firefly-*` IDs).

Typical uses:

- Failover when a cookie account is out of credits, temporarily unavailable, or not entitled to the requested model/quality
- `gpt-image-*` edits that include a **mask** prefer relay accounts (Firefly cookie accounts do not implement official OpenAI inpaint-mask semantics)
- After a Firefly content-safety rejection (for example `image_unsafe`), Sub2API will not try another cookie account, but it may still try a relay account

An Adobe API-key account without `base_url` is not a relay account.

### `size` / `quality` / `background`

| Family | Client `size` | Behavior |
|--------|---------------|----------|
| `gpt-image-2` / `gpt-image-2.5-*` | `WxH`, empty, or `auto` | Forward pixels as-is; omit size when empty/`auto` so Firefly auto-chooses |
| `gpt-image-1.5` | `WxH` | Snap to `1024x1024` / `1536x1024` / `1024x1536` |
| `gemini-*-image` / `gemini-3-pro-image*` | `WxH`, empty, or `auto` | Map the long edge to a 1K/2K/4K square tier and the nearest `aspectRatio`; empty/`auto` uses Firefly's default 1K square. `gemini-3.1-flash-image` also accepts `1:8`, `1:4`, `4:1`, and `8:1` |
| `flux-*` / `imagen-4*` / `gpt-4o-image` / `runway-gen4-image` | `WxH` | Snap to that family's allowed size enum |

`quality` maps to Firefly `detailLevel`: `low` (default) → 1, `medium` → 3, `high` → 5, `xhigh`/`max` → 5 on v2/1.5 or 7 on 2.5.

The gpt-image family accepts OpenAI `background` values: `transparent`, `opaque`, and `auto`.

`n` defaults to 1 (max 10). Cookie accounts split it into n Firefly jobs and bill per image. `output_format` of `png` or `jpeg` is applied locally after download; `webp` is rejected; omitting it keeps whatever Firefly returned. `background=transparent` cannot be combined with `output_format=jpeg`.

### Billing, Quota, And Failover

- Billing is per image on `1K` / `2K` / `4K` tiers derived from the output long edge. Group image prices win when set; otherwise Adobe channel fallback prices are used.
- Cookie accounts surface Firefly Credits on the admin dashboard. Exhausted accounts are temporarily removed from scheduling (about 30 minutes by default) so the next request can use another account.
- A missing model or quality entitlement fails over to another account; it is not treated as a dead cookie.
- Upstream `429` / `5xx` / network errors are retryable across accounts.

### Example

```bash
curl https://your-sub2api.example.com/v1/images/generations \
  -H "Authorization: Bearer sk-your-sub2api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "model": "gpt-image-2",
    "prompt": "a red panda in a bamboo forest",
    "size": "1024x1024"
  }'
```

The same public model names also accept Gemini-native image generation (the client chooses the protocol; models are not bound to one envelope):

```bash
curl "https://your-sub2api.example.com/v1beta/models/gemini-3-pro-image:generateContent" \
  -H "x-goog-api-key: sk-your-sub2api-key" \
  -H "Content-Type: application/json" \
  -d '{
    "contents": [{"role": "user", "parts": [{"text": "a red panda in a bamboo forest"}]}],
    "generationConfig": {
      "imageConfig": {"aspectRatio": "16:9", "imageSize": "2K"}
    }
  }'
```

`streamGenerateContent` writes **one** SSE frame after generation (`data: {full JSON}\n\n`). It is not token-by-token incremental output and does not send `[DONE]`. Reference images use `parts[].inlineData` (base64) or `fileData.fileUri` (`https://` only).

With `generationConfig.candidateCount` set to n, the response has n candidates (each with an `index`), one image per candidate. Images are returned as `inlineData`; when an API-key relay account only returns an image URL the gateway cannot download, that image is returned as `fileData.fileUri` for the client to fetch. If no image can be delivered at all, the request fails with 502 and is not billed.

`generationConfig.imageConfig.aspectRatio` / `imageSize` (`1K`/`2K`/`4K`) are Gemini-protocol fields; OpenAI Images `size` (`WxH`) is a separate mapping. Banana models on the Gemini path use `aspectRatio`+`imageSize` directly instead of inferring them from WxH; other models and API Key relay accounts receive a `WxH` derived from `aspectRatio`+`imageSize` (long edge = tier edge; ratio-only requests use 1K).

---

## Ecosystem

Community projects that extend or integrate with Sub2API:

| Project | Description | Features |
|---------|-------------|----------|
| ~~[Sub2ApiPay](https://github.com/touwaeriol/sub2apipay)~~ | ~~Self-service payment system~~ | **Now Built-in** — Payment is now integrated into Sub2API, no separate deployment needed. See [Payment Configuration Guide](docs/PAYMENT.md) |
| [sub2api-mobile](https://github.com/ckken/sub2api-mobile) | Mobile admin console | Cross-platform app (iOS/Android/Web) for user management, account management, monitoring dashboard, and multi-backend switching; built with Expo + React Native |

## Tech Stack

| Component | Technology |
|-----------|------------|
| Backend | Go 1.27.0, Gin, Ent |
| Frontend | Vue 3.4+, Vite 5+, TailwindCSS |
| Database | PostgreSQL 15+ |
| Cache/Queue | Redis 7+ |

---

## Nginx Reverse Proxy Note

When using Nginx as a reverse proxy for Sub2API (or CRS) with Codex CLI, add the following to the `http` block in your Nginx configuration:

```nginx
underscores_in_headers on;
```

Nginx drops headers containing underscores by default (e.g. `session_id`), which breaks sticky session routing in multi-account setups.

---

## Deployment

### Method 1: Script Installation (Recommended)

One-click installation script that downloads pre-built binaries from GitHub Releases.

#### Prerequisites

- Linux server (amd64 or arm64)
- PostgreSQL 15+ (installed and running)
- Redis 7+ (installed and running)
- Root privileges

#### Installation Steps

```bash
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | sudo bash
```

The script will:
1. Detect your system architecture
2. Download the latest release
3. Install binary to `/opt/sub2api`
4. Create systemd service
5. Configure system user and permissions

#### Post-Installation

```bash
# 1. Start the service
sudo systemctl start sub2api

# 2. Enable auto-start on boot
sudo systemctl enable sub2api

# 3. Open Setup Wizard in browser
# http://YOUR_SERVER_IP:8080
```

The Setup Wizard will guide you through:
- Database configuration
- Redis configuration
- Admin account creation

#### Upgrade

You can upgrade directly from the **Admin Dashboard** by clicking the **Check for Updates** button in the top-left corner.

The web interface will:
- Check for new versions automatically
- Download and apply updates with one click
- Support rollback if needed

#### Useful Commands

```bash
# Check status
sudo systemctl status sub2api

# View logs
sudo journalctl -u sub2api -f

# Restart service
sudo systemctl restart sub2api

# Uninstall
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | sudo bash -s -- uninstall -y
```

---

### Method 2: Docker Compose (Recommended)

Deploy with Docker Compose, including PostgreSQL and Redis containers.

#### Prerequisites

- Docker 20.10+
- Docker Compose v2+

#### Quick Start (One-Click Deployment)

Use the automated deployment script for easy setup:

```bash
# Create deployment directory
mkdir -p sub2api-deploy && cd sub2api-deploy

# Download and run deployment preparation script
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh | bash

# Start services
docker compose up -d

# View logs
docker compose logs -f sub2api
```

**What the script does:**
- Downloads `docker-compose.local.yml` (saved as `docker-compose.yml`) and `.env.example`
- Generates secure credentials (JWT_SECRET, TOTP_ENCRYPTION_KEY, POSTGRES_PASSWORD)
- Creates `.env` file with auto-generated secrets
- Creates data directories (uses local directories for easy backup/migration)
- Displays generated credentials for your reference

#### Manual Deployment

If you prefer manual setup:

```bash
# 1. Clone the repository
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api/deploy

# 2. Copy environment configuration
cp .env.example .env
chmod 600 .env

# 3. Edit configuration (generate secure passwords)
nano .env
```

**Required configuration in `.env`:**

```bash
# PostgreSQL password (REQUIRED)
POSTGRES_PASSWORD=your_secure_password_here

# JWT Secret (RECOMMENDED - keeps users logged in after restart)
JWT_SECRET=your_jwt_secret_here

# TOTP Encryption Key (RECOMMENDED - preserves 2FA after restart)
TOTP_ENCRYPTION_KEY=your_totp_key_here

# Optional: Admin account
ADMIN_EMAIL=admin@example.com
ADMIN_PASSWORD=your_admin_password

# Optional: Custom port
SERVER_PORT=8080
```

**Generate secure secrets:**
```bash
# Generate JWT_SECRET
openssl rand -hex 32

# Generate TOTP_ENCRYPTION_KEY
openssl rand -hex 32

# Generate POSTGRES_PASSWORD
openssl rand -hex 32
```

```bash
# 4. Create data directories (for local version)
mkdir -p data postgres_data redis_data

# 5. Start all services
# Option A: Local directory version (recommended - easy migration)
docker compose -f docker-compose.local.yml up -d

# Option B: Named volumes version (simple setup)
docker compose up -d

# 6. Check status
docker compose -f docker-compose.local.yml ps

# 7. View logs
docker compose -f docker-compose.local.yml logs -f sub2api
```

#### Deployment Versions

| Version | Data Storage | Migration | Best For |
|---------|-------------|-----------|----------|
| **docker-compose.local.yml** | Local directories | ✅ Easy (tar entire directory) | Production, frequent backups |
| **docker-compose.yml** | Named volumes | ⚠️ Requires docker commands | Simple setup |

**Recommendation:** Use `docker-compose.local.yml` (deployed by script) for easier data management.

#### Access

Open `http://YOUR_SERVER_IP:8080` in your browser.

If admin password was auto-generated, find it in logs:
```bash
docker compose -f docker-compose.local.yml logs sub2api | grep "admin password"
```

#### Upgrade

```bash
# Pull latest image and recreate container
docker compose -f docker-compose.local.yml pull
docker compose -f docker-compose.local.yml up -d
```

#### Easy Migration (Local Directory Version)

When using `docker-compose.local.yml`, migrate to a new server easily:

```bash
# On source server
docker compose -f docker-compose.local.yml down
cd ..
tar czf sub2api-complete.tar.gz sub2api-deploy/

# Transfer to new server
scp sub2api-complete.tar.gz user@new-server:/path/

# On new server
tar xzf sub2api-complete.tar.gz
cd sub2api-deploy/
docker compose -f docker-compose.local.yml up -d
```

#### Useful Commands

```bash
# Stop all services
docker compose -f docker-compose.local.yml down

# Restart
docker compose -f docker-compose.local.yml restart

# View all logs
docker compose -f docker-compose.local.yml logs -f

# Remove all data (caution!)
docker compose -f docker-compose.local.yml down
rm -rf data/ postgres_data/ redis_data/
```

---

### Method 3: Apple container (macOS)

Apple-silicon Macs running macOS 26 can run the full Sub2API, PostgreSQL, and Redis stack with Apple `container` 1.1.0 or newer:

```bash
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api/deploy
./apple-container.sh init
./apple-container.sh up
./apple-container.sh status
```

This is an operator-managed local workflow; Docker Compose remains the recommended production path. See [deploy/APPLE_CONTAINER.md](deploy/APPLE_CONTAINER.md) for lifecycle commands, persistence, upgrades, and runtime limitations.

---

### Method 4: Build from Source

Build and run from source code for development or customization.

#### Prerequisites

- Go 1.21+
- Node.js 18+
- PostgreSQL 15+
- Redis 7+

#### Build Steps

```bash
# 1. Clone the repository
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api

# 2. Install pnpm (if not already installed)
npm install -g pnpm

# 3. Build frontend
cd frontend
pnpm install
pnpm run build
# Output will be in ../backend/internal/web/dist/

# 4. Build backend with embedded frontend
cd ../backend
VERSION="$(./scripts/resolve-version.sh)"
go build -tags embed -ldflags="-X main.Version=${VERSION}" -o sub2api ./cmd/server

# 5. Create configuration file
cp ../deploy/config.example.yaml ./config.yaml

# 6. Edit configuration
nano config.yaml
```

> **Note:** The `-tags embed` flag embeds the frontend into the binary. Without this flag, the binary will not serve the frontend UI.

**Key configuration in `config.yaml`:**

```yaml
server:
  host: "0.0.0.0"
  port: 8080
  mode: "release"

database:
  host: "localhost"
  port: 5432
  user: "postgres"
  password: "your_password"
  dbname: "sub2api"

redis:
  host: "localhost"
  port: 6379
  username: ""
  password: ""

jwt:
  secret: "change-this-to-a-secure-random-string"
  expire_hour: 24

default:
  user_concurrency: 5
  user_balance: 0
  api_key_prefix: "sk-"
  rate_multiplier: 1.0
```

Additional security-related options are available in `config.yaml`:

- `cors.allowed_origins` for CORS allowlist
- `security.url_allowlist` for upstream/pricing/CRS host allowlists
- `security.url_allowlist.enabled` to disable URL validation (use with caution)
- `security.url_allowlist.allow_insecure_http` to allow HTTP URLs when validation is disabled
- `security.url_allowlist.allow_private_hosts` to allow private/local IP addresses
- `security.response_headers.enabled` to enable configurable response header filtering (disabled uses default allowlist)
- `security.csp` to control Content-Security-Policy headers
- `billing.circuit_breaker` to fail closed on billing errors
- `security.trust_forwarded_ip_for_api_key_acl` enables legacy raw forwarded-header takeover (enabled by default for upgrade compatibility); disable it to enforce `server.trusted_proxies`, which should contain only the exact proxy CIDRs that connect directly to Sub2API
- `security.forwarded_client_ip_headers` configures up to 16 third-party CDN client-IP header names; they are checked in order before the built-in headers only while legacy takeover is enabled
- `turnstile.required` to require Turnstile in release mode

Custom client-IP headers can be set in YAML or as a comma-separated environment variable:

```bash
SECURITY_FORWARDED_CLIENT_IP_HEADERS=True-Client-IP,X-CDN-Client-IP
```

Header names are validated, canonicalized, and de-duplicated. The admin security settings can update the list without a restart; new installations persist YAML/environment defaults and existing installations backfill a missing database value. When legacy takeover is disabled, all custom and built-in raw forwarding headers are ignored and Gin uses only `server.trusted_proxies`. While takeover is enabled, firewall the origin to CDN/proxy addresses and make the edge overwrite every trusted client-IP header. See [`deploy/EDGE_SECURITY.md`](deploy/EDGE_SECURITY.md) for the complete migration and trust-boundary rules.

**⚠️ Security Warning: HTTP URL Configuration**

When `security.url_allowlist.enabled=false`, the system performs minimal URL validation and **allows HTTP URLs by default** (dev-friendly mode; Docker Compose deployments use the same default). For production, explicitly tighten this to HTTPS-only:

```yaml
security:
  url_allowlist:
    enabled: false                # Disable allowlist checks
    allow_insecure_http: false    # HTTPS only (recommended for production)
```

**Or via environment variable:**

```bash
SECURITY_URL_ALLOWLIST_ENABLED=false
SECURITY_URL_ALLOWLIST_ALLOW_INSECURE_HTTP=false
```

**Risks of allowing HTTP:**
- API keys and data transmitted in **plaintext** (vulnerable to interception)
- Susceptible to **man-in-the-middle (MITM) attacks**
- **NOT suitable for production** environments

**When to use HTTP:**
- ✅ Development/testing with local servers (http://localhost)
- ✅ Internal networks with trusted endpoints
- ✅ Testing account connectivity before obtaining HTTPS
- ❌ Production environments (use HTTPS only)

**Example error for HTTP URLs when `allow_insecure_http: false` is set:**
```
Invalid base URL: invalid url scheme: http
```

If you disable URL validation or response header filtering, harden your network layer:
- Enforce an egress allowlist for upstream domains/IPs
- Block private/loopback/link-local ranges
- Enforce TLS-only outbound traffic
- Strip sensitive upstream response headers at the proxy

#### OpenAI Responses WebSocket ingress limits

`gateway.openai_ws` bounds the lifetime and aggregate count of client-facing
Responses WebSocket sessions. These safeguards apply independently from
per-turn user and account concurrency slots, which are released between turns.

```yaml
gateway:
  openai_ws:
    # Total time to receive and decompress the first client message.
    client_first_message_timeout_seconds: 30
    # Close a client socket idle between completed turns; 0 disables this safeguard.
    ingress_inter_turn_idle_timeout_seconds: 300
    # Distributed API-key limit for live client ingress sessions; 0 disables it.
    max_ingress_connections_per_api_key: 64
```

The first-message timeout is a total read deadline. Deployments that accept
large contexts or image-heavy requests over slower links can raise it to
120-300 seconds. It expires before HTTP bridge routing, so bridge mode does not
override this limit.

The connection cap is coordinated through Redis using a 60-second lease that
is refreshed every 20 seconds. A process that cannot confirm a lease for a
full lease lifetime closes its local WebSocket rather than continuing outside
the global cap.

Enable the v2 mode router before selecting an account-level WS mode such as
`http_bridge`:

```yaml
gateway:
  openai_ws:
    mode_router_v2_enabled: true
```

Or set `GATEWAY_OPENAI_WS_MODE_ROUTER_V2_ENABLED=true` in the environment.
Use `http_bridge` for client-WebSocket/upstream-HTTP operation when rolling out
or mitigating upstream WebSocket issues.

#### Force OpenAI upstream HTTP/SSE

When an egress proxy or network repeatedly reconnects OpenAI Responses
WebSockets, set the global fallback in the persisted deployment configuration:

```yaml
gateway:
  openai_ws:
    force_http: true
```

For Compose and Apple container deployments, the equivalent `.env` setting is:

```bash
GATEWAY_OPENAI_WS_FORCE_HTTP=true
```

This selects HTTP/SSE for OpenAI upstream Responses traffic that would
otherwise use WebSocket. It does not change the client-facing protocol or force
HTTP/1.1; configure `gateway.openai_http2.enabled` (or
`GATEWAY_OPENAI_HTTP2_ENABLED=false`) separately when a proxy is incompatible
with HTTP/2. Unlike the account-level `http_bridge` mode, this global fallback
takes effect without enabling `mode_router_v2_enabled`. Keep the setting in the
deployment's persisted `.env` or `config.yaml`, rather than inside a running
container, so it is read again after an image update or container recreation.

#### ⚠️ Important: Creating the Admin Account

The initial admin account is **only created via the setup wizard** (served at `http://<host>:8080` on first run). The `default.admin_email` / `default.admin_password` fields in `config.yaml` are **not used** to create it — they exist in the template for historical reasons.

Because step 5 above pre-creates `config.yaml`, the setup wizard will be **skipped on first run**: the server detects an existing config and boots straight into normal mode with an empty `users` table, so the first login attempt fails with `invalid email or password`.

**Two ways to create the admin account:**

1. **Recommended — let the wizard generate `config.yaml`:** Skip step 5 (do not run the `cp`). Start `./sub2api` directly; the setup wizard at `http://localhost:8080` walks you through database, Redis, and admin account setup, then writes `config.yaml` for you.

2. **If you already created `config.yaml`:** Temporarily move it aside so the wizard can trigger on first run, then restore it afterwards:
   ```bash
   mv config.yaml config.yaml.bak
   ./sub2api        # wizard runs at http://localhost:8080 and writes a fresh config.yaml
   # stop the server (Ctrl+C) once the wizard completes, then restore your config:
   mv config.yaml.bak config.yaml
   ./sub2api        # restart in normal mode and log in with the admin you just created
   ```

```bash
# 6. Run the application
./sub2api
```

#### Development Mode

```bash
# Backend (with hot reload)
cd backend
go run ./cmd/server

# Frontend (with hot reload)
cd frontend
pnpm run dev
```

#### Code Generation

When editing `backend/ent/schema`, regenerate Ent + Wire:

```bash
cd backend
go generate ./ent
go generate ./cmd/server
```

---

## Simple Mode

Simple Mode is designed for individual developers or internal teams who want quick access without full SaaS features.

- Enable: Set environment variable `RUN_MODE=simple`
- Difference: Hides SaaS-related features and skips billing process
- Security note: In production, you must also set `SIMPLE_MODE_CONFIRM=true` to allow startup

---

## Asynchronous Image Tasks

Long-running OpenAI/Grok image generation and editing can be submitted through `/v1/images/generations/async` or `/v1/images/edits/async`, then polled at `/v1/images/tasks/{task_id}` without holding a CDN connection open. Adobe Firefly groups are synchronous-only on `/v1/images/generations` and `/v1/images/edits`. See [Asynchronous Image Tasks](docs/ASYNC_IMAGE_TASKS.md) for request and response examples.

---

## Grok / xAI Support

Sub2API supports both Grok subscription accounts through xAI OAuth and standard xAI API-key accounts. Both account types forward OpenAI-compatible Responses traffic to xAI.

### Supported Scope

- Platform name: `grok`
- Account types: OAuth subscription accounts and xAI API-key accounts
- Public Responses targets: `/v1/responses`, `/responses`, and `/backend-api/codex/responses`, forwarded to the Grok subscription proxy for OAuth accounts or `https://api.x.ai/v1/responses` for API-key accounts
- Public Claude-compatible target: `/v1/messages`, converted to xAI Responses and returned as Anthropic Messages output for Claude CLI style clients
- Public Chat Completions targets: `/v1/chat/completions` and `/chat/completions`, forwarded to the account-type-specific xAI upstream
- Codex CLI style Responses WebSocket ingress is accepted on the Responses targets and bridged to xAI HTTP/SSE Responses upstream
- Text models: `grok-4.5`, `grok-4.3`, `grok-build-0.1`, `grok-composer-2.5-fast`, `grok-4.20-0309-reasoning`, `grok-4.20-0309-non-reasoning`, and `grok-4.20-multi-agent-0309`
- Media targets for Grok groups: `/v1/images/generations`, `/images/generations`, `/v1/images/edits`, `/images/edits`, `/v1/videos/generations`, `/videos/generations`, `/v1/videos/edits`, `/videos/edits`, `/v1/videos/extensions`, `/videos/extensions`, `/v1/videos/{request_id}`, and `/videos/{request_id}`. Generation, editing, and extension requests require the group image-generation permission.
- Media models: `grok-imagine`, `grok-imagine-image-quality`, `grok-imagine-image`, `grok-imagine-image-2.0`, `grok-imagine-edit`, `grok-imagine-video`, and `grok-imagine-video-1.5`
- JSON image-edit and video-generation requests accept image references in `image`, `images`, `reference_images`, and `mask` objects. Use `url` for xAI-compatible payloads; the legacy `image_url` field remains accepted and is normalized to `url` before forwarding.
- Out of scope for this provider: TTS, transcription, browser automation, cookies, and Grok web scraping

### OAuth Configuration

The Grok OAuth flow uses PKCE and does not require committing private secrets. The default client details follow the public xAI OAuth flow used by compatible clients, and every value can be overridden by environment variable:

| Variable | Default |
|----------|---------|
| `XAI_OAUTH_CLIENT_ID` | Public xAI OAuth client ID |
| `XAI_OAUTH_SCOPE` | `openid profile email offline_access grok-cli:access api:access` |
| `XAI_OAUTH_REDIRECT_URI` | `http://127.0.0.1:56121/callback` |
| `XAI_OAUTH_AUTHORIZE_URL` | `https://auth.x.ai/oauth2/authorize` |
| `XAI_OAUTH_TOKEN_URL` | `https://auth.x.ai/oauth2/token` |
| `XAI_BASE_URL` | `https://api.x.ai/v1`; runtime-diagnostics override (account `base_url` controls request forwarding) |
| `XAI_GROK_CLI_VERSION` | `0.2.114`; optional override for the client identity sent to `cli-chat-proxy.grok.com`. The pinned value is also the floor: an override below it is dropped |

Administrators can create Grok OAuth or API-key accounts from the dashboard. OAuth authorization and reauthorization are also available through the admin API:

| Endpoint | Purpose |
|----------|---------|
| `POST /api/v1/admin/grok/oauth/auth-url` | Generate an xAI OAuth authorization URL |
| `POST /api/v1/admin/grok/oauth/exchange-code` | Exchange a callback URL, query string, or code for OAuth credentials |
| `POST /api/v1/admin/grok/oauth/refresh-token` | Validate or refresh a Grok refresh token |
| `POST /api/v1/admin/grok/accounts/:id/refresh` | Refresh an existing Grok account |

OAuth credential storage reuses the existing account JSON fields: `access_token`, `refresh_token`, `token_type`, `expires_at`, `base_url`, optional `email`, optional `subscription_tier`, and `entitlement_status`. OAuth inference defaults to `https://cli-chat-proxy.grok.com/v1`; existing OAuth accounts that stored the old `https://api.x.ai/v1` default are redirected to the subscription proxy at runtime. Explicit custom upstreams remain unchanged.

For API-key accounts, select **Grok → API Key** in the create-account dialog. The official base URL defaults to `https://api.x.ai/v1`; credentials use the existing `base_url` and `api_key` account fields. OAuth accounts continue to use the subscription flow above.

### Grok Build CLI Configuration

1. In the Sub2API admin dashboard, add either a `grok` OAuth account and complete xAI authorization, or add a Grok API-key account.
2. Create a Grok group, attach the account to it, then create a Sub2API API key assigned to that group.
3. In the user API-key page, click **Use Key** and select **Grok CLI**. The modal generates the correct file and base URL for macOS/Linux or Windows. It also provides an OpenCode configuration on the **OpenCode** tab.
4. If configuring manually, save the following as `~/.grok/config.toml` (Windows: `%USERPROFILE%\.grok\config.toml`):

```toml
[models]
default = "grok"
web_search = "grok"

[model."grok"]
model = "grok-4.5"
base_url = "https://your-sub2api.example.com/v1"
name = "Grok 4.5"
api_key = "sk-your-sub2api-key"
api_backend = "responses"
context_window = 1000000
supports_backend_search = true
```

Back up an existing `config.toml` before merging the entry. The file contains a Sub2API API key, so keep it private and restrict its permissions where supported. Verify the effective configuration and make a smoke request:

```bash
grok inspect
grok -p "Reply with sub2api-ok" -m grok
```

The `base_url` above is the public Sub2API URL ending in `/v1`, not `api.x.ai` or the internal xAI OAuth proxy URL.

### Usage And Quota Display

xAI quota is passive. Sub2API does not invent subscription quota values; it records whitelisted xAI rate-limit headers from successful or rate-limited upstream responses when xAI sends them. Before the first usable upstream response, the dashboard shows quota as unknown and still displays local Sub2API usage stats.

`401` responses temporarily remove accounts with invalid credentials from scheduling. `403` responses are treated as access or entitlement failures instead of token-refresh loops. `429` responses use `Retry-After` or a short cooldown to temporarily remove the account from scheduling.

New Grok image and video generation requests use a media-specific eligibility check. API-key accounts remain eligible. OAuth accounts with explicit Free or forbidden billing evidence are excluded from new media generation. Missing or malformed observations are probed before dispatch; a successful but incomplete billing response is treated as `billing_inconclusive` and remains eligible for backwards compatibility, because an unknown billing schema is not proof that the account lacks media entitlement. Operators can quarantine a known-bad account with `extra.grok_media_eligible=false` or force-enable a verified account with `true`. Imports run the billing-first quota probe proactively. Chat requests and video status lookups are not affected by this media-only quarantine. If no eligible account remains, the media endpoint returns HTTP `503` with error type `grok_media_no_eligible_account`.

Administrators can override automatic media eligibility through the account create/update API by setting `extra.grok_media_eligible` to `false` (exclude) or `true` (force eligible). On update, set it to `null` to remove the override and return to automatic probe-based behavior; omitting the field preserves the current override. A weekly allowance period alone is not treated as a paid tier signal. Successful image responses must contain at least one actual image output; empty HTTP `200` responses trigger account failover instead of being counted and returned as successful generations.

---

## Antigravity Support

Sub2API supports [Antigravity](https://antigravity.so/) accounts. After authorization, dedicated endpoints are available for Claude and Gemini models.

### Dedicated Endpoints

| Endpoint | Model |
|----------|-------|
| `/antigravity/v1/messages` | Claude models |
| `/antigravity/v1beta/` | Gemini models |

### Claude Code Configuration

```bash
export ANTHROPIC_BASE_URL="http://localhost:8080/antigravity"
export ANTHROPIC_AUTH_TOKEN="sk-xxx"
```

### Hybrid Scheduling Mode

Antigravity accounts support optional **hybrid scheduling**. When enabled, the general endpoints `/v1/messages` and `/v1beta/` will also route requests to Antigravity accounts.

> **⚠️ Warning**: Anthropic Claude and Antigravity Claude **cannot be mixed within the same conversation context**. Use groups to isolate them properly.
## Project Structure

```
sub2api/
├── backend/                  # Go backend service
│   ├── cmd/server/           # Application entry
│   ├── internal/             # Internal modules
│   │   ├── config/           # Configuration
│   │   ├── model/            # Data models
│   │   ├── service/          # Business logic
│   │   ├── handler/          # HTTP handlers
│   │   └── gateway/          # API gateway core
│   └── resources/            # Static resources
│
├── frontend/                 # Vue 3 frontend
│   └── src/
│       ├── api/              # API calls
│       ├── stores/           # State management
│       ├── views/            # Page components
│       └── components/       # Reusable components
│
└── deploy/                   # Deployment files
    ├── docker-compose.yml    # Docker Compose configuration
    ├── .env.example          # Environment variables for Docker Compose
    ├── config.example.yaml   # Full config file for binary deployment
    └── install.sh            # One-click installation script
```

## Star History

<a href="https://star-history.dera.page/#Wei-Shaw/sub2api&Date">
 <picture>
   <source media="(prefers-color-scheme: dark)" srcset="https://star-history.dera.page/svg?repos=Wei-Shaw/sub2api&type=Date&theme=dark" />
   <source media="(prefers-color-scheme: light)" srcset="https://star-history.dera.page/svg?repos=Wei-Shaw/sub2api&type=Date" />
   <img alt="Star History Chart" src="https://star-history.dera.page/svg?repos=Wei-Shaw/sub2api&type=Date" />
 </picture>
</a>

---

## License

This project is licensed under the [GNU Lesser General Public License v3.0](LICENSE) (or later).

Copyright (c) 2026 Wesley Liddick

---

<div align="center">

**If you find this project useful, please give it a star!**

</div>
