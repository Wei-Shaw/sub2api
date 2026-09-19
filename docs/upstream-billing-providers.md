# 上游对账供应商接入说明

这些接入读取供应商的费用接口，不是余额查询，也不会用 sub2api 的价格表冒充供应商账单。供应商费用通常延迟更新，当月结果不是最终发票；关账后仍可能出现退款、调账。重新同步同一账期会更新金额，不能累加两次。

本期支持 Azure OpenAI、腾讯云国内站混元、Anthropic、阿里云国内站百炼 / 通义千问，以及火山引擎方舟。只配置 API Key 类型的上游账号；绑定的是供应商账单中的资源或工作区标识，不是模型调用密钥本身。多个上游账号共用一个资源时，资源账单无法直接证明其中某把 Key 的真实费用；下游虚拟 Key 的费用只能依据本地使用记录分摊，必须保留“分摊”标识。平台外调用也包含在官方资源账单中，可能造成差异。

## Azure OpenAI（`azure`）

需要 Microsoft Entra 应用 / 服务主体的 `tenant_id`、`client_id`、`client_secret`，以及 `subscription_id` 和完整 `resource_id`。仅 `client_secret` 是秘密字段；应用 ID、租户 ID、订阅 ID 和资源 ID 是配置标识。不能使用 Azure OpenAI 推理接口的 `api-key` 查询云账单。

给服务主体授予目标订阅的 **Cost Management Reader** 权限，且订阅本身需允许查看费用（例如某些 EA 订阅还需启用 AO view charges）。使用只读费用角色，不要为了查询费用授予 Owner 权限。接入固定使用 Azure 公有云端点，不支持世纪互联 Azure 中国区或其他主权云。

资源 ID 示例：

```text
/subscriptions/<subscription-id>/resourceGroups/<group>/providers/Microsoft.CognitiveServices/accounts/<account>
```

一个连接只读取这个显式指定的资源，不会把整个订阅的其他云产品费用混入。资源 ID 匹配不区分大小写。

费用来自 Cost Management Query API `Usage` / `PreTaxCost`，按 UTC 日期和资源分组，保留供应商币种；这是官方税前实际使用费用，不等于含税发票应付总额，也不保证对应最终现金付款。Azure 同一个资源的 key1 / key2 不能通过该接口分别结算。

