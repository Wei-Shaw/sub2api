# 图片生成多角度输出 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 为图片预设增加“每个角度单独生成一张”选项，在单张多视角拼图和同一结果消息内的多张独立角度图片之间切换。

**Architecture:** 扩展 `ImagePresetSelection` 保存布尔选择，`PromptComposer` 只负责显示和更新控件，`useImageGeneration` 根据选择构造一个普通拼图请求或一个包含多个 `variants` 的请求。后端继续用单条历史记录处理 variants，并把每个子项的标签和成功/失败状态投影到同一条助手消息。

**Tech Stack:** Vue 3、TypeScript、Vitest、Vue Test Utils、Go、`net/http/httptest`

---

### Task 1: 独立角度复选框状态与交互

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/types/index.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/components/PromptComposer.vue`
- Modify: `plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts`

- [ ] **Step 1: 写复选框显示、切换和清空的失败测试**

在 `components.spec.ts` 中挂载含两个角度的 `PromptComposer`，断言存在 `data-testid="separate-angle-images"`、默认未勾选，勾选后发出包含 `separateAngleImages: true` 的 `update:presetSelection`；再触发“清空”，断言发出完整空状态：

```ts
{
  styles: [], scenes: [], effects: [], angles: [], separateAngleImages: false,
}
```

另挂载只有一个角度的组件，断言该复选框不存在。

- [ ] **Step 2: 运行组件测试并确认按预期失败**

Run: `npm run test -- src/components/components.spec.ts`

Working directory: `plugin-service/plugins/image-generation/frontend`

Expected: FAIL，原因是 `separate-angle-images` 尚不存在，且类型没有 `separateAngleImages`。

- [ ] **Step 3: 扩展类型和全部默认预设状态**

将 `ImagePresetSelection` 改为：

```ts
export interface ImagePresetSelection {
  styles: string[]
  scenes: string[]
  effects: string[]
  angles: string[]
  separateAngleImages: boolean
}
```

把 `emptyPresetSelection()`、组件 props 默认值以及所有清空路径统一设置为 `separateAngleImages: false`。`presetCount` 只统计数组项，不把布尔值计入已选预设数量。

- [ ] **Step 4: 实现角度区域复选框和动态说明**

在角度 fieldset 后、操作按钮前，仅当 `presetSelection.angles.length > 1` 时渲染：

```vue
<label class="preset-angle-output-option">
  <input
    type="checkbox"
    data-testid="separate-angle-images"
    :checked="presetSelection.separateAngleImages"
    @change="toggleSeparateAngleImages"
  >
  <span>每个角度单独生成一张</span>
</label>
```

`toggleSeparateAngleImages` 发出完整的新对象。说明文案根据布尔值分别显示“一张多视角拼图”或“每个角度一张，结果显示在同一条消息中”。

- [ ] **Step 5: 运行组件测试并确认通过**

Run: `npm run test -- src/components/components.spec.ts`

Expected: PASS。

- [ ] **Step 6: 提交交互增量**

```bash
git add plugin-service/plugins/image-generation/frontend/src/types/index.ts plugin-service/plugins/image-generation/frontend/src/components/PromptComposer.vue plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts
git commit -m "feat: add separate angle image option"
```

### Task 2: 单张拼图与单请求 variants 提交

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/types/index.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`

- [ ] **Step 1: 写未勾选时单张拼图请求的失败测试**

选择 `front`、`back` 且 `separateAngleImages: false` 后提交，断言 `api.generate` 只调用一次，请求为 `output_count: 1`、不含 `variants`，且 prompt 同时包含“正面”“背面”“多视角拼图”和按选择顺序排列的要求。

- [ ] **Step 2: 写勾选时单请求 variants 的失败测试**

选择相同角度且 `separateAngleImages: true` 后提交，断言 `api.generate` 只调用一次，请求包含：

```ts
variants: [
  { label: '正面', prompt: expect.stringContaining('角度：正面') },
  { label: '背面', prompt: expect.stringContaining('角度：背面') },
]
```

每个 variant prompt 不得包含另一个角度或“多视角拼图”。同时断言提交期间只有一条 pending 助手消息，完成后只有一条助手消息且其中有两张按正面、背面排列的图片。

