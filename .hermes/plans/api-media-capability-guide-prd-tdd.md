# PRD + TDD — Hướng dẫn API Video / Image theo capability thực tế

Trạng thái: **đã cập nhật sau đối chiếu code + runtime DB; chờ review trước khi code**
Snapshot code: `origin/main` tại `dee5643df26b576a5133735008d219f47714497c`
Worktree phân tích: `/home/ubuntu/workspaces/sub2api-origin-main`
Ngày đối chiếu runtime: `2026-10-10`
Không ghi credential, token, password, connection string hoặc giá trị secret trong tài liệu.

> Tài liệu này là PRD/TDD, chưa phải source implementation. Endpoint capability được đề xuất ở dưới **chưa tồn tại** trong snapshot hiện tại.

---

## 1. Quyết định nền tảng

Capability hiển thị trong **Use Key** phải được tính theo chuỗi:

```text
API key
  → group của key
  → group gate + model allowlist
  → account thuộc group / scope scheduler
  → account active + schedulable
  → provider/platform resolution
  → model mapping của account
  → route-specific capability/feature gate
  → capability dùng được cho đúng endpoint
```

Không dùng các giá trị sau làm nguồn sự thật độc lập:

- tên group như `Image`, `Video`, `China Model`;
- `platform` của group mà không kiểm tra account;
- keyword trong tên model (`image`, `video`, `seedance`, `veo`, `wan`, ...);
- `allow_image_generation` để suy ra account có model;
- một bảng runtime phụ đang rỗng để kết luận composite không có capability.
- model visibility trong Model Plaza như một per-user block; model policy hiện được quản trị ở group/channel scope.

Keyword model chỉ được dùng để **tìm candidate khi điều tra**. Resolver production phải dùng cùng predicate với scheduler/handler hiện có: model mapping, `IsModelSupported`, account endpoint capability, platform resolution, group allowlist và feature flag.

---

## 2. Vấn đề sản phẩm

`frontend/src/components/keys/UseKeyModal.vue` hiện nhận `apiKey`, `baseUrl`, `platform` và sinh hướng dẫn theo client/platform. Modal chưa có capability model-aware nên không thể nói chính xác key hiện tại dùng được:

- image sync hay image async;
- video generation/status/content;
- video edit/extension;
- Seedance task API;
- batch image;
- model nào đã được account mapping và route thật chấp nhận.

Kết quả hiện tại dễ gây hai lỗi đối nghịch:

1. Hiện hướng dẫn cho route mà key không thể chạy, vì group gate hoặc account/provider không phù hợp.
2. Ẩn capability có thật chỉ vì tên group/platform không phản ánh account mapping.

### Mục tiêu

1. Trong Use Key, chỉ hiện hướng dẫn media khi route-specific capability đã được backend resolve cho **chính API key đó**.
2. Mỗi capability phân biệt được gate, account candidate, route/provider support và feature enablement.
3. Ví dụ `curl` dùng đúng `baseUrl` của hệ thống, không hardcode domain.
4. Model hiển thị phải đến từ mapping/availability thật hoặc endpoint discovery thật; không tạo static list riêng trong frontend.
5. Không lộ account ID, upstream URL, provider credential, model đích sau mapping hoặc lý do nội bộ nhạy cảm.

### Không thuộc phạm vi

- Không đổi route media hiện có.
- Không đổi provider adapter, billing, scheduler hoặc upstream OpenAI/Gemini logic.
- Không bật group flag, thêm account, sửa mapping hay thay đổi production data trong task này.
- Không coi PRD là phê duyệt rollout production.

---

## 3. Bằng chứng implementation hiện tại

### 3.1 Middleware chung

Gateway route dùng chain gồm body/request ID/error logging/endpoint normalization, API-key auth, `groupModelAllowlist`, composite target resolution và group platform guard. Capability resolver phải tôn trọng các guard này; không được dùng capability endpoint để nới model allowlist.

`rootRoute` trong `backend/internal/server/routes/gateway.go` áp dụng cùng chain cho các alias root. Các route dưới `/v1` cũng có chain tương đương.

