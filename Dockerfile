# =============================================================================
# Sub2API Multi-Stage Dockerfile
# =============================================================================
# Stage 1: Build frontend
# Stage 2: Build Go backend with embedded frontend
# Stage 3: Final minimal image
# =============================================================================

ARG NODE_IMAGE=node:24-alpine
ARG GOLANG_IMAGE=golang:1.26.3-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG POSTGRES_IMAGE=postgres:18-alpine
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

# -----------------------------------------------------------------------------
# Stage 1: Frontend Builder
# -----------------------------------------------------------------------------
FROM ${NODE_IMAGE} AS frontend-builder

WORKDIR /app/frontend

# Install pnpm
RUN corepack enable && corepack prepare pnpm@9 --activate

# Install dependencies first (better caching)
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

# Copy frontend source and build
COPY frontend/ ./
RUN pnpm run build

# -----------------------------------------------------------------------------
# Stage 2: Backend Builder
# -----------------------------------------------------------------------------
FROM ${GOLANG_IMAGE} AS backend-builder

# Build arguments for version info (set by CI)
ARG VERSION=
ARG COMMIT=docker
ARG DATE
ARG GOPROXY
ARG GOSUMDB

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app/backend

# Copy plugin-sdk module first (referenced via replace directive in backend/go.mod)
# Only go.mod/go.sum + source structure are needed for `go mod download` to resolve;
# placed before the backend mod download so layer caching works for typical edits.
COPY plugin-sdk/go.mod plugin-sdk/go.sum /app/plugin-sdk/

# Copy backend go.mod files (better caching)
COPY backend/go.mod backend/go.sum ./
RUN go mod download

# Copy plugin-sdk source (after mod download so source-only edits do not bust cache)
COPY plugin-sdk/ /app/plugin-sdk/

# Copy backend source
COPY backend/ ./

# Copy frontend dist from previous stage (must be after backend copy to avoid being overwritten)
COPY --from=frontend-builder /app/backend/internal/web/dist ./internal/web/dist

# Build the binary (BuildType=release for CI builds, embed frontend)
# Version precedence: build arg VERSION > cmd/server/VERSION
RUN VERSION_VALUE="${VERSION}" && \
    if [ -z "${VERSION_VALUE}" ]; then VERSION_VALUE="$(tr -d '\r\n' < ./cmd/server/VERSION)"; fi && \
    DATE_VALUE="${DATE:-$(date -u +%Y-%m-%dT%H:%M:%SZ)}" && \
    CGO_ENABLED=0 GOOS=linux go build \
    -tags embed \
    -ldflags="-s -w -X main.Version=${VERSION_VALUE} -X main.Commit=${COMMIT} -X main.Date=${DATE_VALUE} -X main.BuildType=release" \
    -trimpath \
    -o /app/sub2api \
    ./cmd/server

# -----------------------------------------------------------------------------
# Stage 2.4: Plugin Frontend Builder
# -----------------------------------------------------------------------------
# Iterates over every plugins/<name>/frontend/ that has a package.json and
# runs `pnpm install --frozen-lockfile && pnpm build`. Output dist/ is copied
# to /out/plugin-frontend/<name>/dist so the plugin-builder stage can place it
# back into /src/plugins/<name>/frontend/dist/ before `go build` so that
# `//go:embed all:frontend/dist` finds the real bundle.
#
# Why a separate stage:
#   - Plugin frontends are independent npm projects (independent lockfile,
#     independent vite.config). They are NOT part of root pnpm-workspace.yaml
#     to keep the host frontend's pnpm-lock free of plugin-side dependencies.
#   - Cache mount on /pnpm-store keeps repeat builds fast.
FROM ${NODE_IMAGE} AS plugin-frontend-builder

WORKDIR /src

RUN corepack enable && corepack prepare pnpm@9 --activate
ENV PNPM_HOME=/pnpm-store

# Copy plugin source (deps + src). Host frontend node_modules also needs to be
# accessible because plugin vite config falls back to host frontend/node_modules
# for transitive deps (e.g. @tanstack/vue-virtual used by host common DataTable
# that the plugin reuses via vite alias).
COPY frontend/ /src/frontend/
COPY plugins/ /src/plugins/

# First install host frontend deps once so plugin builds can resolve into them.
# Reuse the same pnpm cache mount as the plugin-frontend installs below.
RUN --mount=type=cache,id=pnpm-store,target=/pnpm-store \
    set -eux; \
    cd /src/frontend; \
    pnpm install --frozen-lockfile

