#!/usr/bin/env bash
set -euo pipefail

WORK_DIR="/root/workspace/sub2api"
NGINX_CONF="/etc/nginx/sites-available/sub2api"

log() { echo "[setup-nginx] $*"; }

# 1. 检查前端构建产物
if [ ! -d "${WORK_DIR}/frontend/dist" ]; then
    echo "ERROR: frontend/dist not found!" >&2
    echo "Build locally first:" >&2
    echo "  cd frontend && pnpm install && pnpm run build" >&2
    echo "Then upload frontend/dist to ${WORK_DIR}/frontend/dist" >&2
    exit 1
fi

# 2. 安装 Nginx
log "Installing Nginx..."
apt-get update -qq
apt-get install -y -qq nginx
rm -f /etc/nginx/sites-enabled/default

# 3. 写入配置
cat > "${NGINX_CONF}" <<'EOF'
server {
    listen 80;
    server_name _;

    location / {
        root /root/workspace/sub2api/frontend/dist;
        try_files $uri $uri/ /index.html;
        expires 1d;
    }

    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 86400;
    }

    location /v1/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_buffering off;
        proxy_read_timeout 86400;
    }
}
EOF

ln -sf "${NGINX_CONF}" /etc/nginx/sites-enabled/sub2api

# 4. 启动 Nginx
nginx -t
systemctl restart nginx
systemctl enable nginx

log "Nginx started on port 80"
log "Access: http://<ECS_IP>/"
