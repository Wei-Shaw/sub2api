# EasyHub 内部页面第三轮设计 QA

## 范围

- 目标：收敛第二轮后仍然突出的设置导航、订阅提示、计划卡片、支付弹窗和公告弹窗样式，使其继续沿用首页/登录页的暖黑、香槟金、方正细线语言。
- 边界：只调整 CSS 类和视觉令牌；未修改路由、接口、权限、数据结构、表单字段或业务状态流。
- 复核页面：系统设置、分组管理、订阅管理、我的订阅，以及支付/续费、公告自动弹出和公告详情组件。

## 视觉基准与实现证据

- 视觉基准：首页 `/home`，截图 `/private/tmp/easyhub-internal-feature-preview/home-style-reference.png`。
  - CSS 视口：`1440 × 1000`。
  - 原始截图：`1432 × 3656` 像素，设备密度 `1`；对照使用与内部页同高的基准裁切。
- 系统设置调整前：`/private/tmp/easyhub-internal-round3/settings-before.png`。
- 系统设置调整后：`/private/tmp/easyhub-internal-round3/settings-after.png`。
  - CSS 视口：`1440 × 1000`。
  - 截图：`1432 × 994` 像素，设备密度 `1`。
- 移动状态：`/private/tmp/easyhub-internal-round3/settings-mobile-after.png`。
  - CSS 视口：`390 × 900`；截图：`382 × 882` 像素，设备密度 `1`。
- 订阅管理空态：`/private/tmp/easyhub-internal-round3/subscriptions-admin-before.png`。
- 用户订阅空态：`/private/tmp/easyhub-internal-round3/subscriptions-user-before.png`。
- 订阅指南弹窗：`/private/tmp/easyhub-internal-round3/subscription-guide-final.png`。
- 首页基准与最终设置页同图对照：`/private/tmp/easyhub-internal-round3/qa-source-final.png`。
- 设置页调整前后同图对照：`/private/tmp/easyhub-internal-round3/qa-before-after.png`。
- 顶部导航重点对照：`/private/tmp/easyhub-internal-round3/qa-focus.png`。
- 公告弹窗调整前：`/private/tmp/easyhub-internal-round3/announcement-popup-before.png`。
- 公告弹窗调整后：`/private/tmp/easyhub-internal-round3/announcement-popup-final.png`。
  - CSS 视口与截图：`1440 × 1000`，设备密度 `1`。
- 公告弹窗移动状态：`/private/tmp/easyhub-internal-round3/announcement-popup-mobile.png`。
  - CSS 视口：`390 × 900`，设备密度 `1`。
- 公告弹窗调整前后同图对照：`/private/tmp/easyhub-internal-round3/announcement-popup-before-after.png`。
- 公告弹窗重点区域对照：`/private/tmp/easyhub-internal-round3/announcement-popup-focus.png`。
- 首页基准与最终公告弹窗同图对照：`/private/tmp/easyhub-internal-round3/announcement-popup-source-final.png`。

测试状态为本地隔离环境中的已登录管理员。站点名、Logo、订阅数据和支付能力继续由现有后台配置与接口提供，未在 UI 中写死。

## 必查项

- 字体与排版：沿用前两轮的页面标题、正文和等宽辅助信息层级；公告标题收敛为内部页标题字重，时间信息使用等宽辅助文本，长标题允许安全换行。
- 间距与布局：设置导航高度、标签数量和桌面分配逻辑不变；移动端继续使用横向滚动。公告弹窗在桌面居中，在 `390px` 视口下保留完整边界、正文滚动区和关闭操作。
- 颜色与视觉令牌：移除设置导航的 Slate 蓝灰玻璃底和青蓝渐变下划线，改为暖黑/暖白底、香槟金边框和实色选中线；订阅指南及公告状态统一为低饱和暖金提示面。
- 圆角与阴影：设置导航、计划卡片、续费弹窗、帮助图片、订阅提示和公告弹窗统一为 `rounded-sm`；去除计划卡片和公告按钮的悬浮位移与重阴影，弹窗仅保留必要层级阴影。
- 语义色：订阅有效/无限额度等成功语义仍保留绿色；渠道平台色和支付品牌色未被覆盖。
- 图片与图标：未新增或伪造图片资产；公告中的 Bell、Clock、Eye、Info、Close 和 Check 图标改为复用现有 `Icon` 组件。
- 状态与交互：实际验证设置标签从“通用设置”切换到“功能开关”、移动端横向标签、订阅指南打开、管理员/用户订阅空态，以及公告预览的打开、关闭、重开和移动显示。
- 可访问性：tablist/tab 的选中语义、按钮名称、键盘焦点样式、公告状态文本、弹窗关闭逻辑和禁用状态未改动。
- 控制台：最终刷新后的新增 error 级别日志为 `0`。

## 对照修正记录

1. P2：系统设置顶部导航仍使用蓝灰玻璃容器、青蓝渐变下划线和卡片阴影，与首页暖黑细线体系冲突。
   - 修正：改为方正细线容器，使用香槟金选中边框、图标底和实色下划线；深色模式采用暖黑表面。
   - 证据：`qa-before-after.png` 和 `qa-focus.png`。
2. P2：支付计划卡片和 Teleport 续费弹窗在应用壳层覆盖之外仍会恢复大圆角、悬浮位移和重阴影。
   - 修正：组件自身显式使用小圆角、纯颜色过渡和轻量阴影，确保普通页面与 Teleport 场景一致。
   - 证据：`SubscriptionPlanCard`、`PaymentView` 的定向单测与生产构建均通过。
3. P2：订阅指南弹窗提示块保留蓝色强调，与页面其余暖金操作色不一致。
   - 修正：改为暖金边框和低对比提示底，表格与弹窗同步使用小圆角。
   - 证据：`subscription-guide-final.png`。
4. P2：移动端需要确认九项设置标签在视觉调整后仍可操作且内容不被压缩。
   - 修正：保留原有横向滚动结构；`390px` 视口下首屏标签与表单边界正常。
   - 证据：`settings-mobile-after.png`。
5. P2：公告自动弹窗和公告列表详情仍使用大圆角、橙/蓝渐变装饰光斑、重阴影和按钮放大，与内部页方正细线体系不一致。
   - 修正：两类公告弹窗统一为暖黑/暖白分区、香槟金状态、实色内容引导线、小圆角和轻量过渡；可见图标统一复用项目图标组件。
   - 证据：`announcement-popup-before-after.png`、`announcement-popup-focus.png` 和 `announcement-popup-mobile.png`。
6. 复核：首页基准与公告弹窗最终实现已放入同一张对照图；未发现剩余 P0/P1/P2。

## 验证

- `vue-tsc --noEmit`：通过。
- ESLint（7 个本轮修改的 Vue 文件）：通过。
- Vitest：`SettingsView.spec.ts`、`PaymentView.spec.ts`、`SubscriptionPlanCard.spec.ts`，共 `55` 项通过。
- Vitest：`AnnouncementPopup.spec.ts`，共 `12` 项通过。
- `vite build`：通过；仅保留仓库已有的动态/静态导入和大 chunk 提示。
- `git diff --check`：通过。

## 已知边界

- 当前本地登录夹具为管理员，访问用户购买路由会按既有权限重定向；计划卡片与支付页视觉调整通过组件测试、类型检查和生产构建验证，没有为截图伪造业务数据。
- 支付渠道品牌色、成功/告警状态色和图表数据色属于必要语义，不做全局暖金化。

final result: passed