### 3.2 Image sync

| Method | Path hiện có | Handler thực tế |
|---|---|---|
| POST | `/v1/images/generations` | `imagesHandler` → OpenAI hoặc Grok handler |
| POST | `/images/generations` | root alias → `imagesHandler` |
| POST | `/v1/images/edits` | `imagesHandler` → OpenAI hoặc Grok handler |
| POST | `/images/edits` | root alias → `imagesHandler` |

`imagesHandler` chỉ dispatch trực tiếp cho group platform `openai` hoặc `grok`; platform khác trả 404 feature gate.

OpenAI image handler:

- kiểm tra `GroupAllowsImageGeneration(apiKey.Group)`;
- phân tích request/model để xác định image capability;
- dùng `SelectAccountWithSchedulerForImages(...)`;
- truyền `routingModel` và required image capability vào scheduler;
- scheduler/account phải chấp nhận model mapping và capability phù hợp.

Grok image handler đi qua Grok media forwarder; forwarder chỉ nhận account platform `grok`, rewrite model theo mapping của account và kiểm tra response có image output.

**Kết luận:** `allow_image_generation=true` chỉ là gate cần thiết, không đủ để hiển thị image guide.

### 3.3 Image async

| Method | Path hiện có | Handler |
|---|---|---|
| POST | `/v1/images/generations/async` | `AsyncImage.Submit` |
| POST | `/v1/images/edits/async` | `AsyncImage.Submit` |
| POST | `/images/generations/async` | root alias |
| POST | `/images/edits/async` | root alias |
| GET | `/v1/images/tasks/:task_id` | `AsyncImage.Get` |
| GET | `/images/tasks/:task_id` | root alias |

Các điều kiện đã thấy trong `image_task_handler.go`:

- object storage/task service phải enabled để submit;
- platform request phải là `openai` hoặc `grok`;
- group phải qua `GroupAllowsImageGeneration`;
- `stream:true` bị từ chối;
- submit trả `202 Accepted`, `Location`, `Retry-After: 3`, `Cache-Control: no-store`;
- polling vẫn có thể đọc task đã nhận khi submit feature bị tắt, nếu task store còn pollable;
- public task response không lộ user/key ownership fields.

Resolver phải có field riêng cho `async.submit_enabled` và `async.poll_enabled`; không được suy ra async chỉ từ image sync.

### 3.4 Batch image

| Method | Path hiện có |
|---|---|
| POST | `/v1/images/batches` |
| GET | `/v1/images/batches` |
| GET | `/v1/images/batches/models` |
| GET | `/v1/images/batches/:id` |
| GET | `/v1/images/batches/:id/items` |
| GET | `/v1/images/batches/:id/items/:custom_id/content` |
| GET | `/v1/images/batches/:id/download` |
| POST | `/v1/images/batches/:id/cancel` |
| DELETE | `/v1/images/batches/:id` |
| DELETE | `/v1/images/batches/:id/outputs` |

Snapshot hiện tại không đăng ký root alias cho batch.

`BatchImagePublicService.ensureGroupAllowsBatchImage` yêu cầu **đồng thời**:

1. `group.AllowBatchImageGeneration == true`;
2. `group.Platform == gemini`.

`ListModels` và submit tiếp tục lọc account:

- account thuộc group/platform candidate;
- `account.IsSchedulable()`;
- provider hỗ trợ account;
- model lấy từ account mapping;
- pricing cho model tồn tại;
- `account.IsModelSupported(model)`.

Provider registry hiện có `gemini-api` và `vertex`, cùng map về platform `gemini`. Vì vậy batch không thể được bật chỉ vì group có `allow_image_generation`.

Các giới hạn/contract cần đưa vào guide khi batch thực sự available: idempotency key; tối đa items/output/reference theo config; reference image limit theo model family; public status (`queued`, `running`, `processing_results`, `settling`, `completed`, `failed`, `cancelled`, `output_deleted`); list/items/download/cancel/delete semantics.

### 3.5 Video qua Grok media

