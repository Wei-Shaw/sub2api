# 实现说明：分组上游订阅档位（upstream_plan）

| 项 | 内容 |
|----|------|
| 状态 | 共识已确认 · 待实现 |
| 日期 | 2026-07-29 |
| 用途 | **仅元数据/展示**（不绑号、不调度） |
| 相关 | 创建/编辑分组、分组列表、系统设置 |

---

## 1. 背景与目标

### 1.1 现状

- `groups` 仅有 `platform`，无上游套餐/档位字段。
- 账号侧档位分散：`credentials.plan_type`、Grok billing/quota、Antigravity `load_code_assist` tier 等。
- 创建分组 UI 选平台后无档位选择；列表无法展示「该组对应哪档上游订阅」。

### 1.2 目标

1. 系统设置中按平台维护可选档位列表 `{ code, label }`。
2. 首次空配置时用**内置种子**回填到设置，之后以配置为准。
3. 创建/编辑分组：选平台后单选档位（可选空）；将 **code** 存入 `groups.upstream_plan`。
4. 分组列表展示档位徽章（用配置 label，未知 code 回退显示 raw）。
5. **不**参与绑号过滤、**不**参与调度选号。

### 1.3 非目标

- 绑账号时强制 plan 匹配。
- 网关/调度按 plan 过滤账号池。
- 多选档位。
- 自动私有组（`private-*`）写入档位。
- Composite 平台档位选择。

---

## 2. 产品规则（已锁定）

| 决策 | 结论 |
|------|------|
| 用途 | 仅展示/元数据 |
| 存储 | 单字段字符串 code（`upstream_plan`） |
| 选择 | 单选；可空 = 未指定 |
| 配置 | 系统设置按平台 `[{code,label}]` |
| 空配置 | Get 时若未配置/空 map → 用内置种子回填到配置（持久化） |
| 校验 | 非空时 code 必须 ∈ 该平台配置列表，否则 400 |
| 换平台 | 清空档位（前端清空 + 后端兜底） |
| 私有组 provision | 不写 `upstream_plan` |
| Composite | 不展示档位选择 |
| UI | 创建 + 编辑 + 列表 + 设置页 |
| 种子 | 见 §3.3 |

---

## 3. 数据模型

### 3.1 分组字段

| 字段 | 类型 | 约束 | 说明 |
|------|------|------|------|
| `upstream_plan` | `varchar(64)` NULL | 可空 | 存 **code**（小写规范建议与种子一致，校验时与配置项 code 精确匹配） |

- Ent：`backend/ent/schema/group.go` 增加字段。
- Migration：建议 `192_add_group_upstream_plan.sql`（在 191 之后）。
- 服务模型 `service.Group`、DTO、`CreateGroupInput` / `UpdateGroupInput` 同步。
- 生成：`go generate ./ent`（或项目惯用命令）。

**存量分组**：`upstream_plan = NULL`，行为 = 未指定。

### 3.2 系统设置

| Key | 建议名 | 值形态 |
|-----|--------|--------|
| 设置键 | `group_upstream_plans` | JSON object |

JSON 示例：

```json
{
  "openai": [
    { "code": "free", "label": "Free" },
    { "code": "plus", "label": "Plus" },
    { "code": "team", "label": "Team" },
    { "code": "pro", "label": "Pro" }
  ],
  "grok": [
    { "code": "free", "label": "Grok Free" },
    { "code": "basic", "label": "Basic" },
    { "code": "supergrok", "label": "SuperGrok" },
    { "code": "supergrokheavy", "label": "SuperGrok Heavy" }
  ],
  "antigravity": [
    { "code": "free-tier", "label": "Free" },
    { "code": "g1-pro-tier", "label": "Pro" },
    { "code": "g1-ultra-tier", "label": "Ultra" }
  ],
  "anthropic": [],
  "gemini": []
}
```

- **不包含** `composite` 键（或包含但创建 UI 忽略）。
- code **大小写**：种子与校验建议 **小写规范化**（入库前 `strings.ToLower(strings.TrimSpace(code))`；label 保持展示用原文）。Antigravity 的 `g1-pro-tier` 等保持连字符小写。
- Grok 种子 code 与前端 `PlatformTypeBadge` 规范化一致：`supergrok` / `supergrokheavy`（非 `SuperGrok` 原串）。

