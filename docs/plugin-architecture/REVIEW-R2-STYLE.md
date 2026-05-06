# Review Round 2 — 代码风格 + CLAUDE.md 规范

## 逐文件检查

### provider.go (20 行)
- [x] 函数行数：OK（无函数体，仅接口定义）
- [x] 嵌套：OK
- [x] 魔法值：OK（无硬编码值）
- [x] 命名：OK（`GatewayProvider` PascalCase 类型，方法名清晰）
- [x] 接口最小化：OK（4 个方法，职责聚焦）
- [x] 注释质量：OK（解释了 Phase 1 / Phase 2+ 的演进意图 = WHY）
- [x] import 组织：OK

### registry.go (69 行)
- [x] 函数行数：OK（最长 `ForProtocol` 14 行，`HasProvider` 13 行）
- [x] 嵌套：OK（最深 2 层：for+if）
- [x] 魔法值：OK
- [x] 命名：OK
- [x] 并发安全：OK（所有公开方法持 RWMutex）
- [x] 注释质量：OK

### request.go (69 行)
- [x] 函数行数：N/A（纯 struct 定义）
- [x] 命名：OK
- [x] 注释质量：好（分节注释解释每组字段的生命周期，`GinContext` 有 Phase 1 临时性说明）
- [ ] 魔法值：见问题 #1（Protocol 字段注释引用硬编码字符串，但更关键的是 provider 文件中的硬编码）

### result.go (34 行)
- [ ] **gofmt**：见问题 #2
- [x] 命名：OK
- [x] 注释质量：OK

### pipeline.go (422 行)
- [ ] 函数行数：见问题 #3（`Execute` 33 行，超出 30 行限制）
- [x] 嵌套：OK（最深 2 层）
- [x] 魔法值：OK（`defaultMaxFailovers = 10` 已用常量）
- [ ] 命名：见问题 #4（`middleware2` 别名）
- [ ] import 组织：见问题 #5（第三方和项目内混合）
- [ ] 错误处理：见问题 #6（`resolveChannelMapping` 吞掉错误）、问题 #7（`resolveSessionHash` 吞掉错误）
- [x] 并发安全：OK（pipeline 本身无共享状态）
- [x] 注释质量：OK（每个方法有生命周期注释，TODO 标注了 M3 遗留）

### anthropic_provider.go (84 行)
- [ ] **gofmt**：见问题 #2
- [x] 函数行数：OK（最长 `anthropicResultToForwardResult` 18 行）
- [ ] 魔法值：见问题 #1
- [ ] 复用：见问题 #8（`anthropicResultToForwardResult` 与 `antigravityResultToForwardResult` 完全重复）
- [ ] 复用：见问题 #9（`ShouldFailover` 三份相同实现）
- [x] 注释质量：OK

### openai_provider.go (69 行)
- [ ] **gofmt**：见问题 #2
- [x] 函数行数：OK
- [ ] 魔法值：见问题 #1
- [ ] 复用：见问题 #9
- [x] 注释质量：OK

### antigravity_provider.go (101 行)
- [ ] **gofmt**：见问题 #2
- [x] 函数行数：OK
- [x] 嵌套：OK
- [ ] 魔法值：见问题 #1
- [ ] 复用：见问题 #8、#9
- [x] 注释质量：OK

### wire.go (34 行)
- [x] 函数行数：OK
- [x] 命名：OK
- [x] 注释质量：OK

### registry_test.go (134 行)
- [x] 构建标签：OK（`//go:build unit` 在第 1 行）
- [x] 函数行数：OK
- [x] 覆盖度：OK（Register / Get / Unregister / ForProtocol / HasProvider / Overwrite 均测）
- [x] 命名：OK

