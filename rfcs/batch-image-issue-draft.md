# RFC Issue Draft: Batch Image

## Title

```text
RFC: add asynchronous Gemini image batch generation with Gemini API key and Vertex providers
```

## Body

```markdown
## Summary

I would like to propose an MVP for asynchronous Gemini image batch generation in Sub2API.

I want to add a new batch image gateway surface under `/v1/images/batches`, backed by Redis workers and PostgreSQL state, with two initial upstream providers:

- Gemini Developer API / AI Studio API key accounts
- Vertex AI Gemini service-account accounts

The goal is to expose one stable Sub2API batch interface while keeping provider-specific details such as Gemini file names, Vertex job names, GCS paths, and service-account credentials internal.

## Why

Sub2API already has most of the primitives needed for this:

- Gemini accounts already support `platform=gemini,type=api_key`.
- Vertex service-account helpers already exist.
- Redis is already part of the runtime.
- PostgreSQL/Ent is already the source of truth.
- Existing usage billing already has idempotent billing via `usage_billing_dedup`.

Gemini API and Vertex both support async batch generation, but their auth/storage/result mechanics are different. I want to keep one public API and put those differences behind a small provider abstraction.

The main reason I want to build this is that the official Gemini Batch API is designed for asynchronous, non-urgent large-volume requests and is documented as running at 50% of the standard cost. For image generation, that makes batch mode useful both for higher-throughput workloads and for lowering user-facing cost compared with realtime generation.

Official references:

- Gemini Batch API: https://ai.google.dev/gemini-api/docs/batch-api
- Gemini image generation batch section: https://ai.google.dev/gemini-api/docs/image-generation#batch-api
- Vertex Gemini batch prediction: https://cloud.google.com/vertex-ai/generative-ai/docs/multimodal/batch-prediction-gemini

## MVP

The MVP I want to build includes:

- Batch submit
- Async worker execution
- Status query
- Result indexing
- Single image streaming download
- ZIP streaming download
- Basic hold -> settlement billing
- Idempotency and crash recovery
- First providers: `gemini_api` and `vertex`

## API

Gateway routes, API-key authenticated:

```text
POST   /v1/images/batches
GET    /v1/images/batches/{idREDACTED
GET    /v1/images/batches/{idREDACTED/items
GET    /v1/images/batches/{idREDACTED/items/{custom_idREDACTED/content
GET    /v1/images/batches/{idREDACTED/download
POST   /v1/images/batches/{idREDACTED/cancel
DELETE /v1/images/batches/{idREDACTED/outputs
```

I want to use `/v1/images/batches` because this is a gateway/API-key feature rather than a dashboard/JWT-only feature under `/api/v1`.

## Implementation Shape

High-level shape:

- Add `batch_image_jobs`, `batch_image_items`, and optional `batch_image_events`.
- Store `provider` as `gemini_api` or `vertex`.
- Store selected `account_id` on the job so worker retries are deterministic.
- Use Redis `LPUSH/BRPOP`, an active set, and per-job locks.
- Keep PostgreSQL as the source of truth.
- Stream downloads through Sub2API without writing image bytes to local disk.
- Keep Gemini file names, Vertex job names, GCS URIs, bucket names, and service-account details internal.

Provider abstraction:

```go
type BatchImageProvider interface {
    Name() string
    SupportsAccount(account *Account) bool
    Submit(ctx context.Context, job *BatchImageJob, account *Account, input BatchImageInput) (*BatchProviderJob, error)
    Get(ctx context.Context, job *BatchImageJob, account *Account) (*BatchProviderStatus, error)
    Cancel(ctx context.Context, job *BatchImageJob, account *Account) error
    OpenResult(ctx context.Context, job *BatchImageJob, item *BatchImageItem, account *Account) (io.ReadCloser, string, error)
    Cleanup(ctx context.Context, job *BatchImageJob, account *Account, target CleanupTarget) error
REDACTED
```

Billing:

- Estimate cost at submit time and place a hold.
- Charge only successful generated images.
- Failed items are not charged in the MVP.
- Settlement is idempotent.
- I want to reuse the existing `UsageBillingRepository.Apply` / `usage_billing_dedup` path with a synthetic request id like `batch_image_settlement:{job_idREDACTED`.

## PR Split

1. Schema, Ent models, repository CRUD, status machine
2. Redis queue, idempotency, active job recovery
3. Provider core plus both `gemini_api` and `vertex` providers
4. Settlement / billing integration
5. Download APIs
6. Cleanup worker

## Questions for maintainers

1. Is `/v1/images/batches` the right public route for this feature?
2. Is storing hold fields on the batch job acceptable for MVP, with final settlement reusing existing usage billing?
3. Would you prefer the first implementation to be API-only, or include dashboard pages from the beginning?
4. Do you prefer a different naming convention for provider names, table names, or statuses?

---

## 中文版本

我想为 Sub2API 增加一个异步 Gemini 批量生图 MVP。

我想新增 `/v1/images/batches` 这一组网关 API，由 Redis worker 和 PostgreSQL 状态表驱动，首版支持两个上游 provider：

- Gemini Developer API / AI Studio 的 API key 账号
- Vertex AI Gemini 的 service account 账号

目标是让用户只调用一套 Sub2API batch 接口，同时把 Gemini file name、Vertex job name、GCS 路径、bucket、service account 等内部细节留在服务端。

### 为什么这样做

Sub2API 现有架构已经比较适合这个功能：

- 现有账号模型已经支持 `platform=gemini,type=api_key`。
- 代码里已有 Vertex service account token helper。
- Redis 已经是运行时依赖。
- PostgreSQL/Ent 已经是主要状态源。
- 现有账务已经有 `usage_billing_dedup` 这种幂等扣费机制。

Gemini API 和 Vertex 都有异步 batch 能力，但认证、存储、结果读取方式不同。所以我想在内部加一个小的 provider 抽象，对外保持一套稳定 API。

我想做这个功能的主要原因是：Gemini 官方 Batch API 本身就是为异步、非实时的大批量请求设计的，而且官方文档写明成本是标准实时请求的 50%。对于批量生图场景，这既能提升大批量任务的可用性，也能让用户成本低于实时生成。

### MVP

我想先实现：

- 批量提交
- 异步 worker 执行
- 状态查询
- 结果索引
- 单图流式下载
- ZIP 流式下载
- 基础 hold -> settlement 计费
- 幂等与 crash recovery
- 首批 provider：`gemini_api` 和 `vertex`

### API

这些路由走 API key 鉴权：

```text
POST   /v1/images/batches
GET    /v1/images/batches/{idREDACTED
GET    /v1/images/batches/{idREDACTED/items
GET    /v1/images/batches/{idREDACTED/items/{custom_idREDACTED/content
GET    /v1/images/batches/{idREDACTED/download
POST   /v1/images/batches/{idREDACTED/cancel
DELETE /v1/images/batches/{idREDACTED/outputs
```

我想放在 `/v1/images/batches`，因为这是网关/API key 能力，不是只给后台面板用的 `/api/v1` JWT API。

### 实现方式

- 新增 `batch_image_jobs`、`batch_image_items`，以及可选的 `batch_image_events`。
- job 记录 `provider=gemini_api|vertex`。
- job 记录选中的 `account_id`，保证 worker 重试时不会换账号。
- Redis 使用 `LPUSH/BRPOP`、active set 和 per-job lock。
- PostgreSQL 作为事实状态源。
- 下载经 Sub2API 流式返回，不把图片字节写入本地磁盘。
- 不向用户暴露 Gemini file name、Vertex job name、GCS URI、bucket、service account 等细节。

计费：

- 提交时估算费用并冻结额度。
- 只对成功生成的图片收费。
- MVP 中失败 item 不收费。
- settlement 必须幂等。
- 我想复用现有 `UsageBillingRepository.Apply` / `usage_billing_dedup`，使用类似 `batch_image_settlement:{job_idREDACTED` 的 synthetic request id。

### PR 拆分

1. Schema、Ent models、repository CRUD、状态机
2. Redis queue、幂等、active job recovery
3. Provider core + `gemini_api` 和 `vertex` 两个 provider
4. Settlement / billing integration
5. Download APIs
6. Cleanup worker

### 想请维护者确认的问题

1. `/v1/images/batches` 是否是合适的公开路由？
2. MVP 中把 hold 字段先存在 batch job 表上，并在最终结算时复用现有 usage billing，是否可以接受？
3. 首版做 API-only 是否可以，还是需要一开始就包含 dashboard 页面？
4. provider 名称、表名、状态名是否有维护者偏好的命名规范？
```
