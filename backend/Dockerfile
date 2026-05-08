# =============================================================================
# Sub2API Backend-Only Dockerfile
# =============================================================================
# Stage 1: Build Go backend
# Stage 2: Final minimal runtime image
# =============================================================================

ARG GOLANG_IMAGE=golang:1.26.1-alpine
ARG ALPINE_IMAGE=alpine:3.21

# -----------------------------------------------------------------------------
# Stage 1: Backend Builder
# -----------------------------------------------------------------------------
FROM ${GOLANG_IMAGE} AS backend-builder

ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

# Install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# Copy go mod files first (better caching)
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build static binary
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /app/main \
    ./cmd/server

# -----------------------------------------------------------------------------
# Stage 2: Final Runtime Image
# -----------------------------------------------------------------------------
FROM ${ALPINE_IMAGE}

# Labels
LABEL maintainer="Wei-Shaw <github.com/Wei-Shaw>"
LABEL description="Sub2API - AI API Gateway Platform (backend-only)"
LABEL org.opencontainers.image.source="https://github.com/Wei-Shaw/sub2api"

# Install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    && rm -rf /var/cache/apk/*

# Create non-root user
RUN addgroup -g 1000 sub2api && \
    adduser -u 1000 -G sub2api -s /bin/sh -D sub2api

# Set working directory
WORKDIR /app

# Copy binary with ownership
COPY --from=backend-builder --chown=sub2api:sub2api /app/main /app/main

# Create data directory
RUN mkdir -p /app/data && chown sub2api:sub2api /app/data

# Switch to non-root user
USER sub2api

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O /dev/null http://localhost:${SERVER_PORT:-8080}/health || exit 1

CMD ["/app/main"]
