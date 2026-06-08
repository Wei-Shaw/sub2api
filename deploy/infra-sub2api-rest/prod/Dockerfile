# =============================================================================
# Sub2API Frontend Build Dockerfile (frontend only)
# =============================================================================
# Builds the frontend with pnpm and outputs the static assets to
#   /app/backend/internal/web/dist
# (vite outDir is ../backend/internal/web/dist relative to /app/frontend).
#
# The backend is built natively on the host (go/make), so this image only
# provides the frontend dist to be `docker cp`-ed out of the container.
# =============================================================================

ARG NODE_IMAGE=node:24-alpine
# npm registry mirror to avoid slow/timeout access to registry.npmjs.org.
# Override with an internal mirror if available.
ARG NPM_REGISTRY=https://registry.npmmirror.com

# -----------------------------------------------------------------------------
# Frontend Builder
# -----------------------------------------------------------------------------
FROM ${NODE_IMAGE} AS frontend-builder

ARG NPM_REGISTRY
# corepack downloads pnpm itself from the npm registry, so point it at the mirror too.
ENV COREPACK_NPM_REGISTRY=${NPM_REGISTRY}

WORKDIR /app/frontend

# Install pnpm (pinned to v9 to match the lockfile; pnpm v10+ no longer reads
# the "pnpm.overrides" field from package.json and breaks --frozen-lockfile)
RUN corepack enable && corepack prepare pnpm@9 --activate

# Configure registry + generous timeouts/retries for flaky networks
RUN pnpm config set registry "${NPM_REGISTRY}" && \
    pnpm config set fetch-retries 5 && \
    pnpm config set fetch-retry-mintimeout 20000 && \
    pnpm config set fetch-retry-maxtimeout 120000 && \
    pnpm config set fetch-timeout 300000

# Install dependencies first (better caching)
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy frontend source and build (outputs to /app/backend/internal/web/dist)
COPY frontend/ ./
RUN pnpm run build