| Method | Path hiện có | Điều kiện route |
|---|---|---|
| POST | `/v1/videos`, `/videos` | group platform `grok` hoặc `composite` |
| POST | `/v1/videos/generations`, `/videos/generations` | `grok` hoặc `composite` |
| POST | `/v1/videos/edits`, `/videos/edits` | chỉ `grok` |
| POST | `/v1/videos/extensions`, `/videos/extensions` | chỉ `grok` |
| GET | `/v1/videos/.../:request_id` và alias | `grok` hoặc `composite` |
| GET | `/v1/videos/.../:request_id/content` và alias | `grok` hoặc `composite` |

Đây là bất đối xứng có thật trong `gateway.go`: composite được phép tạo/status/content nhưng edit/extension bị chặn ở route handler. Không viết “composite không hỗ trợ video” một cách tổng quát.

Video là async: client tạo job, poll status, rồi lấy content. Request parser/forwarder hỗ trợ model, prompt, số lượng, size/aspect ratio, resolution, duration, image/reference input và mask tùy endpoint. Edit source image có giới hạn tối đa 3 ảnh theo constant hiện tại.

Handler/service còn dùng:

- `SelectMediaVideoRequestAccount` hoặc `SelectAccountWithSchedulerForCapability`;
- model mapping của account;
- `OpenAIEndpointCapabilityGrokMediaGeneration`;
- eligibility/billing/runtime checks riêng của Grok;
- forwarder chỉ nhận account platform `grok`.

Với composite, resolver phải chạy qua đường target-platform/account-selection hiện có. Không được kết luận từ `composite_model_routes` rỗng; runtime snapshot hiện có bảng này bằng 0 nhưng group composite vẫn có account/model mappings media.

### 3.6 Seedance task API

Gateway đăng ký ba method cho từng prefix `/api/v3`, `/v3`, `/v1` và root:

| Method | Path |
|---|---|
| POST | `{prefix}/contents/generations/tasks` |
| GET | `{prefix}/contents/generations/tasks/:task_id` |
| DELETE | `{prefix}/contents/generations/tasks/:task_id` |

Handler là `OpenAIGateway.SeedanceTasks`; service `ForwardSeedance` yêu cầu:

- endpoint là create/status/delete Seedance;
- account có `OpenAIEndpointCapabilitySeedance`;
- account là API-key style OpenAI account có custom base URL hợp lệ;
- request model được account mapping rewrite;
- create response có task ID.

Do đó `platform=openai` hoặc mapping có keyword `seedance` chưa đủ. Seedance capability chỉ true khi account candidate thực sự thỏa endpoint capability và scheduler scope của key.

### 3.7 Gemini native image response

Ngoài OpenAI-compatible `/images/*` và batch image, snapshot hiện có Gemini native content route:

| Method | Path hiện có | Handler / route gate |
|---|---|---|
| POST | `/v1beta/models/{model}:generateContent` | `GeminiV1BetaModels`; Gemini group, or composite group resolved to Gemini |
| POST | `/v1beta/models/{model}:streamGenerateContent?alt=sse` | `GeminiV1BetaModels`; Gemini group, or composite group resolved to Gemini |
| POST | `/antigravity/v1beta/models/{model}:generateContent` | `GeminiV1BetaModels`; forced Antigravity platform; assigned group required |
| POST | `/antigravity/v1beta/models/{model}:streamGenerateContent?alt=sse` | `GeminiV1BetaModels`; forced Antigravity platform; assigned group required |

`backend/internal/pkg/antigravity/gemini_types.go` có `generationConfig.imageConfig` với `aspectRatio` và `imageSize`; default Gemini/Antigravity model mapping có aliases image cụ thể. Handler chọn Gemini account theo group/model; request được forward qua AI Studio-compatible path hoặc Antigravity native forwarder tùy account/platform, với account-specific model rewrite/support checks. Đây không phải khẳng định mọi lane/account type hỗ trợ image. Antigravity usage parsing có trường `ImageOutputTokens` khi upstream metadata cung cấp.

Đây là **native multimodal/content generation**, không phải OpenAI Images API. Resolver không được biến mọi request `generateContent` thành “image available”, cũng không được coi keyword `gemini-*image` là proof duy nhất. Native image chỉ available khi đồng thời:

