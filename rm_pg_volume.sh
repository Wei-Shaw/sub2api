# 清理数据库
docker compose -f deploy/docker-compose.yml down
docker volume rm deploy_postgres_data
docker compose -f deploy/docker-compose.yml up -d
