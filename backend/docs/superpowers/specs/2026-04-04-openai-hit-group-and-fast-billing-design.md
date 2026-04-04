# OpenAI 命中账号组与 Fast 单价计费设计

## 背景

当前系统已经具备以下能力：

1. OpenAI 请求在调度后会记录最终命中的 `routing_target_group`。
2. OpenAI 请求会记录 `service_tier`，其中 `priority` 对应用户可理解的 Fast 模式。
3. 现有计费链路已经区分：
   - `total_cost`：标准费用
   - `actual_cost`：应用倍率后的实际费用
   - `rate_multiplier`：分组/用户专属倍率
   - `account_rate_multiplier`：账号倍率
4. `priority` 服务档本身在定价层已经与标准档不同，例如 `gpt-5.4` 当前在 `billing_service.go` 中：
   - 输入：标准 `$2.5 / MTok`，priority `$5 / MTok`
   - 输出：标准 `$15 / MTok`，priority `$30 / MTok`
   - Cache Read：标准 `$0.25 / MTok`，priority `$0.5 / MTok`

用户这次的最终要求已经收敛为：

1. **优先账号倍率保留**：最终命中非耗尽组时，仍然按 `100x` 计费。
2. **Fast 不再额外乘 2**：Fast 的价格差异只来自当前已有的 `priority` 单价本身，不再在其上额外乘一层 `2x`。
3. **Fast 必须可见**：用户要知道这条请求命中了 Fast / priority 单价。
4. **展示重点改为“实际单价 + 价格来源 + 倍率因子”**。
5. **用户侧不能知道 exhausted/non-exhausted 这套内部概念**，但要能理解为什么某条请求更贵，并得到 `-Sys` 引导。

## 目标

本次设计目标是：

1. 把 OpenAI 请求的实际费用计算收敛为：
   - 基础倍率 `rate_multiplier`
   - 账号倍率 `account_rate_multiplier`
   - 优先账号倍率 `priority_account_multiplier`（命中非耗尽组时 = 100，否则 = 1）
   - **不再引入 Fast 额外倍率**
2. 在 usage 写库时固化：
   - 优先账号倍率
   - 最终倍率
   - 实际使用的输入/输出/缓存读单价
   - Fast/priority 价格来源标记
   - 最终 `actual_cost`
3. 让余额计费和订阅计费统一使用这套新规则。
4. 让用户侧明细、统计卡片、订阅用量都统一读取新口径。
5. 让用户能看到“价格为什么变贵”，但看不到 exhausted / non-exhausted 这类内部术语。

## 非目标

本次不做以下事情：

1. 不改变 OpenAI 调度本身的 active / exhausted 规则。
2. 不在用户侧直接暴露 `routing_target_group` 原值。
3. 不把 Fast 再抽象成新的后端调度概念；这轮它只是价格来源与展示标记。
4. 不引入新的独立计费表；优先扩展现有 usage / subscription / stats 结构。

## 计费行为契约

### 1. 最终费用的组成

OpenAI 请求最终费用只由以下因子决定：

1. `base_multiplier`
   - 来自当前的 `rate_multiplier`
2. `account_multiplier`
   - 来自当前的 `account_rate_multiplier`
3. `priority_account_multiplier`
   - 命中**非耗尽组**时 = `100`
   - 否则 = `1`
4. `service_tier` 影响的**是单价档位**，不是额外倍率：
   - `standard/default` 单价
   - `priority` 单价

因此最终倍率为：

```text
effective_multiplier =
  base_multiplier × account_multiplier × priority_account_multiplier
```

最终实际费用为：

```text
actual_cost = service-tier-adjusted total_cost × effective_multiplier
```

这里的 `total_cost` 已经是按当前 `service_tier` 单价算出的标准费用，因此：

- `priority` 不再额外乘 `2x`
- Fast 的差异只体现在使用了 `priority` 单价，而不是单独再乘一层业务倍率

### 2. 优先账号的用户语义

虽然内部实现是“命中非耗尽组时乘 100”，但用户侧统一解释为：

- `优先账号 100x`

并附带引导：

- 如果希望降低这层额外费用，请优先使用 `-Sys` 变体。

### 3. Fast 的用户语义

用户侧不再展示“Fast 模式 2x”，而是展示：

- `Fast/priority 单价生效`

同时把实际输入/输出单价展示出来，让用户能看到：

- 这条请求为什么贵
- 贵在使用了哪一档单价

### 4. 余额计费与订阅计费统一

本次调整不区分余额用户与订阅用户：

1. 余额扣费按新 `actual_cost` 扣减。
2. 订阅累计（daily / weekly / monthly）按新 `actual_cost` 累加。
3. 两条链路共享同一套价格来源和最终费用结果。

## 数据层设计

### 1. usage 记录需要固化的新增信息

现有 usage 记录已保存：

- `rate_multiplier`
- `account_rate_multiplier`
- `actual_cost`
- `service_tier`
- `routing_target_group`

本次需要额外固化：