- [ ] **Step 3: 运行 composable 定向测试并确认失败**

Run: `npm run test -- src/composables/useImageGeneration.spec.ts`

Expected: FAIL；当前实现会为 GPT 角度创建多个顶层请求和多条助手消息，未勾选模式也没有拼图语义。

- [ ] **Step 4: 分离角度 prompt 构造函数**

在 composable 中保留 `angleVariants(basePrompt)` 作为独立角度 prompt 构造器，并新增拼图 prompt 构造器。拼图 prompt 需要生成稳定文本，例如：

```ts
`角度：${labels.join('、')}（生成在同一张多视角拼图中，按此顺序排列；每个分区只展示对应角度）`
```

单角度仍沿用普通单图 prompt，不显示拼图要求。

- [ ] **Step 5: 把一次用户提交固定为一个顶层任务和一条等待消息**

删除多角度分支对 `descriptors.map` 的顶层并发提交。构造一个 `GenerateRequest`：未勾选时 `output_count: 1` 且无 `variants`；勾选时 `output_count` 为角度数并携带 variants。只创建一个 `ActiveGenerationTask` 和一个 pending 消息，完成时用 `terminalMessage` 一次性替换。

普通 GPT 多数量生成保持既有行为，不属于本功能；仅多角度分支改为单顶层请求。

- [ ] **Step 6: 确保返回图片按 variant 顺序并带标签**

`imagesFromResult` 优先使用服务端 `variant_label`。若缺失且请求使用 variants，则只按结果索引补充对应请求标签；索引越界时不猜测角度，保留 revised prompt 或通用标题。

在 `GenerationResult` 增加可选字段 `failed_variants?: string[]`。`terminalMessage` 在存在成功图片和失败角度时仍返回成功图片，并把内容设置为 `生成结果\n生成失败的角度：${failedVariants.join('、')}`。在 composable 测试中构造“一张正面成功、背面失败”的响应，断言最终仍只有一条助手消息、保留正面图片且正文包含“背面”。

- [ ] **Step 7: 运行 composable 测试并确认通过**

Run: `npm run test -- src/composables/useImageGeneration.spec.ts`

Expected: PASS，包括现有普通多图、取消和重试测试。

- [ ] **Step 8: 提交请求与聚合结果增量**

```bash
git add plugin-service/plugins/image-generation/frontend/src/types/index.ts plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.ts plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts
git commit -m "feat: group multi-angle generation results"
```

### Task 3: 历史记录恢复和角度标题展示

