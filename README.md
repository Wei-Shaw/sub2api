# Sub2API

这是一个基于 `sub2api` 上游持续同步维护的中文默认版本。

当前仓库以你自己的 fork 为准：

- 仓库地址：[https://github.com/biubiubiu125/sub2api](https://github.com/biubiubiu125/sub2api)
- 上游参考：[https://github.com/Wei-Shaw/sub2api](https://github.com/Wei-Shaw/sub2api)

本仓库保留了上游的大部分通用能力与持续更新，同时保留了当前业务实际在用的定制功能。

## 当前版本特点

在持续同步上游通用更新的基础上，本仓库保留了以下定制能力：

1. 自定义推广返佣链路
   - 推广员申请、审核、批准、停用、恢复
   - 邀请链接 / 邀请码 / Cookie 绑定
   - 佣金冻结、结算、提现申请、审核、打款、退款冲正
   - 管理员手动加佣、减佣、查看推广员详情

2. 人民币支付展示调整
   - 用户充值支付金额按人民币显示
   - 站内额度与支付金额展示区分更清晰

3. 保留上游新增的风控 / 内容审核能力
   - 管理后台风控中心
   - 内容审核配置
   - 风险日志与相关管理能力

4. 保留上游持续更新的网关、支付、OAuth、调度、兼容层修复

## 适用场景

适合以下场景：

- 自建 AI API 网关
- 多账号统一调度
- 面向用户发放 API Key
- 用户自助充值 / 订阅
- 推广返佣运营
- 管理后台统一运维

## 主要功能

- 多上游账号接入：OAuth、API Key、Service Account 等
- API Key 分发与权限控制
- 用量统计与计费
- 支付系统
- 推广返佣系统
- 管理后台
- 风控 / 内容审核
- 可用渠道展示
- 渠道监控

## 部署说明

这个 fork **推荐使用源码仓库 + 本地构建部署**，不要默认按上游“官方 release 安装脚本”理解。

原因很简单：

- 你的定制功能在你自己的仓库里
- 直接用上游 release / 官方镜像，可能拿不到你的推广返佣和展示调整

### 推荐方式一：本地目录版 Docker Compose

适合正式环境，方便备份、迁移、恢复。

```bash
git clone https://github.com/biubiubiu125/sub2api.git
cd sub2api/deploy
cp .env.example .env
```

编辑 `.env` 后启动：

```bash
docker compose -f docker-compose.local.yml build sub2api
docker compose -f docker-compose.local.yml up -d
```

健康检查：

```bash
curl -fsS http://127.0.0.1:8080/health
```

### 推荐方式二：服务器已有目录直接更新

如果服务器已经部署在：

`/opt/sub2api`

常规更新命令：

```bash
cd /opt/sub2api
git pull --ff-only origin main

cd /opt/sub2api/deploy
docker compose -f docker-compose.local.yml build sub2api
docker compose -f docker-compose.local.yml up -d sub2api

curl -fsS http://127.0.0.1:8080/health
```

如果 `docker-compose.local.yml` 在服务器上有本地改动，更新前先确认工作区干净，避免 `git pull` 被拦住。

## 备份与迁移建议

如果你使用的是本地目录版部署，核心备份内容通常包括：

- `deploy/.env`
- `deploy/docker-compose.local.yml`
- `deploy/data/`
- `deploy/postgres_data/`
- `deploy/redis_data/`

推荐优先使用你现有服务器上的目录级备份方案，再配合云盘保留多份快照。

## 推广返佣说明

当前这份仓库使用的是**自定义 referral 主链路**，不是单纯沿用上游旧版 affiliate 流程。

现有逻辑重点包括：

- 推广绑定后不允许用户随意改上级
- 佣金按真实支付金额计算
- 支持冻结期
- 支持退款自动冲正
- 支持负佣金抵扣
- 提现需推广员主动申请
- 管理员后台审核与打款

## 上游同步策略

当前仓库长期采用以下同步原则：

- 上游通用修复：同步
- 你当前业务在用的推广返佣链路：保留
- 人民币展示调整：保留
- 上游新增加的通用能力（如风控、OAuth、网关修复）：按需合入

也就是说，这不是一个完全脱离上游的私有分叉，而是一个**持续跟上游、同时保留关键业务定制**的版本。

## 安装注意

如果你是第一次部署，请优先确认：

1. 使用的是你自己的仓库源码，而不是上游 release 包
2. 使用的是 `docker-compose.local.yml`
3. 数据目录挂载正确
4. 升级前 `git status` 干净

## 常用命令

查看状态：

```bash
git status --short --branch
```

查看容器：

```bash
docker ps --format 'table {{.Names}}\t{{.Status}}\t{{.Image}}'
```

查看健康状态：

```bash
curl -fsS http://127.0.0.1:8080/health
```

## 说明文件

本仓库默认以中文说明为准。

- 主说明：`README.md`
- 支付说明：`docs/PAYMENT_CN.md`

其余多语言 README 不再作为主要入口。
