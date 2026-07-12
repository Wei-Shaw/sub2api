# Design — openai-image-upscale

## 背景与约束

详见 `proposal.md`。本设计聚焦三个硬点：**同步 upscale 的插入位置（含流式）**、**判定与倍数逻辑**、**失败兜底与计费自洽**。

已确认的决定：

| 维度 | 决定 |
|------|------|
| 客户端是否拿放大图 | 是，可阻塞 → upscale 移到响应写出之前的同步主链路 |
| 流式 | 同样放大：缓冲整张图 → 放大 → 再吐（退化为非流式） |
| 触发条件 | 仅 b64_json；目标档位 ∈ {2K,4K}；真实档位 < 目标档位 |
| 目标 | 按归一档位（1K/2K/4K），不追求精确像素 |
| 倍数 | 1K→2K=2，1K→4K=4，2K→4K=2；SeedVR `upscale_mode=factor` |
| 模型 | `fal-ai/seedvr/upscale/image`，fal 队列 Submit→Status→Result |
| 超时 | 可配，默认 300s；超时→原图兜底 |
| 失败兜底 | 调用/超时/下载失败 → 原图 + 按原图档位计费 |
| 计费 | 成功按目标档位、失败按原图档位（随交付字节由现有解码得出） |
| 成本 | 平台吸收，不计入用户 |
| 配置 | 分组开关 + 3 个系统配置 |

## 插入位置：响应写出之前（关键）

现状：非流式 handler `body=读全量` → `c.Data(body)` 立刻写客户端 → 之后才解析。要让客户端拿到放大图，upscale 必须在 `c.Data` 之前，并**改写 body 内的 b64**。

```
出图响应处理（4 处：images/generations 流&非流 + Responses 流&非流）
  缓冲整张图字节
  └─ upscaleOpenAIImages(result/body, group)   ← 新增同步步骤
       对每张图 i (仅 b64_json):
         realTier = ClassifyTier(decode(b64[i]))        # 复用现有解码
         targetTier = NormalizeImageBillingTier(请求 size)
         IF group.ImageUpscaleOnRsp
            && group.ImageDecodeSizeOnRsp
            && targetTier ∈ {2K,4K}
            && tierOrd(realTier) < tierOrd(targetTier)
            && fal upscale 已配置:
              factor = factorFor(realTier, targetTier)   # 2 或 4
              outURL = seedvr.Upscale(dataURI(b64[i]), factor, timeout)  # 队列轮询
              outBytes = download(outURL)
              成功 → b64[i] = base64(outBytes); 标记 upscaled
              失败/超时/下载失败 → 保留 b64[i]（兜底）
  写改写后的 body → 客户端
  scheduleCosUpload(同字节)            # COS 与客户端一致
  RecordUsage → 解码交付的 b64 → 自然按 target/原图 计费
```

### 流式

图片流式是渐进预览。开启 upscale 时，流式路径 SHALL 缓冲全部图像事件 → 放大 → 一次性吐出（丢失渐进预览，文档需告知）。未开启时维持原渐进透传。

### 4 处落点

- `images/generations` 非流式：`handleOpenAIImagesNonStreamingResponse` —— 在 `c.Data(body)` 前改写 body。
- `images/generations` 流式：`handleOpenAIImagesStreamingResponse` —— 缓冲后改写再吐。
- Responses API 非流式 / 流式：`openai_images_responses.go` 对应两处。

## 判定与倍数

复用 `image_billing_size.go` 的档位体系（1K/2K/4K）：

```
tierOrd: 1K=1, 2K=2, 4K=3
factorFor(real, target):
  1K→2K = 2
  1K→4K = 4
  2K→4K = 2
  其余（real≥target 或 target=1K）→ 不放大
```

- `targetTier` 取自请求归一档位（与计费同一套 `NormalizeImageBillingTier`），保证「按归一档位判定」。
- `realTier` 取自对交付 b64 的解码（与 `DecodeOpenAIImageOutputSizes` 同一解码），保证「走解析回包分辨率路径」。
- 不做精确像素：SeedVR factor 模式放大 2x/4x 后只要落进目标档位的像素区间即可；不二次 resize。

## SeedVR upscale 客户端

新增独立 req/resp 类型（现有 `fal.Request`/`Response` 是文生图/编辑形状，不复用）：

```
请求 (POST endpoint, 队列模式)：
  {
    "image_url": "data:image/png;base64,<原图>",   // data URI 传字节（tasks 首步实测确认）
    "upscale_mode": "factor",
    "upscale_factor": <2|4>,
    "output_format": "png"                          // 与原图一致
  }
轮询：Submit → Status(直到 completed) → Result
出参：{ "image": { "url": "...", "content_type": "image/png" } }
  → download(url) 得字节
```

- **单图入单图出**：N 张图调用 N 次。可在超时预算内并发（每图独立 goroutine），任一失败仅该图兜底。
- **传输**：走 fal 队列（Submit/Status/Result，已有 client 方法），不用 Sync——5 分钟预算下队列更稳，避免 HTTP 同步超时。
- **超时**：单次放大（含轮询+下载）受 `fal_upscale_timeout_seconds` 约束；超时取消并兜底原图。
- **鉴权/endpoint**：用系统配置 `fal_upscale_token` / `fal_upscale_endpoint` 构造独立 fal client（与出图账号的 fal 凭据无关）。

## 失败兜底与计费自洽

```
upscale 成功 → 交付/COS = 放大字节 → RecordUsage 解码 → target 档位计费
upscale 失败 → 交付/COS = 原图字节 → RecordUsage 解码 → 原图档位计费
```

因为最终都让「交付的 b64」成为计费解码的输入，#5（成功按目标）/#3（失败按原图）**自动满足，无需在计费层特判**。upscale 成本平台吸收：不向 `balance_ledger`/用户余额追加任何费用。

未配置 endpoint/token 时：开关即便开着也直接走兜底（不报错、不放大），保证可灰度。

## 配置面

```
分组 (platform=openai)：
  image_upscale_on_rsp  bool  default false
  校验：置 true 时必须 image_decode_size_on_rsp=true 且 platform=openai

系统配置 (settings)：
  fal_upscale_endpoint         默认 "fal-ai/seedvr/upscale/image"
  fal_upscale_token            （admin 写入，状态接口仅回显掩码）
  fal_upscale_timeout_seconds  默认 300
```

## 风险

- **延迟叠加**：客户端出图请求串行等「OpenAI 生成 + SeedVR 放大(+下载)」，最坏到超时上限。出图是交互接口，P99 显著拉长。缓解：超时可配；可评估对流式给更短预算。
- **流式退化**：开启后流式丢失渐进预览（A 的必然代价）。
- **多图放大**：N 张 = N 次 fal 调用，成本与延迟随张数线性增长。
- **data URI 体积**：大图 base64 入参可能触发 fal 请求体上限；tasks 首步需实测，必要时改为先上传图再传 URL。

## 待实现期确认（写入 tasks 首步）

- SeedVR `image_url` 是否接受 base64 data URI；体积上限。
- 输出是否始终为托管 URL（是否有 sync_mode 可内联省下载）。
- `upscale_factor` 取值范围与与档位的实际像素对应。
