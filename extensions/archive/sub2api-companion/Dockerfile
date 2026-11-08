FROM golang:1.26.5-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/companion ./cmd/companion

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata && addgroup -S companion && adduser -S -G companion companion
WORKDIR /app
COPY --from=build /out/companion /app/companion
COPY config /app/config
RUN mkdir -p /data && chown -R companion:companion /data /app
USER companion
EXPOSE 8090
ENTRYPOINT ["/app/companion"]
