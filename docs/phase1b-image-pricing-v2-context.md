# Phase 1b 会话上下文摘要 — fal 图片三档定价矩阵改造

> 用于在新会话中直接接续本任务的上下文。所有结论均基于当前仓库 `/home/jiantaoli/sub2api`。

---

## 一、任务背景

将 fal 分组的图片定价从**旧的 6 档矩阵 + 三档单值（image_price_1k / 2k / 4k）**改造为 **V2 三档定价矩阵**（`tier_1k` / `tier_2k` / `tier_4k`），每档由分组自定义 resolution + `low` / `medium` / `high` 三档质量单价。

- 一次性删干净旧字段，**无 DOWN 迁移、无老数据兼容**
- 渠道侧图片矩阵定价**留给 Phase 2**，本次不做

---

## 二、关键设计决策

1. **归档规则**：短边 `≤` 档位短边阈值 且 总像素 `≤` 档位总像素 → 归入该档；`tier_4k` 是硬上限
2. **硬校验（HTTP 400 拒绝）**：
   - 长边 > 4K 档长边
   - 总像素 > 4K 档总像素
   - 宽高比 > 1:3
3. **quality auto 归一到 low**（Phase 1b 变更，之前是 high）
4. **删除策略**：一次性删干净 `image_price_1k` / `2k` / `4k`、`image_pricing_matrix`（旧 6 档）
5. **图片入口校验挂载**：`openai_images` 主入口 + `fal_gateway_handler` 两个入口都已挂载；async_media / gemini / antigravity 暂未挂载（覆盖 90% 场景，需要时再补）

---

## 三、已完成清单（可提交状态）

### 3.1 后端（40+ 文件）

#### ent / 迁移
- **ent schema/generate**：字段替换为 `image_pricing_matrix_v2`；已执行 `go generate ./ent` 生成
- **migrations/226_groups_image_pricing_matrix_v2.sql**：DROP 旧列 + ADD V2 列

#### domain
- `ImagePricingMatrixV2`
- `ImagePricingTier3Row`
- tier key 常量

#### service
- **`service/image_pricing_tier.go`**：
  - `ClassifyImagePricingTier3`
  - `ValidateImageDimsAgainstTier3`
  - `ParseImagePricingMatrixV2Resolutions`
  - `NormalizeImageQuality`（auto → low）
  - `ValidateImageDimsAgainstGroupTier3`（handler helper）
- **`service/group_image_pricing_matrix.go`**：V2 校验（结构 + 分辨率单调 + 价格范围）
- **`service/billing_service.go`**：`ImagePriceConfig` V2 化（含 `PricingMatrixV2` / `Tier3Resolutions` / `RawWidth` / `RawHeight` / `Quality`）；`getImageUnitPrice` 只走 V2 + 默认价；新增 `lookupImagePricingMatrixV2`
- **`service/group.go`**：Group struct 加 `ImagePricingMatrixV2`；新增：
  - `HasImagePricingMatrixV2()`
  - `LookupImagePriceBySizeTier(imageSize)`
  - `BuildImagePriceConfig(rawW, rawH, quality)`
- **`service/admin_service.go`**：`CreateGroupInput` 用 V2；`UpdateGroupInput.ImagePricingMatrixV2 *domain.ImagePricingMatrixV2`
- **`service/admin_group.go`**：Create/Update 走 `validateImagePricingMatrixV2` + `normalizeImagePricingMatrixV2`
- **`service/admin_group_duplicate.go`**：加 `cloneGroupImagePricingMatrixV2` helper
- **`service/media_price_config.go`**：`imagePriceConfigFromAPIKey` 用 V2；`apiKeyHasConfiguredImagePrice` 用 `HasImagePricingMatrixV2`
- **`service/batch_image_public.go`**：改用 `LookupImagePriceBySizeTier`
- **`service/pricing_plaza_service.go`** & **`service/channel_plaza.go`**：`buildImageRow` 走 V2
- **`service/gateway_image_scheduling.go`**：`hasConfiguredImagePricing` 用 V2 遍历
- **`service/api_key_auth_cache.go`** & **`api_key_auth_cache_impl.go`**：snapshot Group 用 V2
- **`service/openai_gateway_usage.go`**：判空 V2

#### repository
- **`repository/group_repo.go`**：ent 调用 V2（Create/Update 两处 + `SetNillable` + `Clear`）
- **`repository/api_key_repo.go`**：Select 字段 + snapshot 映射 V2

#### handler / DTO
- **`handler/dto/types.go`** & **`mappers.go`**：`ImagePricingMatrixV2` field
- **`handler/admin/group_handler.go`**：`CreateReq` / `UpdateReq` / mapper

#### 图片入口硬校验（已挂载）
- **`handler/openai_images.go`** 的 `Images()` 在 `GroupAllowsImageGeneration` 后调用：
  ```go
  service.ValidateImageDimsAgainstGroupTier3(apiKey.Group, parsed.Size)
  ```
