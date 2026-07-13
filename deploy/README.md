# Sub2API Deployment Files

This directory contains files for deploying Sub2API on Linux servers and Kubernetes clusters.

## Deployment Methods

| Method | Best For | Setup Wizard |
|--------|----------|--------------|
| **Docker Compose** | Quick setup, all-in-one | Not needed (auto-setup) |
| **Helm / Kubernetes** | Kubernetes clusters | Not needed (auto-setup) |
| **Binary Install** | Production servers, systemd | Web-based wizard |

## Files

| File | Description |
|------|-------------|
| `docker-compose.yml` | Docker Compose configuration (named volumes) |
| `docker-compose.local.yml` | Docker Compose configuration (local directories, easy migration) |
| `docker-deploy.sh` | **One-click Docker deployment script (recommended)** |
| `helm/sub2api/` | Helm chart for Kubernetes deployment |
| `helm-values.simple.yaml` | Simple Helm values override file |
| `helm-deploy.sh` | Simple Helm deployment script |
| `.env.example` | Docker environment variables template |
| `DOCKER.md` | Docker Hub documentation |
| `install.sh` | One-click binary installation script |
| `install-datamanagementd.sh` | datamanagementd 一键安装脚本 |
| `sub2api.service` | Systemd service unit file |
| `sub2api-datamanagementd.service` | datamanagementd systemd service unit file |
| `DATAMANAGEMENTD_CN.md` | datamanagementd 部署与联动说明（中文） |
| `config.example.yaml` | Example configuration file |

---

## Docker Deployment (Recommended)

### Method 1: One-Click Deployment (Recommended)

Use the automated preparation script for the easiest setup:

```bash
# Download and run the preparation script
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh | bash

# Or download first, then run
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/docker-deploy.sh -o docker-deploy.sh
chmod +x docker-deploy.sh
./docker-deploy.sh
```

**What the script does:**
- Downloads `docker-compose.local.yml` and `.env.example`
- Automatically generates secure secrets (JWT_SECRET, TOTP_ENCRYPTION_KEY, POSTGRES_PASSWORD)
- Creates `.env` file with generated secrets
- Creates necessary data directories (data/, postgres_data/, redis_data/)
- **Displays generated credentials** (POSTGRES_PASSWORD, JWT_SECRET, etc.)

**After running the script:**
```bash
# Start services
docker compose -f docker-compose.local.yml up -d

# View logs
docker compose -f docker-compose.local.yml logs -f sub2api

# If admin password was auto-generated, find it in logs:
docker compose -f docker-compose.local.yml logs sub2api | grep "admin password"

# Access Web UI
# http://localhost:8080
```

### Method 2: Manual Deployment

If you prefer manual control:

```bash
# Clone repository
git clone https://github.com/Wei-Shaw/sub2api.git
cd sub2api/deploy

# Configure environment
cp .env.example .env
nano .env  # Set POSTGRES_PASSWORD and other required variables

# Generate secure secrets (recommended)
JWT_SECRET=$(openssl rand -hex 32)
TOTP_ENCRYPTION_KEY=$(openssl rand -hex 32)
echo "JWT_SECRET=${JWT_SECRET}" >> .env
echo "TOTP_ENCRYPTION_KEY=${TOTP_ENCRYPTION_KEY}" >> .env

# Create data directories
mkdir -p data postgres_data redis_data

# Start all services using local directory version
docker compose -f docker-compose.local.yml up -d

# View logs (check for auto-generated admin password)
docker compose -f docker-compose.local.yml logs -f sub2api

# Access Web UI
# http://localhost:8080
```

### Deployment Version Comparison

| Version | Data Storage | Migration | Best For |
|---------|-------------|-----------|----------|
| **docker-compose.local.yml** | Local directories (./data, ./postgres_data, ./redis_data) | ✅ Easy (tar entire directory) | Production, need frequent backups/migration |
| **docker-compose.yml** | Named volumes (/var/lib/docker/volumes/) | ⚠️ Requires docker commands | Simple setup, don't need migration |

