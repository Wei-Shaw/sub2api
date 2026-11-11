#!/bin/bash
# Sub2API 启动脚本
# 依赖：Docker + Docker Compose，配置文件位于 deploy/.env


info "启动 Sub2API 服务..."

docker compose -f deploy/docker-compose.yml up -d

echo ""
echo "服务已启动，访问地址：http://localhost:8080"
echo ""
echo "查看日志：  docker compose -f deploy/docker-compose.yml logs -f sub2api"
echo "停止服务：  ./stop.sh"
