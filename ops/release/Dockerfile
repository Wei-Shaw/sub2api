ARG BUAGA_BASE_IMAGE
FROM node:22-alpine AS frontend
WORKDIR /src/frontend
RUN corepack enable && corepack prepare pnpm@10.11.0 --activate
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
COPY docs/legal/ /src/docs/legal/
RUN pnpm build

FROM golang:1.27.2-alpine AS backend
WORKDIR /src/backend
COPY backend/ ./
COPY --from=frontend /src/backend/internal/web/dist/ ./internal/web/dist/
ARG VERSION
ARG COMMIT
RUN CGO_ENABLED=0 GOMAXPROCS=2 go build -p 2 -tags embed -trimpath -ldflags "-s -w -X main.Version=${VERSION} -X main.Commit=${COMMIT} -X main.BuildType=custom" -o /out/sub2api ./cmd/server

FROM ${BUAGA_BASE_IMAGE}
USER root
COPY --from=backend /out/sub2api /app/sub2api
COPY --from=frontend /src/backend/internal/web/dist/ /app/.buaga-release/
USER sub2api
