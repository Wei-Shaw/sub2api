# Images Status API

## Overview

Image generation and image edit requests can publish a short-lived status record when the client provides `x-client-request-id`.

Supported producers:

- `POST /v1/images/generations`
- `POST /v1/images/edits`
- `POST /v1/responses` requests that use the `image_generation` tool, including both generate and edit actions

Tracked Images API and Responses image outputs use the same result pipeline. When the group has image upscale enabled and the request asks for an upscale-eligible size, the returned image is upscaled before the final status is written. Upscale results keep the fal image URL in `urls` so clients can still retrieve the image when COS is not configured or COS upload fails. When COS upload is enabled, successful output images are uploaded to COS and the status result includes `cos_urls`. Responses image requests that also return assistant text include that text in `texts`.

If the client disconnects while a tracked request is still running, the server continues the upstream image workflow when possible and keeps publishing progress/result updates for later status polling.

Status records are stored for 7 days under `gen_img:<request_id>`. The key is not scoped by API key. If the same request ID is reused, the later request can overwrite the previous status. Clients should use high-entropy unique request IDs.

## Start Tracking

```http
POST /v1/images/generations
Authorization: Bearer sk-...
x-client-request-id: img_01HXAMPLE
Content-Type: application/json
```

```http
POST /v1/images/edits
Authorization: Bearer sk-...
x-client-request-id: img_01HXEDIT
Content-Type: application/json
```

```http
POST /v1/responses
Authorization: Bearer sk-...
x-client-request-id: img_01HXAMPLE
Content-Type: application/json
```

Only requests that include the header are tracked. Requests without `x-client-request-id` are processed normally and do not need to create an image status record.

## Query Status

```http
GET /v1/images/status/?request_ids=img_01HXAMPLE,img_01HXBATCH
Authorization: Bearer sk-...
```

The query endpoint requires a valid API key. The API key is only a permission gate; the lookup uses `gen_img:<request_id>` and does not enforce ownership matching.

Query parameters:

- `request_ids`: Comma-separated request IDs.
- `request_id`: One request ID. It can be repeated and can be combined with `request_ids`.
- `ids` and `id`: Accepted aliases.

Duplicate IDs are ignored in the response. A single query supports at most 100 unique request IDs. Requests with more than 100 IDs return `400 invalid_request_error`.

## Response Fields

```json
{
  "data": [
    {
      "request_id": "img_01HXAMPLE",
      "status": "succeeded",
      "progress": 100,
      "urls": ["https://upstream.example/image.png"],
      "cos_urls": ["https://cos.example/image.png"],
      "texts": ["Generated the image and adjusted the prompt."],
      "created_at": "2026-07-04T12:00:00Z",
      "updated_at": "2026-07-04T12:00:05Z"
    }
  ]
}
```

Fields:

- `request_id`: Client request ID from `x-client-request-id`.
- `status`: One of `accepted`, `running`, `upstream_done`, `cos_uploading`, `succeeded`, or `failed`.
- `progress`: Coarse progress from 0 to 100.
- `urls`: Upstream or post-processing fallback image URLs when available. For upscaled images this can include the fal result URL.
- `cos_urls`: COS image URLs when COS upload succeeds.
- `texts`: Assistant text returned alongside a `/v1/responses` image generation or edit result, when available.
- `error`: Present for failed requests.
- `created_at`, `updated_at`: Status timestamps.

When a batch query contains missing or expired IDs, the endpoint returns found records in `data` and missing IDs in `not_found`.

Clients should prefer `cos_urls` when present. If `cos_urls` is empty or contains an empty slot, fall back to `urls`.

```json
{
  "data": [
    {
      "request_id": "img_01HXAMPLE",
      "status": "running",
      "progress": 25,
      "created_at": "2026-07-04T12:00:00Z",
      "updated_at": "2026-07-04T12:00:01Z"
    }
  ],
  "not_found": ["img_expired"]
}
```

## Examples

Successful COS-backed result:

```json
{
  "data": [
    {
      "request_id": "img_01HXAMPLE",
      "status": "succeeded",
      "progress": 100,
      "cos_urls": ["https://cos.example/generated/img_01HXAMPLE.png"],
      "created_at": "2026-07-04T12:00:00Z",
      "updated_at": "2026-07-04T12:00:08Z"
    }
  ]
}
```

Failed result:

```json
{
  "data": [
    {
      "request_id": "img_01HXAMPLE",
      "status": "failed",
      "progress": 100,
      "error": {
        "message": "image generation failed"
      },
      "created_at": "2026-07-04T12:00:00Z",
      "updated_at": "2026-07-04T12:00:02Z"
    }
  ]
}
```

Missing or expired single-ID query:

```http
HTTP/1.1 404 Not Found
```

```json
{
  "error": {
    "type": "not_found_error",
    "message": "Image status not found"
  }
}
```