# Iterate over plugins/*/frontend/, install + build each that has a package.json.
RUN --mount=type=cache,id=pnpm-store,target=/pnpm-store \
    set -eux; \
    mkdir -p /out/plugin-frontend; \
    for dir in /src/plugins/*/frontend/; do \
      [ -f "$dir/package.json" ] || { echo "skip $dir (no package.json)"; continue; }; \
      name="$(basename "$(dirname "$dir")")"; \
      echo "=== plugin frontend: $name ==="; \
      cd "$dir"; \
      pnpm install --frozen-lockfile; \
      pnpm run build; \
      mkdir -p "/out/plugin-frontend/$name"; \
      cp -r dist "/out/plugin-frontend/$name/"; \
    done; \
    ls -laR /out/plugin-frontend/ || true

# -----------------------------------------------------------------------------
# Stage 2.5: Built-in Plugin Binaries
# -----------------------------------------------------------------------------
# Builds every official plugin under plugins/* into a Linux binary that lands
# at /out/plugins/<name>/<name>. The final image copies this whole directory
# into /app/plugins, which is the read-only "builtin plugin dir". Plugins
# discovered there are auto-enabled on first start (see PluginManager).
FROM ${GOLANG_IMAGE} AS plugin-builder

ARG GOPROXY
ARG GOSUMDB
ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}
ENV CGO_ENABLED=0 GOOS=linux

RUN apk add --no-cache git

WORKDIR /src

# plugin-sdk and plugin source code (each plugin has its own go.mod with a
# local replace pointing at ../../plugin-sdk).
COPY plugin-sdk/ /src/plugin-sdk/
COPY plugins/ /src/plugins/

# Pull the pre-built plugin frontend dist from plugin-frontend-builder so
# `//go:embed all:frontend/dist` in each plugin's main.go finds real bundle
# files. The for loop overwrites the .keep placeholder with the real dist.
COPY --from=plugin-frontend-builder /out/plugin-frontend/ /tmp/plugin-frontend/
RUN set -eux; \
    if [ -d /tmp/plugin-frontend ]; then \
      for d in /tmp/plugin-frontend/*/; do \
        n="$(basename "$d")"; \
        echo "=== injecting frontend dist for plugin: $n ==="; \
        rm -rf "/src/plugins/$n/frontend/dist"; \
        mkdir -p "/src/plugins/$n/frontend/dist"; \
        cp -r "$d/dist/." "/src/plugins/$n/frontend/dist/"; \
      done; \
    fi

# Build every plugin under plugins/<name>/. The convention is that each plugin
# directory contains a main package buildable from its root.
RUN set -eux; \
    mkdir -p /out/plugins; \
    for dir in /src/plugins/*/; do \
      name="$(basename "$dir")"; \
      if [ ! -f "$dir/go.mod" ]; then echo "skip $name (no go.mod)"; continue; fi; \
      echo "=== building plugin: $name ==="; \
      cd "$dir"; \
      mkdir -p /out/plugins/"$name"; \
      go build -trimpath -ldflags='-s -w' -o /out/plugins/"$name"/"$name" ./; \
    done; \
    ls -la /out/plugins/

# -----------------------------------------------------------------------------
# Stage 3: PostgreSQL Client (version-matched with docker-compose)
# -----------------------------------------------------------------------------
FROM ${POSTGRES_IMAGE} AS pg-client

# -----------------------------------------------------------------------------
# Stage 4: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${ALPINE_IMAGE}

# Labels
LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API - AI API Gateway Platform"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    su-exec \
    libpq \
    zstd-libs \
    lz4-libs \
    krb5-libs \
    libldap \
    libedit \
    && rm -rf /var/cache/apk/*

# Copy pg_dump and psql from the same postgres image used in docker-compose
# This ensures version consistency between backup tools and the database server
COPY --from=pg-client /usr/local/bin/pg_dump /usr/local/bin/pg_dump
COPY --from=pg-client /usr/local/bin/psql /usr/local/bin/psql
COPY --from=pg-client /usr/local/lib/libpq.so.5* /usr/local/lib/

# Create non-root user
RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

# Set working directory
WORKDIR /app

# Copy binary/resources with ownership to avoid extra full-layer chown copy
COPY --from=backend-builder --chown=sub2api:sub2api /app/sub2api /app/sub2api
COPY --from=backend-builder --chown=sub2api:sub2api /app/backend/resources /app/resources

# Built-in plugins (read-only, ships with the image). PluginManager defaults
# plugins.builtin_dir to /app/plugins and auto-enables anything found here.
COPY --from=plugin-builder --chown=sub2api:sub2api /out/plugins /app/plugins

# Create data directory (user-installed plugins, if any, live under data/plugins)
RUN mkdir -p /app/data && chown sub2api:sub2api /app/data

# Copy entrypoint script (fixes volume permissions then drops to sub2api)
COPY deploy/docker-entrypoint.sh /app/docker-entrypoint.sh
RUN chmod +x /app/docker-entrypoint.sh

# Expose port (can be overridden by SERVER_PORT env var)
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/health || exit 1

# Run the application (entrypoint fixes /app/data ownership then execs as sub2api)
ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["/app/sub2api"]