### 3.3 内置种子（K1）

| 平台 | 默认项 |
|------|--------|
| openai | free, plus, team, pro |
| grok | free, basic, supergrok, supergrokheavy |
| antigravity | free-tier, g1-pro-tier, g1-ultra-tier |
| anthropic | `[]` |
| gemini | `[]` |

**回填策略（推荐）**：

1. `GetGroupUpstreamPlans(ctx)`：读设置；若 key 不存在或解析为空 map → 写入种子 JSON 到 DB → 返回种子。
2. 若 key 存在但某平台 key 缺失 → 对该平台用 `[]`（不强制补种子），避免覆盖运营故意删空；**仅整体空才 seed**。
3. 更新设置：全量替换 map；校验每项 `code` 非空、同平台 code 唯一；`label` 空则回退为 `code`。
4. 加入 `OmittedSettingKeys`：部分 PATCH 不误清。

---

## 4. 后端 API / 服务

### 4.1 SettingService

| 方法 | 行为 |
|------|------|
| `GetGroupUpstreamPlans(ctx) (map[string][]PlanOption, error)` | 读 + 空则 seed |
| `Update` 路径 | 解析/校验 `group_upstream_plans` |
| `ListPlansForPlatform(ctx, platform) []PlanOption` | 给校验与前端用；composite → 空 |

```go
type GroupUpstreamPlanOption struct {
    Code  string `json:"code"`
    Label string `json:"label"`
}
```

### 4.2 CreateGroup / UpdateGroup

**CreateGroupInput / UpdateGroupInput** 增加：

```go
UpstreamPlan *string // 或 string；空串与 nil 均视为未指定
```

**校验** `validateUpstreamPlan(ctx, platform, plan string) error`：

1. `plan == ""` → OK，存 NULL。
2. `platform == composite` → 若 plan 非空 → 400（composite 不允许档位）。
3. 取该平台配置列表；plan 规范化后不在列表 → 400  
   `INVALID_GROUP_UPSTREAM_PLAN`。
4. 通过则写入规范化后的 code。

**换平台（Update）**：

- 若 `input.Platform` 变更且与旧不同：  
  - 若本次未显式传新 `upstream_plan` → **强制清空**（P1 服务端兜底）。  
  - 若传了新 plan → 按新平台校验。
- 私有组身份锁（既有）：禁改 name/platform/subscription_type；**允许**改 `upstream_plan`（G1 仅自动供给不写；人工编辑可填——若产品希望私有组也禁改档位，可另加；当前共识 G1 只约束自动供给，编辑未禁）。

> 建议明确：**私有组编辑允许改 `upstream_plan`**（仅自动 provision 写空）。若需锁定，再开一项。

### 4.3 DTO / Handler

- Admin Group 响应 JSON 增加 `upstream_plan`（string | null）。
- Create/Update body 接受 `upstream_plan`。
- 设置 GET/PUT 响应与请求体含 `group_upstream_plans`。

### 4.4 PrivateGroupProvisioner

- `ensurePrivateGroup` / create 路径：**不设置** `upstream_plan`（DB 默认 NULL）。
- 无需改绑定/订阅逻辑。

### 4.5 配置变更与历史数据

- 删除配置中某 code 后，已落库分组仍保留旧 code。
- 列表展示：配置中有 label 则用 label；否则显示 raw code（或 `code (未知)` i18n）。
- **不**做批量改写历史分组。

---

## 5. 前端

### 5.1 类型

```ts
// types
upstream_plan?: string | null

// settings
group_upstream_plans?: Record<string, { code: string; label: string }[]>
```

### 5.2 系统设置页（U3）

- 区块：**「分组上游订阅档位」**。
- 按平台（anthropic / openai / gemini / antigravity / grok）可编辑列表：code、label、增删行、排序（可选）。
- 保存走现有 settings update；注意 omit 语义。
- 加载时后端已 seed，一般总能看到默认 openai/grok/antigravity 项。

### 5.3 创建 / 编辑分组（GroupsView）

1. 平台 `Select` 下方增加「上游订阅档位」`Select`。
2. 选项：
   - 首项：`''` → 「未指定」。
   - 其余：`GetGroupUpstreamPlans[platform]` → `{ value: code, label }`。
