# Frontend 组件清单

**生成时间:** 2026-04-28  
**扫描级别:** Quick Scan  
**范围:** `frontend/src/components` 与 `frontend/src/views`

## 总览

前端组件按“通用控件 + 布局 + 领域组件 + 页面视图”组织。Quick Scan 统计目录和文件名，未逐个组件读取 props/emits/slot 细节。

## 通用组件

`frontend/src/components/common` 是复用基础控件层，包含：

- 弹窗与反馈：`BaseDialog`、`ConfirmDialog`、`Toast`、`LoadingSpinner`、`Skeleton`、`EmptyState`
- 表格与导航：`DataTable`、`Pagination`、`NavigationProgress`
- 表单控件：`Input`、`TextArea`、`Select`、`Toggle`、`SearchInput`、`DateRangePicker`
- 业务通用展示：`StatusBadge`、`GroupBadge`、`GroupCapacityBadge`、`PlatformIcon`、`PlatformTypeBadge`、`ModelIcon`、`VersionBadge`
- 组合控件：`GroupSelector`、`ProxySelector`、`ImageUpload`、`ExportProgressDialog`、`AnnouncementBell`

新增跨页面复用 UI 时应优先放在这里，避免在 view 内重复实现基础表格、弹窗、分页和表单逻辑。

## 布局组件

`frontend/src/components/layout`：

- `AppLayout`、`AppHeader`、`AppSidebar`：登录后主框架。
- `AuthLayout`：认证页面布局。
- `TablePageLayout`：后台表格型页面布局。

## 用户侧组件

| 领域 | 目录 | 说明 |
| --- | --- | --- |
| API Key/账号能力 | `components/account`, `components/keys` | 账号状态、配额、授权、测试、编辑、key 使用方式 |
| 用户 dashboard | `components/user/dashboard` | 统计卡、图表、最近用量、快捷操作 |
| 用户 profile | `components/user/profile` | 头像、资料、密码、TOTP、身份绑定、余额通知 |
| 用户 monitor | `components/user/monitor` | 渠道状态卡、时间线、指标、provider icon |
| 支付 | `components/payment` | 金额输入、支付方式、二维码、Stripe、订单表、套餐卡 |
| 认证 | `components/auth` | LinuxDo/OIDC/WeChat OAuth、TOTP 登录、pending OAuth 创建账号 |

## 管理端组件

`frontend/src/components/admin` 包含多个领域子目录：

- `account`：账号操作菜单、批量操作、导入、测试、定时测试面板。
- `announcements`：公告阅读状态、目标用户编辑。
- `channel`：渠道定价、模型 tag、interval row。
- `group`：分组 RPM override、费率倍数弹窗。
- `monitor`：渠道监控表单、过滤、结果、模板管理。
- `payment`：订单详情、退款、收入图表、支付方式图表、用户排行榜。
- `usage`：用量过滤、统计卡、表格、导出进度、清理任务。
- `user`：用户编辑、余额、API keys、allowed groups、创建用户等。

顶层 `ErrorPassthroughRulesModal` 和 `TLSFingerprintProfilesModal` 是跨页面管理弹窗。

## 图表组件

`frontend/src/components/charts`：

- `TokenUsageTrend`
- `ModelDistributionChart`
- `GroupDistributionChart`
- `EndpointDistributionChart`
- `UserBreakdownSubTable`

图表依赖 Chart.js / vue-chartjs，适合 dashboard 和用量分析页复用。

## 页面视图

| 目录 | 文件数 | 说明 |
| --- | ---: | --- |
| `views/user` | 24 | 用户 dashboard、keys、usage、profile、payment、orders、channel status 等 |
| `views/admin` | 49 | 管理后台各领域页面，含 ops 子系统和 payment order 子系统 |
| `views/auth` | 17 | 登录注册、OAuth 回调、邮件验证、重置密码 |
| `views/setup` | 1 | setup wizard |

## 测试覆盖入口

组件测试分布在各目录的 `__tests__` 下。高风险领域已有测试文件名覆盖：

- 账号与配额：`AccountStatusIndicator`、`AccountTestModal`、`UsageProgressBar`、`BulkEditAccountModal`
- 支付：`PaymentProviderDialog`、`PaymentStatusPanel`、`paymentFlow`、`providerConfig`
- 认证：OAuth 回调、TOTP、pending OAuth
- 运维/图表/表格：`UsageTable`、`ModelDistributionChart`、`GroupDistributionChart`、Ops token stats

## 组件开发规则

- 通用控件保持无领域耦合，领域组件放到对应目录。
- 页面 view 负责组合，不应复制通用控件内部逻辑。
- 新增后台表格/筛选/弹窗时优先沿用现有 admin 目录模式和 Tailwind 暗色 class。
- 新增路由页面必须同步 route meta、i18n 文案、API 模块和必要的组件测试。