**Recommendation:** Use `docker-compose.local.yml` (deployed by `docker-deploy.sh`) for easier data management and migration.

### How Auto-Setup Works

When using Docker Compose with `AUTO_SETUP=true`:

1. On first run, the system automatically:
   - Connects to PostgreSQL and Redis
   - Applies database migrations (SQL files in `backend/migrations/*.sql`) and records them in `schema_migrations`
   - Generates JWT secret (if not provided)
   - Creates admin account (password auto-generated if not provided)
   - Writes config.yaml

2. No manual Setup Wizard needed - just configure `.env` and start

3. If `ADMIN_PASSWORD` is not set, check logs for the generated password:
   ```bash
   docker compose logs sub2api | grep "admin password"
   ```

### Database Migration Notes (PostgreSQL)

- Migrations are applied in lexicographic order (e.g. `001_...sql`, `002_...sql`).
- `schema_migrations` tracks applied migrations (filename + checksum).
- Migrations are forward-only; rollback requires a DB backup restore or a manual compensating SQL script.

**Verify `users.allowed_groups` → `user_allowed_groups` backfill**

During the incremental GORM→Ent migration, `users.allowed_groups` (legacy `BIGINT[]`) is being replaced by a normalized join table `user_allowed_groups(user_id, group_id)`.

Run this query to compare the legacy data vs the join table:

```sql
WITH old_pairs AS (
  SELECT DISTINCT u.id AS user_id, x.group_id
  FROM users u
  CROSS JOIN LATERAL unnest(u.allowed_groups) AS x(group_id)
  WHERE u.allowed_groups IS NOT NULL
)
SELECT
  (SELECT COUNT(*) FROM old_pairs)           AS old_pair_count,
  (SELECT COUNT(*) FROM user_allowed_groups) AS new_pair_count;
```

### datamanagementd（数据管理）联动

如需启用管理后台“数据管理”功能，请额外部署宿主机 `datamanagementd`：

- 主进程固定探测 `/tmp/sub2api-datamanagement.sock`
- Docker 场景下需把宿主机 Socket 挂载到容器内同路径
- 详细步骤见：`deploy/DATAMANAGEMENTD_CN.md`

### Commands

For **local directory version** (docker-compose.local.yml):

```bash
# Start services
docker compose -f docker-compose.local.yml up -d

# Stop services
docker compose -f docker-compose.local.yml down

# View logs
docker compose -f docker-compose.local.yml logs -f sub2api

# Restart Sub2API only
docker compose -f docker-compose.local.yml restart sub2api

# Update to latest version
docker compose -f docker-compose.local.yml pull
docker compose -f docker-compose.local.yml up -d

# Remove all data (caution!)
docker compose -f docker-compose.local.yml down
rm -rf data/ postgres_data/ redis_data/
```

For **named volumes version** (docker-compose.yml):

```bash
# Start services
docker compose up -d

# Stop services
docker compose down

# View logs
docker compose logs -f sub2api

# Restart Sub2API only
docker compose restart sub2api

# Update to latest version
docker compose pull
docker compose up -d

# Remove all data (caution!)
docker compose down -v
```

### Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `POSTGRES_PASSWORD` | **Yes** | - | PostgreSQL password |
| `JWT_SECRET` | **Recommended** | *(auto-generated)* | JWT secret (fixed for persistent sessions) |
| `TOTP_ENCRYPTION_KEY` | **Recommended** | *(auto-generated)* | TOTP encryption key (fixed for persistent 2FA) |
| `SERVER_PORT` | No | `8080` | Server port |
| `ADMIN_EMAIL` | No | `admin@sub2api.local` | Admin email |
| `ADMIN_PASSWORD` | No | *(auto-generated)* | Admin password |
| `TZ` | No | `Asia/Shanghai` | Timezone |
| `GEMINI_OAUTH_CLIENT_ID` | No | *(builtin)* | Google OAuth client ID (Gemini OAuth). Leave empty to use the built-in Gemini CLI client. |
| `GEMINI_OAUTH_CLIENT_SECRET` | No | *(builtin)* | Google OAuth client secret (Gemini OAuth). Leave empty to use the built-in Gemini CLI client. |
| `GEMINI_OAUTH_SCOPES` | No | *(default)* | OAuth scopes (Gemini OAuth) |
| `GEMINI_QUOTA_POLICY` | No | *(empty)* | JSON overrides for Gemini local quota simulation (Code Assist only). |

