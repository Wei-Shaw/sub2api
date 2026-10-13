# 可迁移备份

此工具把完整 PostgreSQL 数据库与解密账号库、站点 TOTP 所需的固定密钥一起存入一个 AES-256-GCM 加密文件。迁移到另一台机器后，账号库的密码、2FA 密钥、OAuth 系统账号、用户及其他数据库记录都会保留。Redis 是缓存，目标机重新启动时会重建；域名、代理、数据库连接和 Redis 连接仍按目标环境配置。

需要 Node.js 22 或更新版本、匹配服务器主版本的 PostgreSQL 客户端 `pg_dump`/`psql`。源部署的密钥文件应包含 `ACCOUNT_VAULT_ENCRYPTION_KEY`、`TOTP_ENCRYPTION_KEY`、`JWT_SECRET` 和源数据库的 `DATABASE_PASSWORD`。如果使用环境变量而非密钥文件配置固定密钥，仍需提供一个有效的 `--secrets-file`；它也可只含数据库密码。

## 导出

先在当前终端设置长度至少 12 字符的 `BACKUP_PASSPHRASE`，不要把密码写进命令行参数或聊天记录。Windows PowerShell 示例：

```powershell
$securePassword = Read-Host '备份密码' -AsSecureString
$passwordPointer = [Runtime.InteropServices.Marshal]::SecureStringToBSTR($securePassword)
try {
  $env:BACKUP_PASSPHRASE = [Runtime.InteropServices.Marshal]::PtrToStringBSTR($passwordPointer)
  node tools/portable-backup/portable-backup.mjs export --out D:/secure/astrogate.agbackup --secrets-file backend/data/local-run-secrets.env --pg-bin D:/dev/postgresql-16.15/pgsql/bin
} finally {
  Remove-Item Env:BACKUP_PASSPHRASE -ErrorAction SilentlyContinue
  [Runtime.InteropServices.Marshal]::ZeroFreeBSTR($passwordPointer)
}
```

Linux 示例：

```sh
read -rs -p '备份密码: ' BACKUP_PASSPHRASE; echo
export BACKUP_PASSPHRASE
node tools/portable-backup/portable-backup.mjs export --out /secure/astrogate.agbackup --secrets-file /secure/sub2api.env
unset BACKUP_PASSPHRASE
```

默认数据库连接为本机 `sub2api` 用户和数据库。可以用 `--db-host`、`--db-port`、`--db-user`、`--db-name` 调整；数据库密码从环境变量 `DATABASE_PASSWORD` 或源密钥文件读取。
导出前可运行 `node tools/portable-backup/portable-backup.mjs check --secrets-file /secure/sub2api.env`，只检查连接与账号库密钥，不创建备份。

导出后可运行 `node tools/portable-backup/portable-backup.mjs verify --in /secure/astrogate.agbackup`，在不连接数据库的情况下核对密码、完整性和压缩内容。

## 导入新部署

1. 在目标机安装同版本或更新版本 PostgreSQL 客户端，并创建可连接的目标数据库。停止目标后端，先单独备份目标已有数据。
2. 设置目标数据库连接环境变量，包括 `DATABASE_PASSWORD`，以及与导出时相同的 `BACKUP_PASSPHRASE`。也可以用 `--secrets-file` 提供目标数据库密码。
3. 运行下列命令。`--replace` 表示允许用迁移包内容替换目标数据库；错误密码或损坏文件在写入数据库前就会被拒绝。

```sh
node tools/portable-backup/portable-backup.mjs import --in /secure/astrogate.agbackup --secrets-out /secure/restored-keys.env --replace
```

4. 将 `restored-keys.env` 的三个固定密钥设置到目标后端的部署配置，然后启动后端。目标数据库密码仍使用目标机自己的密码。备份包和恢复出的密钥文件都应只让管理员读取。OAuth 凭据也在数据库中，但如果上游已撤销令牌，迁移后仍须重新授权。

工具不会上传备份到 GitHub 或远程存储，也不会在终端打印密钥、密码或数据库内容。当前版本只恢复 PostgreSQL 和上述固定密钥；目标机的本地上传文件、图片对象存储和外部服务数据需要另外迁移。