**Files:**
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/conversationMessages.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts`
- Modify: `plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts`

- [ ] **Step 1: 写历史记录投影失败测试**

创建一条 succeeded `HistoryRecord`，`result.images` 含正面和背面两个 `variant_label`，断言 `projectConversationMessages` 只返回一条 user 和一条 assistant，且 assistant 的两张图片依序具有 `variantLabel: '正面'`、`variantLabel: '背面'`。

- [ ] **Step 2: 写图片卡角度标题优先级测试**

挂载 `GeneratedImageCard`，同时提供 `variantLabel: '正面'` 和较长 `revisedPrompt`，断言 figcaption 只显示“正面”。

- [ ] **Step 3: 运行测试并确认失败点真实**

Run: `npm run test -- src/composables/useImageGeneration.spec.ts src/components/components.spec.ts`

Expected: 新增测试若现有行为已满足则应先调整测试覆盖缺失标签回退；最终至少一个测试应因新状态或回退行为缺失而 FAIL，不能跳过红灯确认。

- [ ] **Step 4: 最小化补齐历史标签回退**

历史投影继续以一条记录生成一条助手消息。`images(record)` 优先保留 `image.variant_label`；仅当标签缺失且 `record.request.variants` 对应索引存在时使用该 variant 的 label，禁止根据 revised prompt 猜测角度。

- [ ] **Step 5: 运行测试并确认通过**

Run: `npm run test -- src/composables/useImageGeneration.spec.ts src/components/components.spec.ts`

Expected: PASS。

- [ ] **Step 6: 提交历史恢复增量**

```bash
git add plugin-service/plugins/image-generation/frontend/src/composables/conversationMessages.ts plugin-service/plugins/image-generation/frontend/src/composables/useImageGeneration.spec.ts plugin-service/plugins/image-generation/frontend/src/components/components.spec.ts
git commit -m "fix: preserve angle labels in image history"
```

### Task 4: 后端 variants 部分成功结果

**Files:**
- Modify: `plugin-service/plugins/image-generation/backend/generation_service.go`
- Modify: `plugin-service/plugins/image-generation/backend/generation_service_test.go`

- [ ] **Step 1: 写 batch variants 部分成功的失败测试**

用 `httptest.Server` 模拟 batch 状态 completed、两个 item 中正面 completed 且背面 failed。调用 `Generate` 后轮询 `Status`，断言记录最终保留正面图片及 `variant_label: 正面`，并在 result 中包含失败角度列表 `failed_variants: ['背面']`，而不是返回 `completed batch image item is missing`。

- [ ] **Step 2: 运行后端定向测试并确认失败**

Run: `go test ./plugins/image-generation/backend -run TestGenerationService_BatchVariantsKeepSuccessfulImages -count=1`

Working directory: `plugin-service`

Expected: FAIL，当前 `completeBatchResult` 遇到任一非 completed item 就返回错误。

- [ ] **Step 3: 收集成功图片和失败角度而不中断循环**

调整 `completeBatchResult`：对 variant item，非 completed 或 `ImageCount < 1` 时把 `variantLabels[item.CustomID]` 加入 `failedVariants` 并继续；成功 item 继续归档。非 variant 的单项任务仍沿用原来的严格失败行为。

将结果补充为：

```go
if len(failedVariants) > 0 {
    record.Result["failed_variants"] = failedVariants
}
```

至少有一张成功图片时函数返回 nil；没有任何成功图片时仍返回错误，避免把全失败标记为成功。

- [ ] **Step 4: 补充全失败回归测试**

同一测试文件增加所有 variant 均失败的用例，断言状态为 failed 且不伪造 succeeded 结果。

- [ ] **Step 5: 运行后端测试并确认通过**

Run: `go test ./plugins/image-generation/backend -count=1`

Working directory: `plugin-service`

Expected: PASS。

- [ ] **Step 6: 提交后端增量**

```bash
git add plugin-service/plugins/image-generation/backend/generation_service.go plugin-service/plugins/image-generation/backend/generation_service_test.go
git commit -m "fix: retain successful angle variants"
```

### Task 5: 全量验证和生成静态资源

**Files:**
- Modify generated output as produced by the existing frontend build: `plugin-service/plugins/image-generation/web/assets/app.js`
- Modify generated output as produced by the existing frontend build: `plugin-service/plugins/image-generation/web/assets/app.css`

- [ ] **Step 1: 运行前端完整测试**

Run: `npm run test`

Working directory: `plugin-service/plugins/image-generation/frontend`

Expected: PASS，无失败测试。

- [ ] **Step 2: 运行类型检查和生产构建**

Run: `npm run build`

Working directory: `plugin-service/plugins/image-generation/frontend`

Expected: PASS，`vue-tsc` 无错误，Vite 更新 `../web` 中的静态资源。

- [ ] **Step 3: 验证生成资源已同步**

Run: `npm run verify:generated`

Working directory: `plugin-service/plugins/image-generation/frontend`

Expected: PASS。

- [ ] **Step 4: 运行 Go 插件服务测试**

Run: `go test ./...`

Working directory: `plugin-service`

Expected: PASS。

- [ ] **Step 5: 启动插件服务并做浏览器验证**

按仓库 `plugin-service/README.md` 的本地启动方式启动服务；用浏览器验证桌面和移动宽度下：两个角度时出现复选框，未勾选说明拼图，勾选说明独立图片；结果网格不溢出且角度标题与图片卡一一对应。记录浏览器控制台无错误。

- [ ] **Step 6: 检查最终差异并提交生成资源**

Run: `git diff --check`

Expected: 无输出且退出码为 0。

```bash
git add plugin-service/plugins/image-generation/web/assets/app.js plugin-service/plugins/image-generation/web/assets/app.css
git commit -m "build: update image generation frontend assets"
```