3. `platform === 'composite'` 或选项为空（仅未指定）→ 隐藏档位选择或 disabled + hint。
4. `@change` 平台 → `upstream_plan = ''`（P1）。
5. 提交 body 带 `upstream_plan`（空串或省略）。

### 5.4 分组列表

- 平台列旁或独立小徽章：有 `upstream_plan` 时显示 **label**（查本地已加载的 plans map；列表页可一次拉 settings 或随 groups API 不嵌 label）。
- 推荐：列表页缓存 `group_upstream_plans`，前端 `code → label`；未知则 raw code。

### 5.5 i18n

- zh/en：`admin.groups.form.upstreamPlan`、`upstreamPlanHint`、`upstreamPlanUnspecified`、设置区块文案、错误提示。

---

## 6. 校验错误约定

| 场景 | HTTP | code | message 示例 |
|------|------|------|----------------|
| plan 非空且不在配置 | 400 | `INVALID_GROUP_UPSTREAM_PLAN` | upstream_plan is not allowed for this platform |
| composite 带 plan | 400 | `INVALID_GROUP_UPSTREAM_PLAN` | composite groups cannot have upstream_plan |
| 设置 JSON 非法 | 400 | `INVALID_GROUP_UPSTREAM_PLANS` | … |
| 同平台 code 重复 | 400 | `INVALID_GROUP_UPSTREAM_PLANS` | duplicate code |

---

## 7. 测试计划

### 后端 unit

1. Seed：空设置 → Get 返回 K1 且可持久化（mock repo）。
2. CreateGroup：合法 plan 成功；非法 plan 400；空 plan 成功。
3. CreateGroup composite + plan → 400。
4. UpdateGroup 换 platform 未传 plan → plan 被清空。
5. UpdateGroup 换 platform + 新 plan 合法 → 成功。
6. Private provision 创建的组 `UpstreamPlan` 为空。
7. Settings update：重复 code 拒绝；omit 不擦除。

### 前端（可选 Vitest / 手工）

1. 换平台清空档位。
2. composite 不显示档位。
3. 列表未知 code 显示 raw。

---

## 8. 建议 PR 切片

| PR | 内容 | 依赖 |
|----|------|------|
| **PR1** | migration + Ent + Group 模型/DTO 读写 `upstream_plan`；Create/Update 校验骨架（列表可先 hardcode seed 函数） | 无 |
| **PR2** | Setting key + seed + Get/Update + OmittedSettingKeys + API | PR1 |
| **PR3** | GroupsView 创建/编辑/列表；i18n | PR1+PR2 |
| **PR4** | SettingsView 档位配置 UI + i18n | PR2 |

也可 PR1+PR2 合并为后端一单，PR3+PR4 为前端一单。

---

## 9. 实现检查清单

- [ ] `192_add_group_upstream_plan.sql` + Ent 字段
- [ ] `service.Group` / Input / admin_group Create&Update 校验
- [ ] `SettingKeyGroupUpstreamPlans` + seed + 解析校验
- [ ] Handler/DTO 暴露字段
- [ ] Private provision 不写 plan
- [ ] 前端 types + settings API
- [ ] GroupsView 创建/编辑/列表
- [ ] SettingsView 配置块
- [ ] i18n zh/en
- [ ] 单测覆盖 §7
- [ ] 确认私有组编辑是否允许改 plan（默认允许）

---

## 10. 风险与后续

| 风险 | 缓解 |
|------|------|
| 运营删 code 后列表显示 raw | 可接受；或标记「未知」 |
| code 与账号 plan_type 不完全同构 | 本功能不绑号，仅展示；二期对齐过滤时再统一归一化 |
| 种子与真实账号脏字符串不一致 | 配置可扩展；不扫库 |

**二期可选**：绑号警告/硬过滤；调度过滤；按 plan 筛选分组列表。

---

## 11. 确认项（实现前可选拍板）

1. 字段名是否就用 `upstream_plan` / 设置键 `group_upstream_plans`？  
2. 私有组人工编辑是否允许填写档位？（说明默认：**允许**）  
3. Seed 是否在首次 Get 时**写回 DB**，还是仅内存默认、保存设置后才落库？（说明推荐：**首次 Get 写回**，与「回填到配置」一致）

---

文档路径：`docs/design/group-upstream-plan.md`