- key có group/force-platform phù hợp với Gemini native route;
- model client được group allowlist cho phép;
- có account active+schedulable trong đúng scheduler scope;
- account mapping và `IsModelSupported` chấp nhận model client, với mapped model route-compatible;
- account type/provider lane có thể forward native action;
- model/provider thực tế chấp nhận image generation config hoặc image output; nếu chưa thể prove bằng registry/contract thì trả `account_capability_unverified`;
- capability không bị nhầm với OpenAI image sync, async task hay batch image.

Use Key phải hiển thị native Gemini image bằng section/endpoint riêng, ví dụ `gemini_native.image_generation`, với sample `generateContent` và `streamGenerateContent`. Không dùng path `/v1/images/generations` cho capability này. Nếu sản phẩm muốn hiển thị model IDs, chỉ lấy IDs từ model discovery/mapping đã lọc bởi group allowlist và scheduler predicate; không hardcode danh sách image model trong frontend.

**Giới hạn bằng chứng hiện tại:** source xác nhận route, account selection, mapping, `imageConfig` type và usage observation; chưa có capability endpoint mới hoặc runtime request thành công cho từng mapping. Vì vậy đây là capability cần đưa vào resolver/TDD, không phải tuyên bố mọi Gemini account hiện đang tạo ảnh được.

---

## 4. Runtime DB evidence (không chứa secret)

Các số dưới đây là snapshot nhất quán trong transaction `REPEATABLE READ` ngày `2026-10-10`; chỉ dùng để định hướng kiểm thử/resolver, không hardcode vào sản phẩm.

- Active groups: **18**.
- Active accounts: **92**; active+schedulable: **44**.
- API keys: **1,827** active; **1** active key không có group; **1,843** non-deleted keys. Counts chỉ mô tả snapshot DB, không phải acceptance criterion.
- Candidate discovery trên active+schedulable account mappings tìm thấy **11** account có model name khớp họ media và **10** có model name khớp họ image. Đây là candidate discovery, không phải proof capability cuối; một account có thể thuộc nhiều group.

### 4.1 Group snapshot

Các cột là member accounts, active+schedulable accounts, active+schedulable accounts có candidate media mapping. Candidate được tìm bằng keyword chỉ để kiểm tra DB; không được dùng làm resolver production.

| ID | Group | Platform | Image gate | Batch gate | Accounts | Schedulable | Candidate media accounts |
|---:|---|---|:---:|:---:|---:|---:|---:|
| 14 | CodexPLUS | openai | no | no | 25 | 7 | 1 |
| 26 | CC-MAX | anthropic | no | no | 8 | 5 | 0 |
| 27 | CC-Kiro | anthropic | no | no | 12 | 2 | 0 |
| 31 | CodexPRO | openai | no | no | 15 | 5 | 1 |
| 33 | Gemini | gemini | no | no | 1 | 1 | 0 |
| 40 | Grok-Heavy | grok | yes | no | 8 | 5 | 3 |
| 42 | OpenCode | opencode | no | no | 0 | 0 | 0 |
| 45 | CC-private | anthropic | no | no | 1 | 0 | 0 |
| 46 | Image | openai | yes | no | 5 | 3 | 3 |
| 47 | Grok-Siêu rẻ | grok | yes | no | 0 | 0 | 0 |
| 48 | Video | composite | yes | no | 6 | 5 | 5 |
| 50 | Deepseek | deepseek | no | no | 1 | 0 | 0 |
| 51 | Zhipu GLM | zhipu | no | no | 4 | 1 | 0 |
| 52 | Kimi | kimi | no | no | 1 | 0 | 0 |
| 53 | Minimax | minimax | no | no | 0 | 0 | 0 |
| 55 | China Model | composite | no | no | 12 | 7 | 0 |
| 56 | GPT-Luna | openai | no | no | 1 | 0 | 0 |
| 57 | TypeSafe / JEV | typesafe | no | no | 6 | 6 | 0 |

### 4.2 Mapping examples

