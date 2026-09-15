#!/bin/bash
# 等价上游 systemd Restart=always：server 以任何方式退出（含
# POST /admin/system/restart 触发的 os.Exit(0)）都自动拉起。
# 拉起的是磁盘上的最新 bin/server —— 更新流程: go build 后 kill/restart 即可。
cd "$(dirname "$0")/.."
set -a; source /tmp/sub2api-run.env; set +a
while true; do
  echo "[supervisor] starting bin/server at $(date -Is)" >> /tmp/sub2api-server.log
  ./bin/server >> /tmp/sub2api-server.log 2>&1
  rc=$?
  echo "[supervisor] bin/server exited rc=$rc at $(date -Is); respawn in 1s" >> /tmp/sub2api-server.log
  sleep 1
done