See `.env.example` for all available options.

> **Note:** The `docker-deploy.sh` script automatically generates `JWT_SECRET`, `TOTP_ENCRYPTION_KEY`, and `POSTGRES_PASSWORD` for you.

### Easy Migration (Local Directory Version)

When using `docker-compose.local.yml`, all data is stored in local directories, making migration simple:

```bash
# On source server: Stop services and create archive
cd /path/to/deployment
docker compose -f docker-compose.local.yml down
cd ..
tar czf sub2api-complete.tar.gz deployment/

# Transfer to new server
scp sub2api-complete.tar.gz user@new-server:/path/to/destination/

# On new server: Extract and start
tar xzf sub2api-complete.tar.gz
cd deployment/
docker compose -f docker-compose.local.yml up -d
```

Your entire deployment (configuration + data) is migrated!

---

## Helm / Kubernetes Deployment

Use the first-party Helm chart when deploying Sub2API to Kubernetes. The chart defaults mirror the Docker Compose setup:

- Sub2API image: `weishaw/sub2api:latest`
- Bundled PostgreSQL: Bitnami PostgreSQL subchart, rendered as a StatefulSet
- Bundled Redis: Bitnami Redis subchart, rendered as a StatefulSet
- `AUTO_SETUP=true`
- Persistent volumes for Sub2API data, PostgreSQL data, and Redis data
- `/health` probes for the application

The `postgresql:` and `redis:` values are native upstream Bitnami chart values, so operators can extend them for replication, Redis Sentinel, metrics, resources, scheduling, or other Bitnami-supported options. The Sub2API chart only adds small `connection` override blocks for cases where the application must connect to a non-default service or secret.

When bundled PostgreSQL or Redis are enabled, Sub2API connects through Kubernetes Service DNS such as `sub2api-postgresql.sub2api.svc.cluster.local`. Startup also includes an init container that waits for PostgreSQL and Redis ports before the Sub2API container starts, avoiding first-boot crashes while the dependency StatefulSets are still becoming ready.

For non-default dependency topologies, keep the upstream options under the same keys:

```yaml
postgresql:
  architecture: replication
  readReplicas:
    replicaCount: 2

redis:
  architecture: replication
  replica:
    replicaCount: 2
```

If a topology exposes a different service or secret than the chart defaults, set `postgresql.connection.*` or `redis.connection.*` so the Sub2API application uses the correct endpoint. For Redis Sentinel, Bitnami requires `redis.architecture=replication` and can optionally expose a master-only service with `redis.sentinel.masterService.enabled=true` plus its documented RBAC/serviceAccount settings.

Dependency waiting can be tuned or disabled:

```yaml
waitForDependencies:
  enabled: true
  timeoutSeconds: 300
  intervalSeconds: 2
  image:
    repository: busybox
    tag: "1.36"
```

### Install

Use the helper script to start Sub2API with the simple Helm values file:

```bash
cd deploy
chmod +x helm-deploy.sh
./helm-deploy.sh
```

It runs:

```bash
helm upgrade --install sub2api ./helm/sub2api \
  --dependency-update \
  --namespace sub2api \
  --create-namespace \
  -f ./helm-values.simple.yaml
```

Manual install is also supported:

```bash
# From the repository root
helm upgrade --install sub2api deploy/helm/sub2api \
  --namespace sub2api --create-namespace \
  --dependency-update \
  -f deploy/helm-values.simple.yaml

# Local access without ingress
kubectl -n sub2api port-forward svc/sub2api 8080:8080
```

Open `http://127.0.0.1:8080`.

