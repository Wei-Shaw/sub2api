#!/bin/bash
# 为所有迁移脚本添加可执行权限

chmod +x aws-backup.sh
chmod +x vultr-deploy.sh
chmod +x restore-data.sh

echo "✅ 所有脚本已添加可执行权限"
ls -lh *.sh