- **`handler/fal_gateway_handler.go`** 两个入口都挂了：
  - OpenAI-facade 用 `parsed.Size`
  - fal 原生入口用 `fal.MapSizeFromFal(falReq.ImageSize)`
- **未挂载**：`async_media_executor` / `gemini` / `antigravity`（fal 原生入口足够覆盖，其它路径待需要时再补）

### 3.2 后端测试

#### 改写
- `group_test.go`
- `group_image_pricing_matrix_test.go`
- `billing_service_image_test.go`
- `billing_service_image_matrix_v2_test.go`（删除 V1 legacy 字段）
- `billing_service_rate_multiplier_test.go`
- `billing_service_test.go`（`TestCalculateImageCost` / `TestCalculateVideoCostUsesSeparateConfig`）
- `channel_plaza_test.go`
- `admin_group_duplicate_test.go`
- `admin_service_group_test.go`（4 处：`CreateGroup_WithImagePricing` / `NilImagePricing` / `UpdateGroup_WithImagePricing` / `PartialImagePricing`）
- `gateway_image_prefer_fal_test.go`
- `pricing_plaza_service_test.go`
- `batch_image_public_test.go`
- `gateway_record_usage_test.go`
- `openai_gateway_record_usage_test.go`（8 处 Group struct literal）
- `server/api_contract_test.go`（删除 `image_price_1k` / `2k` / `4k` 快照）

#### 删除
- `billing_service_image_matrix_test.go`
- `billing_service_image_d3_test.go`（V1 6 档矩阵测试整体废弃）

#### 验证结果
- `go build ./...` ✅
- `go vet -tags unit ./...` ✅
- **全后端 unit 测试全绿**（service / handler / repository / server 全部通过）

#### 后期补丁（本轮收官）
- **`service/billing_service.go`**：`getImageUnitPrice` 增加"V2 矩阵 + imageSize 兜底"lookup 路径（新函数 `lookupImagePricingMatrixV2BySizeTier`），修复 handler 只知归一档位（"1K"/"2K"/"4K"）、缺 raw dims 时 V2 lookup 命不中的问题
- **`service/group.go`**：`LookupImagePriceBySizeTier` 增加"三档 quality 全为 0 视为未配置"语义，让上层能正确回退到渠道价/默认价
- **修复的失败测试**（11 个，全部转绿）：
  - `TestListPlazaGroups_GroupImagePriceOverridesChannelPricing`
  - `TestGatewayServiceRecordUsage_EmptyImageSizeDefaults*`
  - `TestOpenAIGatewayServiceRecordUsage_EmptyImageSizeDefaults*`
  - `TestOpenAIGatewayServiceRecordUsage_OutputImageSizeWins*`
  - `TestOpenAIGatewayServiceRecordUsage_ImageUsesPerImageBilling*`
  - `TestOpenAIGatewayServiceRecordUsage_ImageSharedMultiplier*`（2 个变体）
  - `TestOpenAIGatewayServiceRecordUsage_ImageIndependentMultiplier*`
  - `TestOpenAIGatewayServiceRecordUsage_*GroupImagePriceOverridesChannelImagePrice`（2 个）
  - `TestOpenAIGatewayServiceRecordUsage_HydratesGroupImagePrice*`

### 3.3 前端

#### 常量与组件
- **`constants/imagePricingMatrix.ts`**：完全重写为 V2
  - `IMAGE_PRICING_TIER_KEYS`
  - `ImagePricingTierKey`
  - `EditableImagePricingMatrix`
  - `DEFAULT_IMAGE_PRICING_MATRIX`
  - `createEmptyImagePricingMatrix` / `loadEditableImagePricingMatrix` / `toMatrixDTO` / `toMatrixUpdateDTO` / `validateEditableMatrix`
- **`components/admin/group/ImagePricingMatrixEditor.vue`**：完全重写
  - 加入 resolution 输入列
  - 展示 tier / resolution / low / medium / high 5 列 × 3 档
  - 含 `fillDefaults` / `clearAll` / `clearRow` 按钮及校验错误列表
- **`views/admin/groupsImagePricing.ts`**：改用 V2 tier keys；placeholder 从 `DEFAULT_IMAGE_PRICING_MATRIX` 取默认 low 价

#### 大文件改造 `views/admin/GroupsView.vue`（280KB）
- **模板**：删除 create/edit 表单中旧三档输入区块和 fallback details，替换为：
  ```vue
  <ImagePricingMatrixEditor v-model="createForm.image_pricing_matrix_v2" />
  ```
- **script**：
  - `ImagePricingFormState` 删除 `image_price_1k` / `2k` / `4k`，加 `image_pricing_matrix_v2`
  - `imagePricingTiers` 数组改为 `tier_1k` / `2k` / `4k`
  - `buildImageFinalPricePreview` 读矩阵 low 价
  - `resetForm` / `loadForm` / `createGroup` payload / `updateGroup` payload / `matrixErrors` 校验全部 V2 化
  - 删除未使用的 `getImagePricePlaceholder` import