The chart creates stable secrets by default. To read the generated admin password:

```bash
kubectl -n sub2api get secret sub2api-app \
  -o jsonpath='{.data.admin-password}' | base64 -d
```

### Recommended Production Values

Use `deploy/helm-values.simple.yaml` as a small override file instead of editing the full chart `values.yaml`. For production, copy it to a private values file and set fixed credentials before first install:

```yaml
global:
  imageRegistry: "registry.example.com/mirror"
  defaultStorageClass: "fast-storage"
  security:
    allowInsecureImages: true

secrets:
  app:
    jwtSecret: "replace-with-output-of-openssl-rand-hex-32"
    totpEncryptionKey: "replace-with-output-of-openssl-rand-hex-32"
    adminPassword: "replace-with-a-strong-password"
postgresql:
  auth:
    password: "replace-with-a-strong-postgres-password"
redis:
  auth:
    password: "replace-with-a-strong-redis-password"
```

If you keep PostgreSQL or Redis PVCs across `helm uninstall` and reinstall, these fixed passwords are required. Bitnami stores database/cache credentials in the retained data directories; a new random password generated during reinstall will not match the existing data.

`global.imageRegistry` is passed to the Bitnami PostgreSQL/Redis subcharts and is also used by the Sub2API image unless `image.registry` is set. Bitnami charts block unrecognized mirrored images unless `global.security.allowInsecureImages=true` is set. If your mirror rewrites image names, set the upstream image fields directly:

```yaml
image:
  registry: crpi-dgkl9khr1943eg60.cn-hangzhou.personal.cr.aliyuncs.com
  repository: hq-docker-mirror/weishaw-sub2api

postgresql:
  image:
    registry: crpi-dgkl9khr1943eg60.cn-hangzhou.personal.cr.aliyuncs.com
    repository: hq-docker-mirror/bitnami-postgresql
    tag: latest

redis:
  image:
    registry: crpi-dgkl9khr1943eg60.cn-hangzhou.personal.cr.aliyuncs.com
    repository: hq-docker-mirror/bitnami-redis
    tag: latest
```

Then install or upgrade with:

```bash
helm upgrade --install sub2api deploy/helm/sub2api \
  --namespace sub2api --create-namespace \
  -f deploy/helm-values.production.yaml
```

### Existing Secrets

If your platform manages secrets separately, point the chart at existing Kubernetes secrets:

```yaml
secrets:
  app:
    existingSecret: sub2api-app-secret
postgresql:
  auth:
    existingSecret: sub2api-postgres-secret
    secretKeys:
      userPasswordKey: password
      adminPasswordKey: postgres-password
redis:
  auth:
    existingSecret: sub2api-redis-secret
    existingSecretPasswordKey: redis-password
```

For bundled Bitnami PostgreSQL/Redis, do not put these two secrets under
`secrets.postgresql.existingSecret` or `secrets.redis.existingSecret`. Those
legacy keys are only for external PostgreSQL/Redis fallback secrets when the
bundled dependency is disabled.

Create the secrets before installing the chart:

```bash
kubectl create namespace sub2api --dry-run=client -o yaml | kubectl apply -f -

kubectl -n sub2api create secret generic sub2api-app-secret \
  --from-literal=jwt-secret="$(openssl rand -hex 32)" \
  --from-literal=totp-encryption-key="$(openssl rand -hex 32)" \
  --from-literal=admin-password='replace-with-a-strong-admin-password'

kubectl -n sub2api create secret generic sub2api-postgres-secret \
  --from-literal=password='replace-with-a-strong-postgres-password' \
  --from-literal=postgres-password='replace-with-a-strong-postgres-admin-password'

kubectl -n sub2api create secret generic sub2api-redis-secret \
  --from-literal=redis-password='replace-with-a-strong-redis-password'
```

The manually created secrets are not owned by the Helm release, so `helm uninstall`
will not delete them. This is the recommended production pattern when PVCs are
retained across uninstall/reinstall, because the retained PostgreSQL/Redis data
must keep using the same passwords.

