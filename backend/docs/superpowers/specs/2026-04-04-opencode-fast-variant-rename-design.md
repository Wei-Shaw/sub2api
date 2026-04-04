# OpenCode Fast Variant 重命名设计

## 背景

这一轮已经给 `gpt-5.4` / `gpt-5.4-Sys` 增加了一组 Fast 组合 variant，用于把用户侧“Fast 模式 + 推理强度”映射到 wire-level 的：

- `serviceTier: "priority"`
- 对应 `reasoningEffort`

当前命名为：

- `fast-low`
- `fast-medium`
- `fast-high`
- `fast-xhigh`

用户希望把它调整成：

- `low-fast`
- `medium-fast`
- `high-fast`
- `xhigh-fast`

核心原因不是语义变化，而是让 OpenCode 的 variant 列表按“推理强度在前、Fast 在后”的排序方式更自然，更符合使用时的视觉与检索习惯。

## 目标

1. 把现有 Fast 组合 variant key 从 `fast-*` 改成 `*-fast`。
2. 保持行为完全不变：
   - 仍然只作用于 `gpt-5.4` / `gpt-5.4-Sys`
   - 仍然发送 `serviceTier: "priority"`
   - 仍然保留对应 `reasoningEffort`
3. 同步改动两个配置来源：
   - 本机 `C:\Users\34404\.config\opencode\opencode.jsonc`
   - `sub2api` 前端生成的 OpenCode 推荐配置

## 非目标

1. 不新增新的模型 ID。
2. 不保留新旧 variant 名并存的兼容期。
3. 不改 `sub2api` 后端对 `service_tier` / `priority` 的提取与归一化逻辑。
4. 不改文案、显示名称或其他排序逻辑，只改 variant key。

## 行为契约

这次是一次**纯命名重排**。

重命名前后，语义必须保持一致：

| 旧 key | 新 key | 请求语义 |
| --- | --- | --- |
| `fast-low` | `low-fast` | `serviceTier: "priority"` + `reasoningEffort: "low"` |
| `fast-medium` | `medium-fast` | `serviceTier: "priority"` + `reasoningEffort: "medium"` |
| `fast-high` | `high-fast` | `serviceTier: "priority"` + `reasoningEffort: "high"` |
| `fast-xhigh` | `xhigh-fast` | `serviceTier: "priority"` + `reasoningEffort: "xhigh"` |

这意味着：

1. 用户看到的只是 variant 名字变化。
2. 线上请求落线语义不变。
3. 本地 OpenCode 和 `sub2api` 推荐配置之间不能出现命名分叉。

## 实现边界

这次只允许修改 3 个位置：

1. `frontend/src/components/keys/UseKeyModal.vue`
2. `frontend/src/components/keys/__tests__/UseKeyModal.spec.ts`
3. `C:\Users\34404\.config\opencode\opencode.jsonc`

其中：

- `UseKeyModal.vue` 负责推荐配置生成
- `UseKeyModal.spec.ts` 负责锁定输出契约
- 本机 `opencode.jsonc` 负责让用户当前环境立即生效

## 风险

### 风险 1：只改了一边，另一边名字没同步

如果只改推荐配置生成器，没改本机配置，那么：

1. 新复制出来的推荐配置用的是 `low-fast`
2. 本机现有配置还停留在 `fast-low`
3. 用户会看到两套命名并存

控制方式：必须把 repo 内配置源和本机 `opencode.jsonc` 同步修改。

### 风险 2：命名改了但 payload 语义被误改

如果重命名时顺手动了 payload，就可能把“纯命名调整”变成“行为变化”。

控制方式：测试必须显式锁定：

- 新名字存在
- 旧名字消失
- `serviceTier: "priority"` 保持不变
- `reasoningEffort` 保持不变

## 验证方案

### A. 前端测试

验证推荐配置中：

1. `gpt-5.4` / `gpt-5.4-Sys` 下存在：
   - `low-fast`
   - `medium-fast`
   - `high-fast`
   - `xhigh-fast`
2. 对应 payload 仍然有 `serviceTier: "priority"`
3. 旧 key `fast-low` 不再出现
4. `gpt-5.2` 等其他模型仍然没有 Fast 组合项

### B. 前端构建

`pnpm build` 必须通过，确保推荐配置生成链路没有被命名改坏。

### C. 本机真实请求

本机 `opencode.jsonc` 改名后，用真实请求验证：

```text
opencode run -m sub2api-openai/gpt-5.4 --variant xhigh-fast "Reply only with OK."
```

预期：

1. 请求成功
2. 不会出现 `Unsupported service_tier: fast`
3. 说明这次只是 variant 命名调整，没有破坏现有 `priority` 线路

## 结论

这次调整应当被视为一次**纯命名整理**：

1. 只把 `fast-*` 改成 `*-fast`
2. 不改模型 ID
3. 不改 payload
4. 不改后端
5. 不做兼容双写

最终目标是让本机配置和 `sub2api` 推荐配置在命名上完全一致，同时更符合 OpenCode 的 variant 排序习惯。