Runtime mapping candidate rows include, without retaining any credential value:

- OpenAI groups: `gpt-image-*`;
- Grok group 40: `grok-imagine-*`, `grok-video-*`;
- composite group 48: Seedance, Sora, Veo, Wan, Kling and related media model names;
- another OpenAI account contains Gemini image aliases alongside GPT image aliases.

Các dòng trên chỉ xác nhận cần có capability resolver model-aware. Chúng **không** chứng minh mọi alias dùng được cho mọi route. Resolver phải kiểm tra account method/capability, provider support, group gate, model allowlist, scheduler và feature config.

Đặc biệt:

- Group 46 có image gate và candidate image accounts: đây là evidence để test positive path, không phải phê duyệt production request.
- Group 40 có Grok image/video candidates và image gate: cần test cả image và video selector.
- Group 48 có composite media candidates và image gate: route video composite có thể được xem xét; image sync và video edit/extension vẫn phải theo route guard.
- Tất cả 18 active groups trong snapshot đều có batch gate `false`; hiện chưa có group đủ điều kiện batch image từ DB snapshot này.
- Không dùng tên `Image`/`Video` để gắn capability.

---

## 5. Thiết kế capability contract

### 5.1 Endpoint đề xuất

Đặt trong authenticated user route hiện có:

```text
GET /api/v1/user/api-keys/:id/capabilities
```

Frontend `apiClient` dùng path tương đối:

```text
GET /user/api-keys/:id/capabilities
```

Đây là endpoint JWT của user panel, không phải endpoint gateway dùng API key. Handler phải kiểm tra key ID thuộc user hiện tại. Không đưa secret key vào URL/query/body.

### 5.2 Response đề xuất

```json
{
  "object": "api_key_capabilities",
  "key_id": 123,
  "group": {
    "id": 48,
    "platform": "composite"
  },
  "image": {
    "available": false,
    "gate_open": true,
    "account_available": true,
    "sync": {
      "available": false,
      "reason": "platform_unsupported"
    },
    "async": {
      "generation": { "available": false, "reason": "platform_unsupported" },
      "edit": { "available": false, "reason": "platform_unsupported" },
      "poll": { "available": false, "reason": "platform_unsupported" }
    }
  },
  "batch_image": {
    "available": false,
    "reason": "group_disabled",
    "models": { "available": false, "method": "GET", "path": "/v1/images/batches/models" }
  },
  "video": {
    "generation": { "available": true, "method": "POST", "path": "/v1/videos/generations" },
    "status": { "available": true, "method": "GET", "path": "/v1/videos/generations/{request_id}" },
    "content": { "available": true, "method": "GET", "path": "/v1/videos/generations/{request_id}/content" },
    "edit": { "available": false, "reason": "platform_unsupported" },
    "extension": { "available": false, "reason": "platform_unsupported" }
  },
  "seedance": {
    "available": false,
    "reason": "account_capability_unverified"
  },
  "gemini_native": {
    "image_generation": {
      "available": false,
      "reason": "account_capability_unverified",
      "generate_content": {
        "available": false,
        "method": "POST",
        "path": "/v1beta/models/{model}:generateContent"
      },
      "stream_generate_content": {
        "available": false,
        "method": "POST",
        "path": "/v1beta/models/{model}:streamGenerateContent?alt=sse"
      }
    }
  }
}
```

Schema rules:

- `available` means the route is eligible after gate + account/provider/model/feature checks, not merely that the route exists in Gin.
- Keep `gate_open`, `account_available`, and route availability separate so “gate mở nhưng không có account” is explainable.
- Reasons are a closed public enum only:
  `no_group`, `group_disabled`, `platform_unsupported`, `feature_disabled`, `no_account`, `model_not_available`, `account_capability_unverified`, `route_unsupported`.
- Do not return account IDs, account names, provider credentials, `base_url`, upstream mapped target, billing status details, or raw error text.
- Do not return a static model list from this endpoint. Link to `/v1/models` or `/v1/images/batches/models` where an existing discovery contract is authoritative. If the product needs model chips in the modal, expose only client-facing model IDs after applying the exact route/scheduler predicate in a separate field/endpoint.
- `group.name` is intentionally omitted from the capability contract; UI must not derive capability from a display name.