Required keys:

| Secret | Required Keys |
|--------|---------------|
| `secrets.app.existingSecret` | `jwt-secret`, `totp-encryption-key`, `admin-password` |
| `postgresql.auth.existingSecret` | `password` by default, or `postgresql.auth.secretKeys.userPasswordKey` |
| `redis.auth.existingSecret` | `redis-password` by default, or `redis.auth.existingSecretPasswordKey` |

For external PostgreSQL or Redis, the legacy `secrets.postgresql.existingSecret` and `secrets.redis.existingSecret` keys are still supported as fallback secrets.

For bundled PostgreSQL/Redis, prefer either fixed `postgresql.auth.password` / `redis.auth.password` values or upstream Bitnami `existingSecret` settings before the first production install.

If you use `postgresql.auth.existingSecret` or `redis.auth.existingSecret`, keep those secrets when retaining PVCs.

### Mihomo Sidecar Proxy

For clusters that need outbound proxy access, the chart can run a mihomo
sidecar in the same Pod as Sub2API. This recommended mode does not use TUN and
does not inject `HTTP_PROXY` or `HTTPS_PROXY`.

Create a Kubernetes Secret that contains `config.yaml`:

```bash
kubectl -n sub2api create secret generic sub2api-mihomo-config \
  --from-file=config.yaml=./mihomo-config.yaml
```

Recommended mihomo listener settings:

```yaml
mixed-port: 7890
allow-lan: false
bind-address: 127.0.0.1
external-controller: 127.0.0.1:9090
```

Enable the sidecar:

```yaml
mihomo:
  enabled: true
  config:
    existingSecret: sub2api-mihomo-config
    key: config.yaml
```

For Sub2API's built-in external downloads, set `app.update.proxyURL` to the
local mihomo listener. This is used by GitHub release checks and remote pricing
data downloads, including the model pricing list fetched from GitHub:

```yaml
app:
  update:
    proxyURL: http://127.0.0.1:7890
```

For provider/account traffic, add a proxy in the Sub2API admin UI and assign it
to the accounts or providers that need overseas access:

```text
protocol: http
host: 127.0.0.1
port: 7890
```

If using SOCKS mode, use `socks5h` in Sub2API so DNS is resolved through the
proxy. Keep mihomo subscription URLs and node credentials in the Secret, not in
Helm values.

TUN mode is usually unnecessary for this deployment. If you enable TUN in a
custom mihomo config, the Pod also needs cluster-specific permissions and
devices such as `/dev/net/tun` and `NET_ADMIN`; without them mihomo can start
the mixed proxy but fail the TUN listener.

### External PostgreSQL and Redis

Disable bundled services when using managed databases or shared cluster services:

```yaml
postgresql:
  enabled: false
externalPostgresql:
  host: postgres.example.internal
  port: 5432
  username: sub2api
  password: "replace-with-postgres-password"
  database: sub2api
  sslMode: require

redis:
  enabled: false
externalRedis:
  host: redis.example.internal
  port: 6379
  password: "replace-with-redis-password"
```

### Ingress

Ingress is disabled by default and does not assume a specific controller:

```yaml
ingress:
  enabled: true
  className: nginx
  hosts:
    - host: sub2api.example.com
      paths:
        - path: /
          pathType: Prefix
  tls:
    - secretName: sub2api-tls
      hosts:
        - sub2api.example.com
```

### Persistence

PVCs are enabled by default:

```yaml
global:
  defaultStorageClass: fast-storage
persistence:
  size: 10Gi
postgresql:
  primary:
    persistence:
      size: 20Gi
redis:
  master:
    persistence:
      size: 8Gi
```

To use an existing claim:

```yaml
persistence:
  existingClaim: sub2api-data
```

To run with ephemeral storage for testing only:

```yaml
persistence:
  enabled: false
postgresql:
  primary:
    persistence:
      enabled: false
redis:
  master:
    persistence:
      enabled: false
```

> Warning: disabling persistence uses `emptyDir`; data will not survive pod rescheduling.

