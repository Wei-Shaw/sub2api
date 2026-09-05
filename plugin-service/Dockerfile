ARG GOLANG_IMAGE=golang:1.26.4-alpine
ARG ALPINE_IMAGE=alpine:3.21
ARG GOPROXY=https://goproxy.cn,direct
ARG GOSUMDB=sum.golang.google.cn

FROM ${GOLANG_IMAGE} AS builder

ARG GOPROXY
ARG GOSUMDB

ENV GOPROXY=${GOPROXY}
ENV GOSUMDB=${GOSUMDB}

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app/plugin-service

COPY plugin-service/go.mod ./
RUN go mod download

COPY plugin-service/ ./

RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /app/plugin-service-server \
    ./cmd/server

FROM ${ALPINE_IMAGE}

RUN apk add --no-cache ca-certificates tzdata wget && rm -rf /var/cache/apk/*

RUN addgroup -g 1000 plugin && \
    adduser -u 1000 -G plugin -s /bin/sh -D plugin

WORKDIR /app

COPY --from=builder --chown=plugin:plugin /app/plugin-service-server /app/plugin-service-server
COPY --from=builder --chown=plugin:plugin /app/plugin-service/plugins /app/plugins

USER plugin

EXPOSE 8091

HEALTHCHECK --interval=30s --timeout=10s --start-period=10s --retries=3 \
    CMD wget -q -T 5 -O - http://localhost:${PLUGIN_SERVER_PORT:-8091}/healthz >/dev/null || exit 1

CMD ["/app/plugin-service-server"]