### 5.3 Resolver contract

Proposed service: `backend/internal/service/api_key_capabilities.go`.

Inputs:

- current user ID;
- API key ID;
- resolved key/group;
- account repository/scheduler candidate query;
- group/model allowlist evaluator;
- object-storage/task feature state;
- provider capability registry where applicable.

Rules:

1. Missing key/group returns a safe all-unavailable response, not 500.
2. Candidate account query reuses `ListModelAvailabilityCandidates` or an equivalent repository method with the same group/platform scope as the scheduler.
3. Candidate account must be active, schedulable and in the key’s group scope; transient rate-limit/overload state is not silently converted into permanent “no account”.
4. For each route, evaluate the same endpoint capability used by the real handler.
5. For model-dependent routes, use `IsModelSupported`/mapped model compatibility; do not match only by substring.
6. For composite groups, resolve target platform with the existing composite path before checking account/provider support.
7. Any resolver/database/provider lookup failure returns `available=false` with `account_capability_unverified` or a safe equivalent; it must not claim a capability that a request cannot prove.
8. Resolver does not mutate scheduler state, quota, usage, task records or billing holds.
9. Capability results are recomputed on request or behind a short bounded cache with invalidation on group/account/mapping/feature changes; no long-lived frontend cache.
10. Model allowlist remains an independent gateway guard; the capability endpoint must not bypass it.

### 5.4 Route-specific truth table

| Capability | Required checks |
|---|---|
| Image sync | image gate; platform route is OpenAI/Grok; account candidate; route image capability; requested/discovered model supported |
| Image async submit | image sync checks; object storage/task service enabled; non-stream request contract |
| Image async poll | task store pollable; ownership is enforced by task handler; submit may be disabled while poll remains available |
| Batch image | batch gate; group platform Gemini; provider registry; schedulable Gemini account; mapped model; pricing; model allowlist |
| Grok video generation | image/media gate; Grok or composite route resolution; eligible media account; Grok media capability; mapped model; billing/runtime eligibility |
| Grok video status/content | generation route was eligible and task ID is supported; route allows resolved Grok/composite target; task content contract |
| Grok video edit/extension | Grok route support; never mark composite true while current handler rejects it |
| Seedance | OpenAI target; API-key account; custom base URL; Seedance endpoint capability; model mapping; scheduler/account eligibility |
| Gemini native image response | Gemini/Antigravity native route; Gemini or composite→Gemini target / forced Antigravity with group guard; allowlisted client model; active+schedulable native account; mapping/`IsModelSupported`; provider lane; image-capable model/request contract; native `generateContent` or `streamGenerateContent` |

---

## 6. Use Key UI design

### 6.1 Data flow

1. `KeysView.vue` passes `selectedKey.id` to `UseKeyModal` as a new non-secret `apiKeyId` prop.
2. `UseKeyModal.vue` requests `/user/api-keys/{id}/capabilities` when opened or when key ID changes.
3. Loading/error state does not remove existing Claude/Codex/Gemini/OpenCode tabs.
4. The new `API` tab appears only when at least one capability is available, or when the response is loaded and has an explicit “no media capability” state chosen by product.
5. Copy examples use the existing `baseUrl` prop. Never use a hardcoded production domain.
6. The modal does not send `apiKey` secret to the capability endpoint and does not include it in telemetry/logging.

### 6.2 API tab sections

- **Image**: Sync example; Async submit example; polling example; `stream:true` warning. Render only supported variants.
- **Gemini native image**: `generateContent` and `streamGenerateContent` examples with `generationConfig.imageConfig`; render only when native Gemini capability is available. Keep this separate from OpenAI `/images/*`, async image tasks and batch image.
- **Batch image**: show models discovery path and submit/list/status/download examples only when available; otherwise show a localized reason.
- **Video**: create → poll → content examples. Show edit/extension only if each field is true; composite must show generation-only explanation when applicable.
- **Seedance**: create/status/delete examples with all accepted prefixes summarized; render only after account endpoint capability is proven.