### Upgrade and Uninstall

```bash
# Upgrade
helm upgrade sub2api deploy/helm/sub2api -n sub2api --dependency-update -f deploy/helm-values.simple.yaml

# Uninstall release
helm uninstall sub2api -n sub2api

# Remove PVCs only if you intentionally want to delete stored data
kubectl -n sub2api delete pvc -l app.kubernetes.io/instance=sub2api
```

If you uninstall but keep PVCs, reinstall with the same `postgresql.auth.password` and `redis.auth.password` values. If an older release used random generated passwords and the Bitnami secrets were deleted, you must recover the original passwords or reset them inside the retained PostgreSQL/Redis data before reinstalling.

### Helm Validation

```bash
helm dependency update deploy/helm/sub2api
helm lint deploy/helm/sub2api
helm template sub2api deploy/helm/sub2api
```

---

## Gemini OAuth Configuration

Sub2API supports three methods to connect to Gemini:

### Method 1: Code Assist OAuth (Recommended for GCP Users)

**No configuration needed** - always uses the built-in Gemini CLI OAuth client (public).

1. Leave `GEMINI_OAUTH_CLIENT_ID` and `GEMINI_OAUTH_CLIENT_SECRET` empty
2. In the Admin UI, create a Gemini OAuth account and select **"Code Assist"** type
3. Complete the OAuth flow in your browser

> Note: Even if you configure `GEMINI_OAUTH_CLIENT_ID` / `GEMINI_OAUTH_CLIENT_SECRET` for AI Studio OAuth,
> Code Assist OAuth will still use the built-in Gemini CLI client.

**Requirements:**
- Google account with access to Google Cloud Platform
- A GCP project (auto-detected or manually specified)