1. `priority_account_multiplier`
2. `effective_multiplier`
3. `effective_input_unit_price`
4. `effective_output_unit_price`
5. `effective_cache_read_unit_price`（如适用）
6. `pricing_source_flags` 或等价的价格来源字段，至少能表达：
   - 是否命中 `priority` 单价
   - 是否命中优先账号 100x

核心原则是：

- **Fast 记录为价格来源，不记录为额外乘算倍率**

### 2. 订阅累计与统计口径

用户订阅累计、用户统计卡片、管理员统计接口都统一基于新的 `actual_cost` 与新增价格来源字段。

不能采用“查询时再根据 `routing_target_group` / `service_tier` 临时反推”的方式，因为那会带来：

1. 明细与累计值不一致
2. 历史数据回填困难
3. 价格解释不稳定

因此本次采用：

- **写库时固化**
- **统计时直接读取**

### 3. 历史数据处理

已有 usage / subscription 数据当前不包含新价格来源字段，也未按最终规则统一累计。

因此这轮需要：

1. 对已有 usage_logs：补齐新字段，重算 `actual_cost`
2. 对相关订阅累计值：按回填后的 `actual_cost` 重算
3. 对用户/管理员统计接口：统一切到新口径

## 用户界面设计

### 1. 用户 Usage 明细

用户 Usage 明细建议至少展示：

1. 标准费用
2. 实际费用
3. 基础倍率
4. 账号倍率
5. 优先账号倍率（显示为“优先账号 100x”）
6. 实际输入单价
7. 实际输出单价
8. 实际缓存读单价（如适用）
9. 价格来源说明：
   - `Fast/priority 单价生效`
   - `优先账号 100x`
10. 最终倍率

目标是让用户能直观看到：

```text
基础倍率 × 账号倍率 × 优先账号倍率 = 最终倍率
```

以及：

```text
本次请求实际使用了哪一档输入/输出单价
```

### 2. 用户统计卡片和订阅用量

用户侧统计卡片、订阅累计、总费用汇总都统一显示**新规则下的实际费用**。

### 3. 用户文案要求

文案层必须满足：

1. 能解释价格原因
2. 不暴露内部调度分组概念

因此建议使用：

- `优先账号 100x`
- `Fast/priority 单价生效`
- `建议：如果希望降低优先账号倍率，请优先使用 -Sys 变体`

避免出现：

- `active`
- `exhausted`
- `非耗尽组`

## 风险与验证

### 风险 1：Fast 单价与最终费用脱节

如果只展示 `priority` 标记，不展示实际输入/输出单价，用户还是看不懂为什么 Fast 更贵。

### 风险 2：旧方案残留

当前工作区里已经出现过一版“Fast 额外乘 2”的实现思路。若不在正式实现前清理，会把错误规则带进后续任务。

### 风险 3：订阅累计混口径

如果 usage 明细切到新规则，但订阅累计没有一起回填，用户会看到互相打架的数字。

### 风险 4：内部概念泄漏

如果直接把 `routing_target_group` 原值暴露给用户，会泄漏内部路由设计。

## 验证方案

验证分 4 层进行：

### A. 后端单测

至少覆盖：

1. 命中非耗尽组时 `priority_account_multiplier = 100`
2. `service_tier = priority` 时不再额外乘 `2x`
3. 仍然会正确记录 `service_tier = priority` 与实际单价
4. 余额计费链路正确写入新 `actual_cost`
5. 订阅计费链路正确累计新 `actual_cost`

### B. 数据层验证

至少覆盖：

1. usage_logs 新倍率/单价字段写入正确
2. 订阅累计值与 usage 明细汇总一致
3. 用户统计接口和管理员统计接口都读取新口径

### C. 前端验证

至少覆盖：

1. 用户 Usage 明细展示实际单价、价格来源、倍率因子
2. 统计卡片和订阅页显示新规则下的实际费用
3. 文案只出现“优先账号 100x”“Fast/priority 单价生效”
4. 提示文案明确引导用户使用 `-Sys`

### D. 真实线上验证

至少验证 4 类真实请求：

1. 普通请求
2. 命中优先账号请求
3. `priority/Fast` 请求
4. 同时命中优先账号 + Fast 的请求

并确认：

1. usage_logs 明细正确
2. 用户侧统计正确
3. 订阅累计正确

## 最终结论

本次设计采用：

1. 在 usage 写入时固化新的优先账号倍率、最终倍率、实际单价与价格来源
2. Fast 不再作为额外乘算倍率，而是作为单价来源标记
3. 余额计费和订阅计费统一切换到新 `actual_cost`
4. 用户界面完整解释价格来源和倍率因子，但不暴露 exhausted / non-exhausted 内部概念
5. 通过回填/重算避免历史数据混口径

这套方案的核心价值是：

1. 账实一致
2. 统计口径一致
3. 用户能看懂“为什么贵”以及“贵在哪个价格来源”
4. 同时保留内部路由策略与用户展示语义的隔离
