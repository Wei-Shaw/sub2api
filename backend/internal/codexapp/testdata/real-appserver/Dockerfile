FROM golang:1.27.3-bookworm@sha256:dc0963f32053e4bc451d24d87e3892d42d01021b61f093e98eb95c0352bb84b0 AS test-build

WORKDIR /src
COPY go.mod ./
COPY internal/codexapp ./internal/codexapp
RUN go test -c -tags=codexapp_real_e2e -o /out/codexapp-real-e2e.test ./internal/codexapp

FROM node:22-bookworm-slim@sha256:5ec23b7c622b00547753bc6ffd78085dd868a68aeb41c8e0a1418f033e13d96d

ARG CODEX_VERSION=0.166.0
RUN npm install --global "@openai/codex@${CODEX_VERSION}" \
    && useradd --create-home --uid 10001 codex-e2e \
    && mkdir -p /workspace \
    && chown codex-e2e:codex-e2e /workspace

COPY --from=test-build /out/codexapp-real-e2e.test /usr/local/bin/codexapp-real-e2e.test

USER codex-e2e
WORKDIR /workspace
ENV CODEX_REAL_E2E_BIN=/usr/local/bin/codex

ENTRYPOINT ["/usr/local/bin/codexapp-real-e2e.test", "-test.v", "-test.run", "^TestRealAppServerAgainstFakeResponses$"]

