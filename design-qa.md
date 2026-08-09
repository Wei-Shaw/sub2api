# EasyHub 内部页面第二轮设计 QA

## 范围

- 目标：在第一轮应用壳层统一的基础上，继续统一列表、筛选、表格、分页、下拉、空态和弹窗。
- 边界：只修改视觉样式，不修改路由、接口、权限、状态流、表单字段或业务交互。
- 首页与内部页承载内容不同，因此以设计语言一致性为准，不复制首页内容布局。

## 视觉基准与实现证据

- 视觉基准：首页 `/home`，截图 `/private/tmp/easyhub-internal-feature-preview/home-style-reference.png`。
  - CSS 视口：`1440 × 1000`。
  - 原始截图：`1432 × 3656` 像素，设备密度 `1`；对照时裁切为 `1432 × 994`。
- 实现页面：用户管理 `/admin/users`，截图 `/private/tmp/easyhub-internal-round2/users-dark-final.png`。
  - CSS 视口：`1440 × 1000`。
  - 实现截图：`1432 × 994` 像素，设备密度 `1`。
- 浅色状态：`/private/tmp/easyhub-internal-round2/users-light.png`。
- 移动状态：`390 × 844`，截图 `/private/tmp/easyhub-internal-round2/users-mobile.png`。
- 弹窗状态：`/private/tmp/easyhub-internal-round2/user-create-modal.png`。
- 下拉展开状态：`/private/tmp/easyhub-internal-round2/select-open.png`。
- 运维页面覆盖检查：`/private/tmp/easyhub-internal-round2/ops-after.png`。
- 全视图基准对照：`/private/tmp/easyhub-internal-round2/qa-source-final.png`。
- 调整前后对照：`/private/tmp/easyhub-internal-round2/qa-before-after.png`。
- 重点区域对照：`/private/tmp/easyhub-internal-round2/qa-focus.png`。

测试状态为本地隔离环境中的已登录管理员，列表包含一条本地管理员记录。站点名和 Logo 继续读取后台配置，不在 UI 代码中写死。

## 必查项

- 字体与排版：页面标题继续使用紧凑无衬线层级；表头改为更接近首页辅助信息的等宽、小字号、字距明确的样式；正文可读性和截断行为未改变。
- 间距与布局：表格区间距从通用卡片式布局收紧为细线分区；桌面 `1440px` 与移动 `390px` 均无横向页面溢出。移动列表保持卡片化扫描方式，操作区和分页未被遮挡。
- 颜色与视觉令牌：移除表头和固定列残留的 Slate 蓝灰，统一为暖黑/暖白；香槟金继续用于主操作和选中状态；成功、告警、角色等语义色保留。
- 图片与图标：没有新增或伪造图片资产；现有站点 Logo 和图标库保持不变。图标尺寸、描边和按钮对齐正常。
- 文案与内容：未改动页面文案、动态数据或表单字段。
- 状态与交互：验证了创建用户弹窗打开/关闭、分页下拉打开/关闭、明暗主题切换、移动端列表和运维页面加载。
- 可访问性：保留语义控件、标签、禁用状态和金色可见焦点；弹窗焦点管理、Esc 关闭和滚动锁定逻辑未改动。
- 控制台：最终页面刷新后的新增错误为 `0`。

## 对照修正记录

1. P2：第一轮后的表格粘性表头和固定列仍使用硬编码 Slate 蓝灰，深色页面出现明显蓝块。
   - 修正：将表头、固定列、悬停行统一为 EasyHub 暖灰/暖黑令牌。
   - 证据：`qa-before-after.png` 左侧为修正前，右侧为修正后。
2. P2：列表容器、运维卡片、选择器和弹窗仍保留较大的圆角与通用阴影，与首页方正细线语言不一致。
   - 修正：在内部应用壳层内统一大圆角表面，降低卡片阴影；单独调整 Teleport 下拉和弹窗表面。
   - 证据：`users-dark-final.png`、`user-create-modal.png`、`select-open.png` 和 `ops-after.png`。
3. P2：移动端列表需要确认页面级样式覆盖不会引起宽度回归。
   - 修正：移动表格继续使用原有响应式数据卡片，只同步颜色、圆角和边框。
   - 证据：`users-mobile.png`；`390px` 视口下文档滚动宽度为 `390px`。
4. 复核：首页基准与最终实现已放入同一张对照图，未发现剩余 P0/P1/P2。

## 后续微调

- P3：支付渠道等专业页面保留其品牌色和少量品牌渐变；运维图表保留蓝/绿数据语义。这些属于状态与渠道识别，不作为本轮统一项。
- P3：若继续第三轮，可按访问频率逐页优化设置页、分组页和支付页的局部信息密度，不需要改变公共组件或业务结构。

final result: passed
