## ADDED Requirements

### Requirement: 回包分辨率不足时同步 upscale 交付

当分组开启 `image_upscale_on_rsp` 且开启 `image_decode_size_on_rsp`（仅 `platform=openai`）时，系统 SHALL 在把 OpenAI 出图响应写给客户端之前，对每张 `b64_json` 图：解码其真实分辨率归一为档位，并与请求归一出的目标档位比较；当目标档位 ∈ {2K, 4K} 且真实档位低于目标档位时，SHALL 调用 fal `fal-ai/seedvr/upscale/image` 将该图放大到目标档位，并以放大后的图替换返回给客户端的内容。客户端与 COS 转存 SHALL 得到同一份（放大后的）字节。

放大倍数 SHALL 由档位推导（1K→2K 用 2 倍，1K→4K 用 4 倍，2K→4K 用 2 倍），采用 SeedVR `upscale_mode=factor`；系统 SHALL NOT 要求精确像素，达到目标档位区间即可。URL 模式回包 SHALL NOT 放大。

#### Scenario: 真实档位低于目标档位则放大

- **WHEN** 分组 `image_upscale_on_rsp=true` 且 `image_decode_size_on_rsp=true`，请求归一目标档位为 4K，某张 b64 图解码真实档位为 1K
- **THEN** 系统 SHALL 以 `upscale_factor=4` 调用 SeedVR 放大该图
- **AND** 返回给客户端的该图 SHALL 为放大后的图
- **AND** COS 转存的该图 SHALL 为同一份放大后的字节

#### Scenario: 已达目标档位不放大

- **WHEN** 某张图解码真实档位已等于或高于目标档位
- **THEN** 系统 SHALL NOT 放大该图，按原图返回

#### Scenario: 目标档位低于 2K 不放大

- **WHEN** 请求归一目标档位为 1K
- **THEN** 系统 SHALL NOT 触发 upscale

#### Scenario: 仅 b64_json 模式

- **WHEN** 回包为 URL 模式（非 b64_json）
- **THEN** 系统 SHALL NOT 放大，按原样返回

#### Scenario: 流式同样放大

- **WHEN** 流式出图请求命中放大条件
- **THEN** 系统 SHALL 缓冲整张图后放大，再将放大后的图写给客户端（不再保证渐进预览）

### Requirement: upscale 失败兜底与计费自洽

upscale 调用失败、超时或结果下载失败时，系统 SHALL 回退使用原图返回客户端与转存 COS，且 SHALL 按原图真实档位计费。upscale 成功时 SHALL 按目标档位计费。计费档位 SHALL 由最终交付给客户端的图字节经现有回包分辨率解析路径得出（成功即为目标档位、失败即为原图档位），不在计费层做额外特判。

upscale 的上游调用成本 SHALL 由平台吸收，SHALL NOT 计入用户余额或账单。

#### Scenario: 放大成功按目标档位计费

- **WHEN** 某张图由 1K 成功放大到 4K 并返回
- **THEN** 该图 SHALL 按 4K 档位计费

#### Scenario: 放大失败按原图计费且返回原图

- **WHEN** SeedVR 调用失败或超时
- **THEN** 系统 SHALL 返回原图（不报错）
- **AND** 该图 SHALL 按原图真实档位计费

#### Scenario: upscale 成本不计入用户

- **WHEN** 任意一次 upscale 调用发生
- **THEN** 系统 SHALL NOT 因该次放大向用户余额或账单追加费用

### Requirement: upscale 开关与依赖校验

系统 SHALL 提供分组级开关 `image_upscale_on_rsp`，仅在 `platform=openai` 的分组生效。开启该开关 SHALL 要求同分组的 `image_decode_size_on_rsp` 也为开启（否则无法解析真实分辨率）；管理端在保存时 SHALL 校验此依赖与平台限制。默认关闭，关闭时行为等价现状。

#### Scenario: 开启 upscale 必须先开 decode

- **WHEN** 管理员对某分组将 `image_upscale_on_rsp` 置为 true，但 `image_decode_size_on_rsp` 为 false
- **THEN** 系统 SHALL 拒绝保存并提示需先开启回包分辨率解析

#### Scenario: 仅 openai 平台可开

- **WHEN** 管理员对非 `platform=openai` 分组开启 `image_upscale_on_rsp`
- **THEN** 系统 SHALL 拒绝保存

#### Scenario: 默认关闭等价现状

- **WHEN** 分组 `image_upscale_on_rsp=false`
- **THEN** 出图响应 SHALL 不经过任何 upscale，行为与现状一致

### Requirement: fal upscale 系统配置

系统 SHALL 以系统配置（settings）承载 fal upscale 的接入参数：模型 endpoint（默认 `fal-ai/seedvr/upscale/image`）、token、超时秒数（默认 300）。token 的状态查询接口 SHALL NOT 回显明文（仅掩码或存在性）。当 endpoint 或 token 未配置时，即使分组开关开启，系统 SHALL 跳过放大并按原图兜底，不得报错。

#### Scenario: 未配置时安全降级

- **WHEN** 分组 `image_upscale_on_rsp=true` 但系统未配置 `fal_upscale_token`
- **THEN** 系统 SHALL 不放大、按原图返回与计费，且不返回错误

#### Scenario: 超时可配

- **WHEN** 管理员设置 `fal_upscale_timeout_seconds`
- **THEN** 单次放大（含队列轮询与结果下载）的最长耗时 SHALL 受该值约束，超时按兜底处理

#### Scenario: token 不回显

- **WHEN** 管理员查询 fal upscale 配置状态
- **THEN** 响应 SHALL NOT 包含 token 明文