### credentials.go (123 行, plugin-sdk)
- [x] 函数行数：OK
- [x] 接口最小化：OK（4 个方法，三种凭证模型各一 + 注册）
- [x] 命名：OK
- [x] 错误处理：OK（哨兵错误 `ErrCredentialNotFound` / `ErrRefreshFailed` 用 `errors.New`）
- [x] 注释质量：优秀（接口文档解释三种模型的 WHY、nilCredentialManager 解释为何不 no-op）
- [x] import 组织：OK

### credentials_test.go (77 行, plugin-sdk)
- [x] 构建标签：OK（`//go:build unit` 在第 1 行）
- [x] 函数行数：OK
- [x] 覆盖度：OK（所有 nil manager 方法 + 哨兵错误 + 编译时接口检查）
- [x] 命名：OK

---

## 发现的问题

### #1 [warning] 平台字符串应使用已有的 domain 常量

- **文件**：
  - `anthropic_provider.go:23-24`（`"anthropic"`）
  - `openai_provider.go:23-24`（`"openai"`）
  - `antigravity_provider.go:25-26`（`"antigravity"`, `"anthropic"`, `"gemini"`）
  - `antigravity_provider.go:36,38`（switch case `"anthropic"`, `"gemini"`）
- **规范条目**：CLAUDE.md「禁止魔法值」+「跨文件/组件共享的常量只在一处定义」
- **描述**：`backend/internal/domain/constants.go` 已定义 `PlatformAnthropic`、`PlatformOpenAI`、`PlatformGemini`、`PlatformAntigravity`。gateway 包的 provider 文件全部使用硬编码字符串 `"anthropic"` / `"openai"` / `"antigravity"` / `"gemini"`，违反「共享常量只在一处定义」。
- **建议修复**：`import "github.com/Wei-Shaw/sub2api/internal/domain"` 后替换为 `domain.PlatformAnthropic` 等。

### #2 [blocker] gofmt 格式不通过

- **文件**：`result.go`、`anthropic_provider.go`、`antigravity_provider.go`、`openai_provider.go`
- **规范条目**：CLAUDE.md CI 检查「gofmt」
- **描述**：`gofmt -l` 报告这 4 个文件有格式差异。主要是 struct 字段对齐（`result.go`）和单行函数 `Platform()` 的空格对齐（三个 provider 文件）。CI 中 `gofmt` 检查会失败。
- **建议修复**：`gofmt -w backend/internal/gateway/result.go backend/internal/gateway/anthropic_provider.go backend/internal/gateway/antigravity_provider.go backend/internal/gateway/openai_provider.go`

### #3 [warning] Execute 函数超过 30 行限制

- **文件**：`pipeline.go:84-116`（33 行）
- **规范条目**：CLAUDE.md「函数 <= 30 行」
- **描述**：`Execute` 方法含 5 个参数 + 函数签名共 33 行，超出 30 行限制 3 行。虽然逻辑上是线性管道调用（每步一行 + 错误处理），但按规范需要注释说明为何不可拆分，或者实际拆分。
- **建议修复**：考虑将前半段（readAndParse + resolveChannelMapping + acquireUserSlot + prepareBilling + resolveSessionHash）合成一个 `prepareRequest` 方法，将 Execute 缩减到约 15 行。或者添加注释说明此方法是管道编排的顶层入口，拆分会降低可读性。

### #4 [warning] import 别名 `middleware2` 命名不清晰

- **文件**：`pipeline.go:12`
- **规范条目**：CLAUDE.md「命名一致，同一概念统一命名」
- **描述**：`middleware2` 是一个含义不明的别名，数字后缀暗示它是某种冲突规避的产物，但实际上包内没有其他 `middleware` 导入。
- **建议修复**：改为 `authmw` 或直接 `middleware`（无冲突时直接使用包名）。

### #5 [info] import 分组不标准：第三方与项目内混合

