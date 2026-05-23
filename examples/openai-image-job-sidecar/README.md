# OpenAI Image Job Sidecar

This example sidecar wraps Sub2API's synchronous OpenAI-compatible image
endpoints in an async job API. It is useful when a public CDN or reverse proxy
has a shorter origin response timeout than high-resolution image generation.

The sidecar does not replace Sub2API. It runs beside Sub2API and calls the local
Sub2API `/v1/images/generations` or `/v1/images/edits` endpoint in the
background.

```text
Client
  -> POST /image-jobs
     <- 202 + job_id
  -> GET /image-jobs/:job_id
     <- queued | running | succeeded | failed

Image job sidecar
  -> background request to local Sub2API
     /v1/images/generations
     /v1/images/edits
```

## Run

```bash
cp .env.example .env
npm start
```

Use Docker or a process manager for production. Bind to localhost and expose it
through your own reverse proxy if it is public.

## API

Create a job:

```http
POST /image-jobs
Authorization: Bearer <SUB2API_IMAGE_JOB_PROXY_API_KEY>
Content-Type: application/json
```

```json
{
  "endpoint": "/v1/images/edits",
  "request": {
    "model": "gpt-image-2",
    "prompt": "Create a product image",
    "size": "1024x1024",
    "n": 1
  },
  "references": [
    {
      "filename": "product.png",
      "content_type": "image/png",
      "b64": "<base64 image bytes>"
    }
  ]
}
```

Poll:

```http
GET /image-jobs/<job_id>
Authorization: Bearer <SUB2API_IMAGE_JOB_PROXY_API_KEY>
```

Terminal success responses include the upstream Sub2API response under
`result`.

## Environment

- `PORT`: sidecar listen port, default `8788`.
- `SUB2API_IMAGE_JOB_UPSTREAM_BASE_URL`: local Sub2API origin, for example `http://127.0.0.1:8080`.
- `SUB2API_IMAGE_JOB_UPSTREAM_API_KEY`: Sub2API API key used by the sidecar.
- `SUB2API_IMAGE_JOB_PROXY_API_KEY`: client-facing bearer token for this sidecar.
- `SUB2API_IMAGE_JOB_REQUEST_TIMEOUT_MS`: background upstream timeout.
- `SUB2API_IMAGE_JOB_RETENTION_MS`: completed job retention window.
- `SUB2API_IMAGE_JOB_MAX_BODY_BYTES`: create-job request body limit.
- `SUB2API_IMAGE_JOB_MAX_JOBS`: in-memory job cap.
- `SUB2API_IMAGE_JOB_ALLOW_CLIENT_UPSTREAM_AUTH`: allow `X-Upstream-Authorization` forwarding. Prefer server-side upstream auth for production.

## Production Notes

- Jobs are stored in memory. Restarting the sidecar clears active jobs.
- Use Redis, Postgres, or another persistent queue before relying on this pattern for critical production workloads.
- Keep the normal Sub2API `/v1/images/*` endpoints available for clients that do not need async image jobs.
- Do not expose the sidecar publicly without `SUB2API_IMAGE_JOB_PROXY_API_KEY`.
