# EasyHub 内部页面视觉骨架 QA

## 范围

- 目标：让内部页面的基础视觉语言与现有首页/登录页一致。
- 边界：仅调整全局色板、字体、边框、圆角、阴影、按钮、表单和应用壳层样式；不修改路由、接口、权限、状态流或业务交互。
- 本轮是视觉骨架预览，不逐页重排业务内容。

## 设计基准与实现证据

- 设计基准：首页 `/home`，桌面视口 `1440 × 1000`，截图：`/private/tmp/easyhub-internal-feature-preview/home-style-reference.png`。
- 实现页面：管理控制台 `/admin/dashboard`，桌面视口 `1440 × 1000`，深色截图：`/private/tmp/easyhub-internal-feature-preview/dashboard-dark-final.png`。
- 明色截图：`/private/tmp/easyhub-internal-feature-preview/dashboard-light-final.png`。
- 移动端截图：`390 × 844`，`/private/tmp/easyhub-internal-feature-preview/dashboard-mobile.png`。
- 全视图并排对照：`/private/tmp/easyhub-internal-feature-preview/qa-full-comparison.png`。
- 重点区域并排对照：`/private/tmp/easyhub-internal-feature-preview/qa-focus-comparison.png`。

首页与内部页承载的信息和状态不同，因此本次对照以设计语言为准，不做内容位置的像素级复制。两者已统一为黑/暖白底、香槟金强调色、细边框、低圆角、低阴影、紧凑无衬线标题和等宽辅助文字。

## 检查结果

- 桌面端无横向溢出，`1440px` 视口下页面滚动宽度为 `1432px`（浏览器滚动条占位）。
- 移动端无横向溢出，`390px` 视口下页面滚动宽度为 `382px`。
- 侧栏展开/收起、移动端菜单、明暗主题切换均可用。
- 页面刷新后的新增控制台错误为 `0`；早期日志中的公共配置请求错误发生在本地后端启动前，不属于本轮样式改动。
- 键盘焦点从浏览器默认蓝色描边调整为 EasyHub 金色描边，并保留可见焦点。
- 语义状态色（成功、警告、渠道图标）继续保留，避免仅为了视觉统一而损失状态识别。

## 对照修正记录

1. 首轮对照发现内部页仍有较强的蓝绿主色、较大圆角与阴影，与首页的黑金细线语言不一致。
2. 统一基础色板、应用壳层、卡片、按钮、表单、侧栏和标题层级。
3. 移动端检查发现聚焦控件使用浏览器默认蓝色轮廓，补充金色 `focus-visible` 样式。
4. 重新检查桌面深色、桌面明色、移动端和主要交互，未发现阻断项。

## 剩余建议

- P3：少数页面内部的业务组件仍保留原有较大圆角或彩色图标底，可在下一阶段逐页细化；当前不影响整体壳层一致性，也不影响功能。

final result: passed