- **文件**：`pipeline.go:3-15`
- **规范条目**：CLAUDE.md「import 组织 — 标准库 / 第三方 / 内部项目是否分组」
- **描述**：`github.com/gin-gonic/gin`（第三方）与 `github.com/Wei-Shaw/sub2api/...`（项目内部）在同一个 import 组中。Go 惯例是三组：标准库、第三方、项目内部，用空行分隔。
- **建议修复**：将 `gin` 移到第三方组（在标准库和项目 import 之间加一组）。

### #6 [warning] resolveChannelMapping 静默吞掉错误

- **文件**：`pipeline.go:170`
- **规范条目**：CLAUDE.md「错误处理 — 是否有忽略的 error」
- **描述**：`mapping, _ := p.gatewayService.ResolveChannelMappingAndRestrict(...)` 用 `_` 丢弃了错误。同时 `resolveChannelMapping` 函数签名返回 `error` 但实际永远返回 `nil`，误导调用方以为有错误处理。
- **建议修复**：要么正确处理错误（`mapping, err := ...` + `if err != nil { return err }`），要么如果这个调用的错误确实无关紧要，添加注释说明 WHY（例如 "channel mapping failure is non-fatal; proceed with empty mapping"）。

### #7 [info] resolveSessionHash 静默吞掉错误

- **文件**：`pipeline.go:252`
- **规范条目**：CLAUDE.md「错误处理 — 是否有忽略的 error」
- **描述**：`cachedID, _ := p.gatewayService.GetCachedSessionAccountID(...)` 吞掉了错误。如果 sticky session 查询失败，调用方无从得知。考虑到这是非关键路径（session cache miss 不阻塞请求），至少应添加 debug 日志。
- **建议修复**：`cachedID, err := ...; if err != nil { slog.Debug("pipeline.session_cache_lookup_failed", "error", err) }`。

### #8 [warning] anthropicResultToForwardResult 和 antigravityResultToForwardResult 完全重复

- **文件**：
  - `anthropic_provider.go:67-84`
  - `antigravity_provider.go:84-101`
- **规范条目**：CLAUDE.md 工程原则「复用」—「同一概念只在一处实现」
- **描述**：两个函数接受相同类型 `*service.ForwardResult`，执行完全相同的字段映射，产生相同输出 `*ForwardResult`。这是教科书级别的复制粘贴。
- **建议修复**：提取为一个共享函数（例如 `serviceResultToForwardResult`）放在 `result.go` 或单独的 `result_mapping.go` 中，两个 provider 直接调用。

### #9 [warning] ShouldFailover 三个 provider 实现完全相同

- **文件**：
  - `anthropic_provider.go:43-48`
  - `openai_provider.go:44-49`
  - `antigravity_provider.go:47-52`
- **规范条目**：CLAUDE.md 工程原则「复用」
- **描述**：三个 provider 的 `ShouldFailover` 实现完全一样（`errors.As(err, &UpstreamFailoverError)`）。当前是 Phase 1 thin adapter，三者行为相同，但如果某个 provider 未来需要不同的 failover 策略，可以 override。
- **建议修复**：提供一个默认实现（如包级函数 `DefaultShouldFailover`），provider 只需调用 `return DefaultShouldFailover(err)`。或者提供一个 `BaseProvider` 嵌入结构体，提供默认 `ShouldFailover` 实现。这样保留了未来 override 的能力，同时消除重复。

---

## 统计

- 检查文件数：12
- 问题总数：9（blocker: 1, warning: 6, info: 2）
- 整体评价：**PASS WITH WARNINGS**

**Blocker 摘要**：gofmt 格式不通过（4 个文件），会导致 CI 失败，必须在合并前修复。

**整体设计质量**：代码结构整洁，pipeline 拆分合理，函数职责清晰，注释质量高（多数注释解释 WHY 和 Phase 演进意图）。主要问题集中在：(1) gofmt 格式化遗漏，(2) 平台字符串硬编码未复用已有常量，(3) 两处映射函数和三处 failover 函数的 copy-paste 违反复用原则。