参考：[费用查询 API](https://learn.microsoft.com/en-us/rest/api/cost-management/query/usage?view=rest-cost-management-2025-03-01)、[授权与角色](https://learn.microsoft.com/en-us/azure/cost-management-billing/costs/assign-access-acm-data)、[服务主体令牌](https://learn.microsoft.com/en-us/entra/identity-platform/v2-oauth2-client-creds-grant-flow)。

## 腾讯混元 / 腾讯云国内站（`tencent`）

需要专用 CAM 子用户的 `secret_id` / `secret_key`，两者均加密保存。应仅允许查询账单所需操作（`DescribeBillDetail`；查找产品编码时还需 `DescribeBillSummaryByProduct`），不要提供腾讯云主账号密码。混元推理接口的 Bearer API Key 不能替代 CAM 签名凭据。

`business_code` 必填：在费用中心核对混元产品，或调用 `DescribeBillSummaryByProduct`，复制该产品返回的 `BusinessCode`。产品编码不是模型名称或 API Key；不要填猜测值。`region` 留空或填 `domestic`，表示**国内计费站点**而非 `ap-guangzhou` 等云地域。当前不支持国际站，避免在接口缺少币种字段时推测币种。

费用来自 `DescribeBillDetail`，只接受显式指定的产品，累计每条账单 `ComponentSet.RealCost`。这是优惠后费用，包含通过现金、赠送金、代金券等不同方式支付的费用，**不是仅现金账户扣款**；退款和冲正负数保持原符号。国内站以人民币元（CNY）显示。

该接口按北京时间（Asia/Shanghai）完整自然月返回账单，即使传入日级查询参数也会返回整月数据。因此本期按**账单归属月**对账，不把月账单伪装为日级或请求级费用。资源绑定请使用返回的 `ResourceId`，保持原大小写；它不一定是混元调用 Key。供应商没有给出资源 ID 的费用保留为未匹配，不猜测关联。接口只提供近 18 个月数据；是否可查询、何时出账以腾讯云规则为准。

参考：[账单明细](https://cloud.tencent.com/document/api/555/30756)、[产品编码与汇总](https://cloud.tencent.com/document/api/555/35761)、[TC3 签名](https://cloud.tencent.com/document/api/213/30654)。

## Anthropic（`anthropic`）

`admin_api_key` 需要能够访问组织 Cost API 的凭据，普通工作区模型 API Key 不适用。**常规组织 Admin Key 可能同时具有其他管理权限**；本模块只发起费用报告 GET 请求，但不能把凭据本身变成只读。请依供应商当前授权能力创建最低必要权限的专用凭据，并限制接入账户的管理员范围。

`workspace_id` 可选，留空读取组织中各工作区费用；填写则只保存指定工作区费用。资源绑定使用返回的工作区 ID。供应商返回 `workspace_id: null` 的费用在这里使用 `__organization__` 表示“未指定工作区”，不是重复加入一笔组织总费用。

费用来自 `/v1/organizations/cost_report`，按 UTC 天、工作区和费用说明分组。官方 `amount` 是**美分的十进制字符串**，适配器精确除以 100 转为美元，不丢弃不足一美分的金额。只读取已经结束的 UTC 日，今天的数据要在后续同步中补齐。工作区费用可能含 token、搜索、代码执行等类别，不把所有费用都解释成 token 单价。

此接口成本粒度是工作区，不是 API Key；不支持个人账号的组织管理 API，也不是 Claude 订阅或 Claude Enterprise 的费用接口。

为了提高分摊准确性，同步还会尝试读取官方 `/v1/organizations/usage_report/messages`。只有账单与用量的**日期、工作区、模型、上下文档位、服务等级、推理地域，以及 token 类别全部对应**时，才附上该费用类别的官方 token 分母。输入、输出、缓存读取、5 分钟缓存写入和 1 小时缓存写入分别处理，不能把总 token 比例直接套到不同单价的费用上。用量接口权限不足、超时、缺字段、尚未出数，或多个不同费用说明无法区分时，仍然保留成功读取的费用数据，并明确标记用量不可用或暂不支持；不会用成本反推 token 数。

当前官方用量证据只接入 Anthropic，且以工作区为账单范围。尽管 Anthropic 用量 API 也支持按上游 API Key 分组，本期不把工作区成本冒充 Key 级官方账单。Azure 当前费用查询没有 token 数；Azure Monitor 的输入/输出 token 指标不是同一份账单计费计量。腾讯原始账单保留 `UsedAmount`、`UsedAmountUnit`、`RealTotalMeasure` 等字段，但前者可能已扣除资源包，不在缺少确定单位/模型映射时自动解释为 token。

参考：[官方用量 API](https://platform.claude.com/docs/en/api/beta/organization/usage_report/retrieve_messages)、[Azure token 监控指标](https://learn.microsoft.com/en-us/azure/azure-monitor/reference/supported-metrics/microsoft-cognitiveservices-accounts-metrics)。

参考：[Usage and Cost API](https://platform.claude.com/docs/en/build-with-claude/usage-cost-api)、[Cost Report API](https://platform.claude.com/docs/en/api/admin/cost_report/retrieve)。

## 阿里云百炼 / 通义千问（`aliyun`）

本接入使用阿里云国内站费用中心的 `DescribeInstanceBill` 历史账单接口，不使用百炼推理 API Key，也不接收主账号登录密码。创建专用 RAM 用户的 AccessKey，`access_key_id` / `access_key_secret` 均加密保存。权限只授予所需读取动作 `bssapi:DescribeInstanceBill`；该操作支持使用 `bssapi:ProductCode` 等条件约束，按账号授权规则限制费用访问范围，不为查询账单授予管理员权限。

`account_id` 必填，表示**实际使用资源的阿里云主账号数字 ID**，不是 RAM 用户 ID。请求将其作为 `BillOwnerId`，每条返回账单必须由 `OwnerID` 证明归属于这个账号。多账号代付场景中的 `BillAccountID` / 返回查询账号可能不同，原样保留，不拿付款或账单账号冒充资源所属账号。`product_code` 必填，请从费用中心账单或官方 API 调试中复制百炼对应的 `ProductCode`，不要填写模型名称、API Key 或猜测值。更换资源所属账号或产品应新建连接；仅 AccessKey 轮换不会改变账单范围。

请求使用北京时间完整自然月、`Granularity=MONTHLY`、`IsBillingItem=true`、`IsHideZeroCharge=false`，以 `NextToken` 读完全部计费项账单。绑定资源使用返回行的 `InstanceID`（区分大小写）；缺失实例标识的费用保持未匹配。接口返回的是月度计费项汇总，并非每次模型请求，也不能区分同一实例下多把 API Key。没有官方不可变明细 ID 时，使用账号、账期、产品、实例、计费项、账单类型等稳定维度去重；遇到无法区分的重复桶整次同步报错，不静默丢单。

金额读取 `PretaxAmount`：官方该接口的中文定义为**应付金额**；字段名不能被简单理解为所有云厂商一致的税前成本口径。币种必须来自返回的 `Currency`，不默认人民币。`AfterDiscountAmount`（优惠后金额，包含券抵扣）、现金支付、资源包抵扣、原始 `Usage` / `UsageUnit` 等继续保留在原始账单中，不作为应付金额的静默替代，也不自动当作 token 数。

官方规则：仅支持近 18 个月账期；当月未结算数据仅供参考，不能视为最终对账单；次月 3 日 12:00 后提供月度完整账单。账单更新约延迟 24 小时，实例信息约延迟 48 小时，以费用中心实际出账为准。本期固定使用 `business.aliyuncs.com`，暂不接入国际站的不同账单端点。请求采用官方推荐的 ACS3-HMAC-SHA256 签名。

参考：[DescribeInstanceBill 及权限 / 字段 / 出账说明](https://help.aliyun.com/zh/user-center/developer-reference/api-bssopenapi-2017-12-14-describeinstancebill)、[ACS3 请求与签名](https://help.aliyun.com/zh/sdk/product-overview/v3-request-structure-and-signature)、[官方 Go SDK 请求模型](https://github.com/alibabacloud-go/bssopenapi-20171214/blob/master/client/describe_instance_bill_request_model.go)。

## 火山引擎方舟（`volcengine`）

本接入读取费用中心 `ListBillDetail`，不是方舟推理接口。创建具有账单明细读取权限的专用 IAM 凭据，`access_key_id` / `access_key_secret` 均加密保存；按火山引擎 IAM 当前授权页面授予读取 `ListBillDetail` 所需的最小权限，不提供主账号密码，不授予写入或财务管理权限。

`account_id` 是**资源所属火山引擎主账号数字 ID**，不是 IAM 子用户 ID。请求显式传入 `OwnerID` 列表，逐行核对返回的 `OwnerID`；`PayerID` 可能不同。`product_code` 必填，复制费用中心方舟账单中的产品英文标识 `Product`，不填模型名称或猜测产品代码。不同云账号可以存在相同的通用实例字符串，连接按账号和产品限定；更换账号或产品应新建连接。

按北京时间完整自然月查询，`GroupPeriod=2`（明细）、`GroupTerm=0`（计费项）、`IgnoreZero=0`，保留官方 `BillDetailId`，分页检查 `Total` / `Offset` / `Limit`。供应商返回警告、总数在分页中变化、明细 ID 缺失或重复时，不提交不完整快照。账期内的原始消费时间保留在原始账单中，当前按账单归属月展示，不伪装成本地请求时间。

资源绑定使用 `InstanceNo`，即云产品的**计费实例 ID**；不同于原始行也可能包含的 `ResourceID`，更不保证等于方舟 API Key。费用采用 `PayableAmount`（应付金额）及同一计价口径的 `Currency`，不将其与 `SettlePayableAmount` / `CurrencySettlement`（结算金额 / 结算币种）混用，也不把现金支付当作完整费用。原始 `Count`、`Unit`、资源包抵扣、计费项等保留，但在模型及单位尚不能可靠对应时不生成自动 token 分母。

固定使用官方账单端点 `billing.volcengineapi.com`、签名服务 `billing`、签名地域 `cn-north-1` 和 API 版本 `2022-01-01`。签名地域是费用 API 的固定参数，不是方舟部署地域。当前账期持续出账，最终结果及历史可查范围以火山费用中心为准；同步只是读取当时官方已返回的明细，不等于关账或最终发票。

参考：[ListBillDetail 字段与分页](https://www.volcengine.com/docs/BillingCenter/ListBillDetail-Pagequerybilldetails?lang=zh)、[账单 API 调用说明](https://www.volcengine.com/docs/BillingCenter/APIcallinstructions?lang=zh)、[官方签名说明](https://www.volcengine.com/docs/6369/67269)、[官方 Go SDK 签名实现](https://github.com/volcengine/volc-sdk-golang/blob/main/base/sign.go)。

## 仅有余额能力的供应商

DeepSeek 官方公开的 `GET /user/balance` 返回当前余额、赠送余额及充值余额，不提供历史账单明细。**不能通过余额差反推 API Key 或虚拟 Key 的历史费用**，因为充值、赠送、其他调用和调整都会影响余额。因此本期不将 DeepSeek 列为可用的历史对账连接，也不拿本地估算或余额快照冒充官方账单。可继续使用现有供应商余额展示；待有可核验的官方历史费用接口后再添加适配器。

参考：[DeepSeek 用户余额接口](https://api-docs.deepseek.com/api/get-user-balance)。其他供应商也遵循相同原则：有可核验的官方历史账单接口才启用对账，而不是把热门供应商名称加入空壳选项。

## 同步和安全边界

- 接入只允许预设官方 HTTPS 端点，拒绝重定向及 Azure 跨主机/跨订阅分页链接，不能填写自定义账单 URL。
- 配置字段使用白名单；秘密不能放在普通配置字段。供应商错误正文、令牌和签名不会回传到同步错误中。
- 适配器最多读取 100 页 / 30,000 条费用记录，每次响应上限 8 MiB、每条原始费用记录上限 64 KiB。超出大小限制时报错，不悄悄截断原始账单。Azure 原始记录保留列定义和数据行；腾讯保留原始账单行；Anthropic 保留费用结果及日桶时间，均不包含授权请求头或令牌。
- 适配器单次请求超时 40 秒、总预算上限 4 分钟；服务层施加更严格的**每个月同步 150 秒**及**每月 10,000 条账单**限制，实际以最先触发的限制为准。自动回溯可配置最近 1–6 个月，每个连接最多 15 分钟总预算；手动同步只针对单月。可选 Anthropic 用量读取另设最多 30 秒预算（同样受外层剩余时间限制）。任何费用页面失败时，不提交不完整的费用账单。
- 限流或暂时故障进行有界重试；较长的 Retry-After 留给后续同步，不能提前重试。腾讯云、阿里云与火山引擎 HTTP 200 中的已知限流错误也会识别。
- 保留原币种、负数和小数精度，不自动跨币种相加或换汇；不修改原有下游余额扣费、订阅额度扣费及使用记录原始费用。
- 本地自动化测试覆盖真实接口格式的模拟响应；上线仍需使用供应商凭据验证权限、产品编码、出账状态与费用中心结果。
