# Router 生产备份程序

> 这是旧主机时期的版本，路径、容器名和主机已过期。当前导出脚本在 `ssh tmhhost` 的 `/srv/nanafox-ops/backup/export-backup.py`，源码在私有仓库 `nana-fox/nanafox-ops`。

这两份是已安装程序的版本化参考，修改仓库不等于已更新远端，也不能在开发机直接运行：

- `export-router-production.py` → 生产 `/home/nio/backups/fx-production/export-backup.py`，SSH forced command 只接受 backup-v1。
- `nas-pull-router-production.py` → NAS `/home/Nio/.local/share/nanafox-fx-production-backup/backup.py`；cron `/etc/cron.d/nanafox-fx-production` 每天北京时间 03:35，以 Nio 运行。

历史实际目录、容器名、MinIO 前缀保持兼容，不为调整环境称呼而迁移备份数据或密钥。程序运行会生成包含凭据的私有备份，不能输出文件内容或提交制品；SSH key 只留 NAS，严格校验已信任的服务器 host key。

检查最后成功时间、两个库名、对象数量和逐文件回读 SHA256，不把安装定时器视为验证通过。首轮仍在传输时后续触发会通过文件锁跳过。当前网络传输无断点续传，失连需要重跑；不能保证每日都能在下一个窗口前完成。

范围是两生产数据库、角色、配置、容器参数、证书和静态插件/主页/图标。尚未覆盖 Redis AOF、Docker 镜像层、Studio /data 卷及 R2 全量对象的日常异机备份。R2 是当前生产对象存储，模板封面读校验不等于用户作品备份。旧迁移快照只提供迁移时恢复点。

更新程序前核对远端差异及运行锁；正在传输时不停止当前任务。部署新版本后另记其 hash、首轮结果与 MinIO readback，不能将本地语法检查写成远端备份验收。

## Studio 独立每日备份

NAS `/etc/cron.d/nanafox-studio-production` 每天北京时间 03:45 运行相同程序的 `--studio` 模式；使用独立 `studio/run.lock` 和 `studio/last-success.json`，固定 SSH 命令 `backup-studio-v1`。主任务默认模式与原锁不变，仍覆盖两库。

Studio 独立模式只导出 nanafox_studio_prod，保存角色、私有运行参数、Studio 环境、Caddy 与 Studio 证书；MinIO 前缀 `studio-production/<UTC>/`。传输完成后逐文件 SHA256 回读一致才写成功记录。两份远端程序修改前已留 `.before-studio-20260907` 私有备份，本次实际安装已完成，未中断原 Router 备份。

首轮独立 Studio 快照 `20260906T172952Z`，5 文件全部回读一致，北京时间 2026-09-07 01:31:15 完成；数据库 dump 1,877,745 字节。已验证 PG 归档目录与 MinIO 回读，尚未将本快照恢复到另一隔离库做恢复演练。