Placeholders are visual examples only: `{base_url}`, `{api_key}`, `{model}`, `{task_id}`, `{request_id}`. The real API key may be rendered only in the existing local copy flow; it must never be sent to the capability resolver or persisted by this feature.

### 6.3 Accessibility and i18n

- Use `role="tab"`, `aria-selected`, `aria-controls`, and `role="tabpanel"` for the new API tab/sections.
- Preserve keyboard navigation and existing modal focus behavior.
- Add every new leaf key to English and Vietnamese.
- Run the existing parity command, which is explicitly:

```bash
cd frontend
pnpm run check:i18n
```

Required key areas:

```text
keys.useKeyModal.api.tab
keys.useKeyModal.api.loading
keys.useKeyModal.api.error
keys.useKeyModal.api.image.*
keys.useKeyModal.api.batchImage.*
keys.useKeyModal.api.video.*
keys.useKeyModal.api.seedance.*
keys.useKeyModal.api.reason.*
```

---

## 7. TDD plan

### 7.1 Service resolver tests

Use fake repositories/accounts with synthetic non-secret credentials and explicit model mappings. Test table:

1. key missing → all unavailable, `no_group`.
2. inactive/deleted group → all unavailable.
3. image gate closed, account has image mapping → image unavailable, `group_disabled`.
4. image gate open, no eligible account → `account_available=false`, `no_account`.
5. OpenAI image account + mapping + supported endpoint → sync available.
6. Grok image account + mapping + Grok media eligibility → sync available.
7. composite image mapping alone → not enough; route/provider check controls result.
8. async submit disabled by object storage → sync may remain available, async submit `feature_disabled`.
9. async polling remains separately representable when task store is pollable.
10. batch with flag false → `group_disabled`.
11. batch flag true but non-Gemini group → `platform_unsupported`.
12. batch flag/platform/account/model/pricing all pass → available.
13. batch account mapping contains candidate keyword but `IsModelSupported` rejects → unavailable.
14. Grok video generation eligible → generation/status/content available.
15. composite video generation eligible but current route rejects edit/extension → edit/extension false with `route_unsupported`.
16. Seedance OpenAI account lacking endpoint capability → false.
17. Seedance API-key account with custom base URL/capability/mapping → true.
18. Gemini native image model mapping alone → not enough; group/force-platform, allowlist, schedulable account, provider lane and image request/model contract all pass → native image available.
19. Gemini native text-only model or unsupported provider lane → native image unavailable without changing ordinary native text capability.
20. empty `composite_model_routes` does not force a negative result when existing composite target resolution proves an eligible account.
21. repository/provider error → safe unavailable/unverified result; no optimistic true.
22. response serialization has no `credential`, `api_key`, `token`, `password`, `secret`, `base_url`, account ID or upstream target fields.
23. reason values are closed-enum validated.
24. allowlist-rejected model never appears as available.

### 7.2 Handler/auth tests

1. unauthenticated `GET /api/v1/user/api-keys/:id/capabilities` → 401.
2. current user requests another user’s key → existing not-found/forbidden policy, never capability leak.
3. own key → 200 JSON with only public contract fields.
4. ungrouped key → safe all-unavailable response.
5. deleted/inactive key → existing key access semantics, no media data leak.
6. resolver error → stable safe response and structured server log without secret values.
7. repeated request after group/account mapping change reflects invalidation or bounded cache behavior.

### 7.3 Gateway contract/regression tests

Keep existing route tests and add focused cases asserting that capability semantics mirror the route guards:

- OpenAI/Grok image routes versus composite/other platform 404.
- Async object-storage disabled/enabled behavior.
- Batch route gate requires both flag and Gemini platform.
- Composite video generation/status/content accepted, composite edit/extension rejected under current code.
- Seedance prefixes `/api/v3`, `/v3`, `/v1`, root all remain registered.
- `groupModelAllowlist` remains independent and is not weakened by the new endpoint.

Do not implement a brittle test that parses source text only. Prefer httptest router registration plus fake handler/service dependencies; if a route registry is introduced, test the registry as the single route contract.

