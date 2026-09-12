FROM sub2api-prompt-audit-frontend:20260912
RUN apk add --no-cache chromium && npm install --prefix /tmp/browser playwright-core
