# MiniMax 媒体接入与统一分组计费

协议和国际站目录价核对日期：2026-10-29。本文描述代码支持范围；生产可用性需要独立部署和真实请求验收。

## 配置入口与计费规则

在后台「分组管理 → 创建/编辑分组 → MiniMax 媒体计费」配置，适用于 MiniMax 和 composite 分组。入口、独立倍率、空值和免费语义与现有图片/视频计费一致：

- 售价统一使用 USD；留空采用国际站官方默认价，显式 `0` 表示免费。
- 不启用独立倍率时，使用用户分组覆盖倍率或分组默认倍率；启用后只使用 MiniMax 独立倍率。高峰时段的文本倍率不参与媒体计费。
- 最终扣费 = 各计费项零售价之和 × 有效媒体倍率。界面预览使用分组默认倍率，用户单独覆盖时按实际用户倍率结算。
- 新价格存入 `minimax_media_prices`，只保存显式覆盖。优先级：新分组价格 → 历史 `model_pricing[].minimax_media` 有效零售价 → 官方默认价。清空新覆盖后仍可能回退到历史价卡。
- 历史价卡原样保留，不再作为新配置入口。旧任务继续使用已冻结的价格、币种、汇率和倍率；新任务统一采用上述倍率规则。
- 成本优先保留历史价卡的有效成本与汇率，否则采用国际站 USD 目录成本估算。目录成本不是经过核实的供应商账户实际扣款；国内账户也不自动推定与国际价格相同。

图片和视频还需启用分组的生成权限：纯 MiniMax 分组在同一区域勾选「允许当前分组生成图片和视频」；composite 分组沿用现有图片生成区的「允许当前分组生图」开关。关闭此权限不影响语音和音色接口。

账号编辑中启用「MiniMax 媒体」，并选择官方国内 `https://api.minimax.cn` 或国际 `https://api.minimax.io`。凭据必须实际具备对应媒体权限；文本/coding 资格不代表媒体调用已验证。媒体地址不使用文本 Base URL，不允许任意中转站；沿用账号代理且代理失败不回退直连。启用域名白名单时加入所选官方域名。

## 接口范围

| 能力 | 接口 / 模型 |
| --- | --- |
| 同步语音 HTTP/SSE | `POST /v1/t2a_v2`，speech-2.8、2.6、02 的 hd/turbo |
| 克隆样本上传 | `POST /v1/files/upload`，multipart，purpose 为 voice_clone 或 prompt_audio |
| 音色克隆 / 试听 | `POST /v1/voice_clone` |
| 音色设计 / 试听 | `POST /v1/voice_design` |
| 音色查询 | `POST /v1/get_voice`，system/all/voice_cloning/voice_generation |
| 图片文生/图生 | `POST /v1/image_generation`，image-01 |
| 视频创建 | `POST /v2/video_generation`，MiniMax-H3、Highspeed、Lite、Max |
| 视频查询 | `GET /v2/query/video_generation[/{task_id}]` |
| 本地任务回执 | `GET /v1/media/tasks[/{task_id}]` |

尚未接入 WebSocket、异步长文本语音、ASR、Context-IR、视频重生成、回调、取消接口、旧 Hailuo 和音乐。不存在取消即退款行为。

语音文本 1–9999 Unicode 字符；混音中的每个音色都需校验。H3/Highspeed/Lite 为 4–15 秒整数，Max 为 5–15 秒；H3 支持 768P/2K，Highspeed 仅 768P，Lite/Max 支持 480P/768P。图片每次 1–9 张。

## 默认国际目录价（USD）

| 模型/项目 | 价格 |
| --- | --- |
| Speech HD / Turbo | 每百万计费字符 100 / 60 |
| 音色克隆 / 设计首次使用 | 每个音色 1.5 / 3 |
| 设计试听 | 每百万输入字符 60 |
| image-01 | 每张成功图片 0.0035 |
| H3 768P / 2K 输出、输入视频 | 每秒 0.08 / 0.13 |
| Highspeed 768P 输出、输入视频 | 每秒 0.06 |
| Lite 480P / 768P 输出、输入视频 | 每秒 0.025 / 0.04 |
| Max 480P 输出 / 输入视频 | 每秒 0.05 / 0.0553 |
| Max 768P 输出 / 输入视频 | 每秒 0.08 / 0.143 |
| H3 / Highspeed / Lite 超额参考图 | 每张 0.04 / 0.03 / 0.02，前 5 张免费 |
| Max 超额参考图 | 每张 0.074，前 2 张免费 |

参考音频免费。以实际账户和最新官方价格为准，不自动把人民币金额作为 USD 换算。

## 私有音色与文件归属

上传不收费、不落盘保存样本；仅接收 mp3/m4a/wav，单文件最多 20 MiB。音频时长、声纹权限等由上游校验。上传后的 file_id 绑定用户、API Key 和原上游账号，克隆样本与 prompt_audio 必须来自同一账号。不会把原始音频转发到其他账号重试。

克隆需提供 file_id 和 8–256 位自定义 voice_id（字母开头，字母数字及下划线/短横线，末尾为字母数字）。试听 text 最多 1000 字并指定受支持的语音模型。设计需 prompt 和最多 500 字的 preview_text。成功后将 voice_id 登记到当前用户/Key，并在后续 T2A 中固定使用原账号。

音色列表中的私有音色来自本地所属 Key 的登记记录，不读取或暴露供应商账号内其他用户的私有音色。当前版本不自动导入平台外已创建的音色；例如已有私有 voice_id 不能仅凭知道名称就调用。

