## ADDED Requirements

### Requirement: 用户必须能够读取 Web3 充值配置
系统 SHALL 为已认证用户提供 Web3 充值配置 API，返回功能状态、网络、chain ID、USDT0 Token 合约、decimals、最低充值、自动入账上限、手续费和最终性说明。金额字段 MUST 使用 JSON string。

#### Scenario: 功能启用
- **WHEN** 用户读取 Web3 充值配置且 runtime 配置有效
- **THEN** 响应 MUST 表明网络为 Conflux eSpace、chain ID 为 1030、Token 为固定 USDT0 合约
- **THEN** 最低充值 MUST 为字符串 `1.000000`
- **THEN** 自动入账上限 MUST 为字符串 `10000.000000`

#### Scenario: 功能关闭或 runtime unhealthy
- **WHEN** Web3 Deposit 功能关闭或启动预检失败
- **THEN** 配置 API MUST 返回不可用状态和稳定原因码
- **THEN** 用户 MUST NOT 能创建新充值地址

### Requirement: 用户地址 API 必须区分查询与创建
系统 SHALL 提供不产生写操作的地址查询 API 和幂等 get-or-create 地址 API，并 MUST 复用现有 JWT、用户后端模式守卫和面板限流。

#### Scenario: GET 查询未分配地址
- **WHEN** 用户调用 GET address 且尚无地址
- **THEN** 响应 MUST 返回未分配状态
- **THEN** 系统 MUST NOT 创建地址或消耗 derivation index

#### Scenario: POST 创建地址
- **WHEN** 用户调用 POST address 且功能健康
- **THEN** 系统 MUST 幂等创建或返回该用户长期地址
- **THEN** 响应 MUST NOT 包含 xpub、wallet fingerprint 或 derivation index

### Requirement: 用户必须只能查看自己的充值记录
系统 SHALL 提供当前用户的充值列表和详情 API。查询 MUST 按认证 user ID 限定范围，金额使用字符串，并 MUST 隐藏内部租约、RPC 错误栈和钱包派生信息。

#### Scenario: 查询自己的充值历史
- **WHEN** 用户请求充值列表
- **THEN** 系统 MUST 只返回该用户地址对应的充值
- **THEN** 列表 MUST 使用稳定排序和分页，包含金额、tx hash、展示状态和时间

#### Scenario: 枚举其他用户充值 ID
- **WHEN** 用户请求不属于自己的充值详情 ID
- **THEN** 系统 MUST 返回 not found 或 forbidden
- **THEN** 响应 MUST NOT 泄露该充值是否存在、所属地址或金额

### Requirement: 用户界面必须明确网络和误充值风险
用户充值页面 SHALL 同时展示 `Conflux eSpace`、用户 `0x` 地址、固定 USDT0 Token 合约、最低金额、finalized 后到账说明和风险警告。页面 MUST NOT 只显示 symbol 而隐藏合约或网络。

#### Scenario: 展示充值地址
- **WHEN** 用户地址已经分配
- **THEN** 页面 MUST 分别提供地址和 Token 合约复制操作
- **THEN** 二维码 MUST 只编码用户充值地址

#### Scenario: 展示风险提示
- **WHEN** 用户打开充值页面
- **THEN** 页面 MUST 提示不要使用 Conflux Core Space、其他网络或其他同名 Token
- **THEN** 页面 MUST 说明低于 1 USDT0 不自动入账且误充值不自动退款

### Requirement: 用户展示状态必须隐藏内部处理细节
系统 SHALL 将内部充值状态映射为用户可理解的 `confirming`、`credited`、`below_minimum`、`under_review` 和 `failed`，并 MUST 提供检测、最终确认和到账时间中的可用字段。

#### Scenario: 内部状态为 ready_to_credit 或 crediting
- **WHEN** 用户查询处于内部 `ready_to_credit` 或 `crediting` 的充值
- **THEN** 用户状态 MUST 显示为 confirming 或等价处理中状态
- **THEN** 响应 MUST NOT 暴露 lease version 或 Worker claim 信息

#### Scenario: 充值到账
- **WHEN** 内部状态为 credited
- **THEN** 用户状态 MUST 显示为 credited
- **THEN** 页面 MUST 显示精确到账 USD 金额和到账时间

### Requirement: 管理员必须能够查询充值和运行状态
系统 SHALL 提供管理员充值列表、详情和 runtime API，支持按状态、用户、地址、tx hash 和时间筛选，并展示 RPC 健康、leader、latest/finalized/scanned block 和 lag。

#### Scenario: 管理员查看待处理充值
- **WHEN** 管理员筛选 manual_review 或 failed
- **THEN** 系统 MUST 返回链上事实、当前验证状态、失败原因和 Web3 子账户入账结果

#### Scenario: 管理员查看 runtime
- **WHEN** 管理员读取 runtime 状态
- **THEN** 响应 MUST 包含 endpoint ID、健康状态、leader 和游标高度
- **THEN** 响应 MUST NOT 包含完整 xpub、私钥或带凭据的 RPC URL

### Requirement: 管理操作必须受鉴权、二次确认和审计保护
管理员批准、忽略、重试和区间补扫 SHALL 复用现有管理员鉴权、所需 step-up 和管理操作审计。忽略和批准 MUST 要求明确确认，忽略 MUST 要求原因。

#### Scenario: 非管理员尝试批准
- **WHEN** 非管理员或缺少要求 step-up 的会话提交批准请求
- **THEN** 系统 MUST 拒绝操作
- **THEN** 充值状态和余额 MUST 保持不变

#### Scenario: 管理员忽略充值
- **WHEN** 管理员二次确认并提供原因后忽略允许处置的充值
- **THEN** 系统 MUST 记录管理员、原因、旧状态和新状态
- **THEN** 系统 MUST NOT 自动退款或移动链上资金

### Requirement: 补扫操作必须限制区间且不得任意改写游标
系统 SHALL 允许管理员创建受限区间的补扫任务，但 MUST 拒绝未来区块、超过最大跨度或低于安全下界的请求。补扫 MUST 复用事件唯一键，不得直接把生产游标设置为任意值。

#### Scenario: 创建合法补扫任务
- **WHEN** 管理员提交位于允许范围内的历史区块区间
- **THEN** 系统 MUST 创建可审计的补扫任务
- **THEN** 补扫发现的已有事件 MUST 被幂等去重

#### Scenario: 请求过大或非法区间
- **WHEN** 区间终点早于起点、位于未来或超过配置最大跨度
- **THEN** 系统 MUST 拒绝请求并返回稳定错误码
- **THEN** 主 scanner cursor MUST 保持不变

### Requirement: 页面必须满足响应式和国际化要求
用户与管理员 Web3 充值页面 SHALL 使用现有 Vue、组件和 i18n 体系，新增文案 MUST 覆盖项目要求的语言，并 MUST 在桌面和窄屏下保持地址、交易哈希、状态和操作可访问。

#### Scenario: 窄屏查看长地址
- **WHEN** 页面宽度较窄且地址或 tx hash 超过容器宽度
- **THEN** 页面 MUST 使用可复制的截断或换行展示
- **THEN** 关键操作 MUST 不被遮挡

#### Scenario: 页面离开
- **WHEN** 用户离开充值页面
- **THEN** 页面 MUST 停止充值状态 polling
- **THEN** 后台计时器和请求 MUST 被清理
