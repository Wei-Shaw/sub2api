# media-cos-archival Specification

## Purpose
TBD - created by archiving change add-fal-async-image-platform. Update Purpose after archive.
## Requirements
### Requirement: 全局 COS 配置

系统 SHALL 提供全局一份腾讯云 COS 配置（后台可配置），至少包含开关、Endpoint、Region、SecretId/SecretKey、Bucket，以及可选的路径前缀/自定义域名。COS 通过 S3 兼容协议接入，复用既有对象存储抽象。

#### Scenario: 后台配置 COS
- **WHEN** 管理员在后台填写并保存 COS 配置
- **THEN** 系统 SHALL 持久化该配置并在开启后用于转存

#### Scenario: COS 关闭
- **WHEN** COS 开关为关闭
- **THEN** 系统 SHALL 不进行转存，直接使用 fal 原始 url

### Requirement: 生成图片转存 COS

当 COS 开启时，系统 SHALL 在任务出图成功后下载 fal 临时图片并上传至 COS，得到长期可用的 `cos_url`。generations 与 edits、OpenAI 伪同步门面与 fal 原生门面 SHALL 全部执行转存。

#### Scenario: 转存成功
- **WHEN** 任务出图成功且 COS 开启
- **THEN** 系统 SHALL 将图片转存至 COS 并在任务与 usage_log 中记录 `cos_url`

#### Scenario: 两类门面均转存
- **WHEN** 出图来自 OpenAI 伪同步门面或 fal 原生门面
- **THEN** 两者 SHALL 同样执行转存

### Requirement: 转存重试与回退

转存失败时系统 SHALL 最多重试 3 次；仍失败则 `cos_url` 留空、回退使用 fal 原始 url，任务仍算成功、不退费。

#### Scenario: 重试后成功
- **WHEN** 首次转存失败但重试（不超过 3 次）后成功
- **THEN** 系统 SHALL 记录 `cos_url` 并视为成功

#### Scenario: 重试耗尽回退
- **WHEN** 转存连续失败达 3 次
- **THEN** 系统 SHALL 将 `cos_url` 留空、回退用 fal 原始 url，任务仍为成功且不退费

### Requirement: 返回地址优先级

对客户端返回图片地址时，系统 SHALL 在 COS 开启且转存成功时优先返回 `cos_url`，否则返回 fal 原始 url。

#### Scenario: 优先返回 COS url
- **WHEN** 转存成功且存在 `cos_url`
- **THEN** 响应 SHALL 使用 `cos_url`

#### Scenario: 回退返回 fal url
- **WHEN** 未开启 COS 或转存失败
- **THEN** 响应 SHALL 使用 fal 原始 url