### 7.4 Frontend tests

1. capability loading/error keeps existing client tabs usable.
2. API tab is absent when the response has no available capability.
3. image sync/async render independently.
4. Gemini native image section renders only for native capability and uses `/v1beta/models/{model}:generateContent` or `streamGenerateContent`; it never substitutes `/v1/images/generations`.
5. batch disabled renders localized reason, not a fake example.
6. composite generation-only video renders no edit/extension command.
7. Seedance is hidden unless the backend says available.
8. copied snippets contain the `baseUrl` prop and never a hardcoded domain.
9. null group/platform does not crash.
10. API key ID, not API key secret, is used for capability fetch.
11. English/Vietnamese leaf-key parity passes.
12. keyboard and ARIA behavior passes.

### 7.5 Verification commands after implementation

Backend, from the repository's Go module directory `backend/`:

```bash
cd backend
go test ./internal/service/... ./internal/handler/... ./internal/server/routes/...
go test ./...
go vet ./...
golangci-lint run ./...
```

Frontend:

```bash
cd frontend
pnpm run check:i18n
pnpm run typecheck
pnpm run lint:check
pnpm run test:run
pnpm run build
```

Before finalizing, inspect:

```bash
git diff --check
git status --short
git diff --stat
```

No production rollout is part of this document. A rollout, migration, group flag change, account change or external API write requires separate explicit approval and CI evidence.

---

## 8. Files expected to change when implementation is approved

Backend:

- `backend/internal/service/api_key_capabilities.go` — resolver, public reason enum and route-specific predicates.
- `backend/internal/service/api_key_capabilities_test.go` — unit/table tests.
- `backend/internal/handler/api_key_capabilities_handler.go` — authenticated user handler.
- `backend/internal/handler/wire.go` — handler dependency wiring.
- `backend/internal/server/routes/user.go` — user capability route.
- Existing repository/service files only if a small shared query interface is required; do not duplicate scheduler logic.

Frontend:

- `frontend/src/api/keys.ts` — capability fetch API and DTO types (or a dedicated `api/capabilities.ts` if repo convention prefers).
- `frontend/src/components/keys/UseKeyModal.vue` — API tab, loading/error/sections and accessibility.
- `frontend/src/views/user/KeysView.vue` — pass non-secret `apiKeyId`.
- `frontend/src/i18n/locales/en.ts` and `frontend/src/i18n/locales/vi.ts` — complete leaf-key parity.
- Focused component/API tests next to existing frontend test conventions.

No migration is expected: the design reads existing groups/accounts/mappings and existing feature configuration.

---

## 9. Risks and mitigations

| Risk | Mitigation |
|---|---|
| Capability drifts from scheduler | Reuse existing candidate/account predicates; add route-contract tests. |
| Group gate is mistaken for model availability | Separate gate/account/model/route fields and tests. |
| Model keyword produces false positive | Require `IsModelSupported` plus endpoint/provider capability. |
| Composite route asymmetry is hidden | Separate generation/status/content/edit/extension fields. |
| Async feature is advertised without object storage | Read task service enabled/pollable state. |
| Batch model list becomes stale | Link to existing `/images/batches/models`; do not hardcode frontend list. |
| Capability endpoint leaks topology/secrets | Closed reason enum; no account/provider/upstream fields; serialization tests. |
| Runtime query snapshot changes | Treat DB counts as evidence only; test behavior using fixtures and live verification in staging. |
| UI API request causes secret exposure | Fetch by numeric key ID through JWT user route; never send secret to resolver. |

---

## 10. Approval gates before implementation

1. Confirm the proposed capability endpoint namespace and response contract.
2. Confirm whether “API” tab should be hidden when all media capabilities are unavailable or should show a localized explanation.
3. Confirm whether the product wants public model IDs in the capability response, or only links to existing model discovery endpoints.
4. Confirm expected behavior for composite video generation-only support; current route code rejects composite edit/extension.
5. After approval, implement backend contract first, run focused tests, then wire the frontend. Do not alter runtime groups/accounts during implementation.