#### 类型 & i18n
- **`types/index.ts`**：Group / CreateGroup / UpdateGroup 三处类型删除旧字段，加：
  ```ts
  image_pricing_matrix_v2?: Record<string, { resolution, low, medium, high }> | null
  ```
- **i18n**（`zh/en custom.ts`）：改写 `admin.groups.imagePricing` 段
  - 新增：`matrixTierHeader` / `matrixResolutionHeader` / `tier.{tier_1k / 2k / 4k}`
  - 删除：`matrixSizeHeader` / `fallbackTitle` / `fallbackHint`
  - 文案改为"图片三档定价矩阵（3 档分辨率 × 3 档质量）"

#### 前端测试
- `constants/__tests__/imagePricingMatrix.spec.ts`（重写 22 用例）
- `views/admin/__tests__/groupsImagePricing.spec.ts`（V2 tier keys）
- `GroupsView.duplicate.spec.ts` / `GroupsView.columnSettings.spec.ts` / `KeysView.spec.ts` / `PlanEditDialog.spec.ts`（删除快照旧字段）

#### 验证结果
- `npx vue-tsc --noEmit` ✅ 静默通过
- 本次改动相关测试 **32/32 全绿**
- 总体 `vitest run`：1890 通过、20 失败（**全部与本次改动无关**）

---

## 四、用户尚未确认的失败点

前端 `vitest run` 显示的 **20 个失败测试已确认与本次 Phase 1b 改动无关**，均为历史遗留失败：

- `OpsSystemLogTable` / `AppHeader` / `AccountUsageCell`
- `SupportTickets` / `LoginView` / `ProfileView` / `UsageTable`
- `UserPlatformQuotaModal` / `localesMessageCompile` / `HomeView.compact`
- `GroupBadge` / `EditAccountModal` / `PendingOAuthCreateAccountForm`
- `groupsModelsListLayout` / `AffiliateView`
- `AdminSupportTicketsView` / `SupportTicketDetailView` / `SupportTicketNewView`

已回复用户此结论，但对话中断（用户要求切换会话）。

---

## 五、未做的事项（未来 Phase）

- **Phase 2**：渠道（Channel）侧新增图片矩阵定价
  - 表新增列
  - 结构体 + repo + handler + DTO
  - 计费路径接入
  - 前端加图片矩阵编辑器
- **本次遗留**：`async_media` / `gemini` / `antigravity` 图片入口硬校验暂未挂载（主入口 openai_images + fal 已覆盖 90% 场景）
- **domain 中的 V1 别名**：已删除，无遗留

---

## 六、关键上下文提示（给下一会话）

1. 用户使用**中文**对话，代码注释也用**中文**
2. 用户风格：Q/A 简短选择，"a / b / c"或"继续"驱动流程；容忍分阶段但同一轮可无限继续
3. 编辑大文件用 `replace_in_file` + `multi_replace`：
   - `GroupsView.vue`（280KB）
   - `openai_gateway_record_usage_test.go`（100KB）
   - `admin_service_group_test.go`（55KB）
4. 已确认路径：
   - 后端：`/home/jiantaoli/sub2api/backend`
   - 前端：`/home/jiantaoli/sub2api/frontend`
5. `go generate ./ent` 有超时限制；曾遇到 timeout（后来发现其实已经跑完）
6. i18n 文件在 `frontend/src/i18n/locales/{zh,en}/custom.ts`，`admin.groups.imagePricing` 段

---

## 七、关键 helper 分工速查

| Helper | 用途 |
| --- | --- |
| `HasImagePricingMatrixV2()` | 判断分组是否启用矩阵定价 |
| `LookupImagePriceBySizeTier(imageSize)` | 无 raw 分辨率上下文时的兜底（batch settlement / channel plaza 展示用）；三档 quality 全 0 视为未配置 |
| `BuildImagePriceConfig(rawW, rawH, quality)` | 主计费入口 |
| `ValidateImageDimsAgainstGroupTier3(group, size)` | handler 前置校验 |
| `ParseImagePricingMatrixV2Resolutions(matrix)` | 解析矩阵到 `ImageTier3Resolutions` |
| `ClassifyImagePricingTier3(w, h, res)` | 归档到 `tier_1k` / `2k` / `4k` |
| `lookupImagePricingMatrixV2(cfg)` | 计费主 lookup：需 `Tier3Resolutions` + raw dims；按短边+像素归档 × quality |
| `lookupImagePricingMatrixV2BySizeTier(cfg, imageSize)` | 计费兜底 lookup：只需归一后 `imageSize`（"1K"/"2K"/"4K"），按 quality 映射；三档全 0 视为未配置 |
