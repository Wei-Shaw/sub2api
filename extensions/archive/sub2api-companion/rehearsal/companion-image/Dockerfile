FROM sub2api-companion:local

USER root
RUN mkdir -p /app/config
COPY --chown=100:101 config/pricing.json /app/config/pricing.json
USER 100:101