**How to get Project ID (if auto-detection fails):**
1. Go to [Google Cloud Console](https://console.cloud.google.com/)
2. Click the project dropdown at the top of the page
3. Copy the Project ID (not the project name) from the list
4. Common formats: `my-project-123456` or `cloud-ai-companion-xxxxx`

### Method 2: AI Studio OAuth (For Regular Google Accounts)

Requires your own OAuth client credentials.

**Step 1: Create OAuth Client in Google Cloud Console**

1. Go to [Google Cloud Console - Credentials](https://console.cloud.google.com/apis/credentials)
2. Create a new project or select an existing one
3. **Enable the Generative Language API:**
   - Go to "APIs & Services" → "Library"
   - Search for "Generative Language API"
   - Click "Enable"
4. **Configure OAuth Consent Screen** (if not done):
   - Go to "APIs & Services" → "OAuth consent screen"
   - Choose "External" user type
   - Fill in app name, user support email, developer contact
   - Add scopes: `https://www.googleapis.com/auth/generative-language.retriever` (and optionally `https://www.googleapis.com/auth/cloud-platform`)
   - Add test users (your Google account email)
5. **Create OAuth 2.0 credentials:**
   - Go to "APIs & Services" → "Credentials"
   - Click "Create Credentials" → "OAuth client ID"
   - Application type: **Web application** (or **Desktop app**)
   - Name: e.g., "Sub2API Gemini"
   - Authorized redirect URIs: Add `http://localhost:1455/auth/callback`
6. Copy the **Client ID** and **Client Secret**
7. **⚠️ Publish to Production (IMPORTANT):**
   - Go to "APIs & Services" → "OAuth consent screen"
   - Click "PUBLISH APP" to move from Testing to Production
   - **Testing mode limitations:**
     - Only manually added test users can authenticate (max 100 users)
     - Refresh tokens expire after 7 days
     - Users must be re-added periodically
   - **Production mode:** Any Google user can authenticate, tokens don't expire
   - Note: For sensitive scopes, Google may require verification (demo video, privacy policy)

**Step 2: Configure Environment Variables**

```bash
GEMINI_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
GEMINI_OAUTH_CLIENT_SECRET=GOCSPX-your-client-secret

# 可选：如需使用 Gemini CLI 内置 OAuth Client（Code Assist / Google One）
# 安全说明：本仓库不会内置该 client_secret，请在运行环境通过环境变量注入。
# GEMINI_CLI_OAUTH_CLIENT_SECRET=GOCSPX-your-built-in-secret
```

**Step 3: Create Account in Admin UI**

1. Create a Gemini OAuth account and select **"AI Studio"** type
2. Complete the OAuth flow
   - After consent, your browser will be redirected to `http://localhost:1455/auth/callback?code=...&state=...`
   - Copy the full callback URL (recommended) or just the `code` and paste it back into the Admin UI

### Method 3: API Key (Simplest)

1. Go to [Google AI Studio](https://aistudio.google.com/app/apikey)
2. Click "Create API key"
3. In Admin UI, create a Gemini **API Key** account
4. Paste your API key (starts with `AIza...`)

### Comparison Table

| Feature | Code Assist OAuth | AI Studio OAuth | API Key |
|---------|-------------------|-----------------|---------|
| Setup Complexity | Easy (no config) | Medium (OAuth client) | Easy |
| GCP Project Required | Yes | No | No |
| Custom OAuth Client | No (built-in) | Yes (required) | N/A |
| Rate Limits | GCP quota | Standard | Standard |
| Best For | GCP developers | Regular users needing OAuth | Quick testing |

---

## Binary Installation

For production servers using systemd.

### One-Line Installation

```bash
curl -sSL https://raw.githubusercontent.com/Wei-Shaw/sub2api/main/deploy/install.sh | sudo bash
```

### Manual Installation

1. Download the latest release from [GitHub Releases](https://github.com/Wei-Shaw/sub2api/releases)
2. Extract and copy the binary to `/opt/sub2api/`
3. Copy `sub2api.service` to `/etc/systemd/system/`
4. Run:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl enable sub2api
   sudo systemctl start sub2api
   ```
5. Open the Setup Wizard in your browser to complete configuration

### Commands

```bash
# Install
sudo ./install.sh

# Upgrade
sudo ./install.sh upgrade

# Uninstall
sudo ./install.sh uninstall
```

### Service Management

```bash
# Start the service
sudo systemctl start sub2api

# Stop the service
sudo systemctl stop sub2api

# Restart the service
sudo systemctl restart sub2api

# Check status
sudo systemctl status sub2api

# View logs
sudo journalctl -u sub2api -f

# Enable auto-start on boot
sudo systemctl enable sub2api
```

### Configuration

#### Server Address and Port

During installation, you will be prompted to configure the server listen address and port. These settings are stored in the systemd service file as environment variables.

To change after installation:

1. Edit the systemd service:
   ```bash
   sudo systemctl edit sub2api
   ```

2. Add or modify:
   ```ini
   [Service]
   Environment=SERVER_HOST=0.0.0.0
   Environment=SERVER_PORT=3000
   ```

3. Reload and restart:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl restart sub2api
   ```

#### Gemini OAuth Configuration

If you need to use AI Studio OAuth for Gemini accounts, add the OAuth client credentials to the systemd service file:

1. Edit the service file:
   ```bash
   sudo nano /etc/systemd/system/sub2api.service
   ```

2. Add your OAuth credentials in the `[Service]` section (after the existing `Environment=` lines):
   ```ini
   Environment=GEMINI_OAUTH_CLIENT_ID=your-client-id.apps.googleusercontent.com
   Environment=GEMINI_OAUTH_CLIENT_SECRET=GOCSPX-your-client-secret
   ```

   如需使用“内置 Gemini CLI OAuth Client”（Code Assist / Google One），还需要注入：
   ```ini
   Environment=GEMINI_CLI_OAUTH_CLIENT_SECRET=GOCSPX-your-built-in-secret
   ```

3. Reload and restart:
   ```bash
   sudo systemctl daemon-reload
   sudo systemctl restart sub2api
   ```

> **Note:** Code Assist OAuth does not require any configuration - it uses the built-in Gemini CLI client.
> See the [Gemini OAuth Configuration](#gemini-oauth-configuration) section above for detailed setup instructions.

#### Application Configuration

The main config file is at `/etc/sub2api/config.yaml` (created by Setup Wizard).

### Prerequisites

- Linux server (Ubuntu 20.04+, Debian 11+, CentOS 8+, etc.)
- PostgreSQL 14+
- Redis 6+
- systemd

### Directory Structure

```
/opt/sub2api/
├── sub2api              # Main binary
├── sub2api.backup       # Backup (after upgrade)
└── data/                # Runtime data

/etc/sub2api/
└── config.yaml          # Configuration file
```

---

## Troubleshooting

### Docker

For **local directory version**:

```bash
# Check container status
docker compose -f docker-compose.local.yml ps

# View detailed logs
docker compose -f docker-compose.local.yml logs --tail=100 sub2api

# Check database connection
docker compose -f docker-compose.local.yml exec postgres pg_isready

# Check Redis connection
docker compose -f docker-compose.local.yml exec redis redis-cli ping

# Restart all services
docker compose -f docker-compose.local.yml restart

# Check data directories
ls -la data/ postgres_data/ redis_data/
```

For **named volumes version**:

```bash
# Check container status
docker compose ps

# View detailed logs
docker compose logs --tail=100 sub2api

# Check database connection
docker compose exec postgres pg_isready

# Check Redis connection
docker compose exec redis redis-cli ping

# Restart all services
docker compose restart
```

### Binary Install

```bash
# Check service status
sudo systemctl status sub2api

# View recent logs
sudo journalctl -u sub2api -n 50

# Check config file
sudo cat /etc/sub2api/config.yaml

# Check PostgreSQL
sudo systemctl status postgresql

# Check Redis
sudo systemctl status redis
```

### Common Issues

1. **Port already in use**: Change `SERVER_PORT` in `.env` or systemd config
2. **Database connection failed**: Check PostgreSQL is running and credentials are correct
3. **Redis connection failed**: Check Redis is running and password is correct
4. **Permission denied**: Ensure proper file ownership for binary install

---

## TLS Fingerprint Configuration

Sub2API supports TLS fingerprint simulation to make requests appear as if they come from the official Claude CLI (Node.js client).

> **💡 Tip:** Visit **[tls.sub2api.org](https://tls.sub2api.org/)** to get TLS fingerprint information for different devices and browsers.

### Default Behavior

- Built-in `claude_cli_v2` profile simulates Node.js 20.x + OpenSSL 3.x
- JA3 Hash: `1a28e69016765d92e3b381168d68922c`
- JA4: `t13d5911h1_a33745022dd6_1f22a2ca17c4`
- Profile selection: `accountID % profileCount`

### Configuration

```yaml
gateway:
  tls_fingerprint:
    enabled: true  # Global switch
    profiles:
      # Simple profile (uses default cipher suites)
      profile_1:
        name: "Profile 1"

      # Profile with custom cipher suites (use compact array format)
      profile_2:
        name: "Profile 2"
        cipher_suites: [4866, 4867, 4865, 49199, 49195, 49200, 49196]
        curves: [29, 23, 24]
        point_formats: 0

      # Another custom profile
      profile_3:
        name: "Profile 3"
        cipher_suites: [4865, 4866, 4867, 49199, 49200]
        curves: [29, 23, 24, 25]
```

### Profile Fields

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Display name (required) |
| `cipher_suites` | []uint16 | Cipher suites in decimal. Empty = default |
| `curves` | []uint16 | Elliptic curves in decimal. Empty = default |
| `point_formats` | []uint8 | EC point formats. Empty = default |

### Common Values Reference

**Cipher Suites (TLS 1.3):** `4865` (AES_128_GCM), `4866` (AES_256_GCM), `4867` (CHACHA20)

**Cipher Suites (TLS 1.2):** `49195`, `49196`, `49199`, `49200` (ECDHE variants)

**Curves:** `29` (X25519), `23` (P-256), `24` (P-384), `25` (P-521)