克隆和设计时不收取音色创建费；只有首次合成的 `extra_info.usage_voice_count` 才触发该费用。按 `(account_id, voice_id)` 在结算事务内锁定、登记首次使用并去重；并发回执不会重复向用户收取首次使用费。试听字符单独计费。

普通 T2A 和克隆试听只采用上游 `usage_characters`。设计接口目前未文档化返回字符用量，成功返回试听音频且无上游用量字段时，平台按输入 preview_text 的 Unicode 字符数计费，并在回执标记 `request_characters`；这不是已核实的上游字符用量。其目录成本同样仅为估算。若收到实际 extra_info 则校验该回执；异常用量不猜测、不释放预占。

未登记音色、无法归属的首次使用费或不完整计费回执保持待核对，不自动重发、不自动退款。可解析但暂时无法计价的用量会持久化，并标记 `reconciliation_required=true`；这些待核对回执中的零值金额不表示免费。混音的首次使用计数少于预占快照时，结算事务会锁定音色登记记录，只有能够确认剩余待激活音色与计数相符时才结算，否则保留预占供继续核对。未使用的克隆音色可能被上游在 7 天后删除；本地登记不代表上游永久保留。

## 请求示例

以下 Key 为 Sub2API 用户 Key，不是供应商 Key。重复付费请求必须复用原始 Idempotency-Key；未知结果先查询原任务。

```sh
curl https://example.com/v1/files/upload \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -F purpose=voice_clone -F file=@sample.wav

curl https://example.com/v1/voice_clone \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: clone-demo-001' \
  -d '{"file_id":123456789,"voice_id":"MyVoice001"}'

curl https://example.com/v1/t2a_v2 \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: speech-demo-001' \
  -d '{"model":"speech-2.8-turbo","text":"你好，欢迎使用。","voice_setting":{"voice_id":"MyVoice001"}}'

curl https://example.com/v1/image_generation \
  -H "Authorization: Bearer $SUB2API_API_KEY" \
  -H 'Content-Type: application/json' -H 'Idempotency-Key: image-demo-001' \
  -d '{"model":"image-01","prompt":"清晨森林中的小鸟","n":1}'
```

## 资金、用量与排障

标准模式下，提交前在 PostgreSQL 事务内登记任务，并预占钱包余额/订阅和 Key 消费额度。最终回执先持久化，再在同一结算事务内返还预占、扣实际费用、更新额度、写用量和登记音色首次使用。冻结的价格快照不会被后续改价影响。实际费用超过预估时仍记录实际消耗，可能透支余额。

`RUN_MODE=simple` 沿用平台不扣费语义：保留用量及目录成本估算，用户实际费用为零，不预占或扣除钱包、订阅、Key 总额度和账号额度。只有显式开启 `simple_mode_key_rate_limit_enabled` 且 Key 设置了窗口限额时，才预占和结算 Key 的 5 小时、1 天、7 天窗口用量。运行模式对应的计费策略随任务冻结，重启或切换模式不会追溯改变既有任务。

图片以 `metadata.success_count` 计成功数量；视频按 output_seconds、input_seconds、input_image_count 分项计费，total_seconds 和 token 统计不重复计费。H3 后台使用原账号轮询，重启后恢复。刷新结果 URL 不再次收费。

用量明细沿用原表格：模型、端点、类型、计费模式、图片数量/视频信息、费用、耗时和 IP；不显示单独的「媒体账单」展开项。图片写入 image_count，视频沿用 video 字段。内部回执接口继续保留：

- 用户：`GET /api/v1/usage/media/{task_id}`，只含自己的零售回执；`meter_source` 区分上游用量 `upstream_usage` 与设计试听输入字符估算 `request_characters`，`reconciliation_required` 标记尚不能确认费用的回执。
- 管理员：`GET /api/v1/admin/usage/media/{task_id}` 和 `/api/v1/admin/minimax-media/tasks`，包含成本、账号和待核对任务。

语音/图片/音色创建的音频或图像数据不存入任务表，幂等重放返回 409 和原 task_id，不能要求上游重新生成；视频重放返回原本地 task_id。响应头 `X-Sub2API-Task-ID` 可用于查询回执，`X-Sub2API-Billing-State` 表示当前请求的结算状态。视频响应中的 `sub2api_billing` 提供本地任务回执。

相同用户和 Key 使用相同 `Idempotency-Key`、接口类型及原始请求体重放已登记任务时，可以在余额耗尽、Key 过期或额度耗尽后读取原回执，也不依赖上游账号当前是否可调度。重放仍要求用户、Key 和分组可用，并遵守 IP 与分组访问限制；不同请求体或接口复用该键会返回冲突。

网络超时、5xx、缺失或异常用量保留 pending/unknown。无已知任务号的未知创建需管理员核对上游账单；不会换 Key、换账号或自动重发。日志避免密钥、完整提示词、样本音频和签名结果链接。

迁移 251 新增持久化任务和预占回执表，252 新增分组价与倍率，253 新增私有素材登记、扩展任务类型并允许图片使用 `per_image` 按张计费档位；不将按张单价冒充尺寸档位。均保留既有任务和余额。部署前备份数据库，不通过删除任务/冻结金额回滚。

## 官方依据

- https://platform.minimax.io/docs/pricing/overview
- https://platform.minimax.io/docs/api-reference/speech-t2a-http
- https://platform.minimax.io/docs/api-reference/voice-cloning-clone
- https://platform.minimax.io/docs/api-reference/voice-design-design
- https://platform.minimax.io/docs/api-reference/image-generation-t2i
- https://platform.minimax.io/docs/api-reference/video-generation-v2-create
