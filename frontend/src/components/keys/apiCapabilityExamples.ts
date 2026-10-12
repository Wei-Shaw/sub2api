export interface CapabilityAPIExample {
  request: string
  response: string
  responseNote?: string
}

const buildJSONExample = (url: string, body: string, response: string): CapabilityAPIExample => ({
  request: `curl -X POST '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]' \\\n  -H 'Content-Type: application/json' \\\n  -d '${body}'`,
  response
})

const exampleBuilders: Record<string, (url: string) => CapabilityAPIExample> = {
  'image-sync': (url) => buildJSONExample(
    url,
    '{"model":"gpt-image-1","prompt":"A watercolor mountain landscape","n":1,"size":"1024x1024"}',
    '{\n  "created": 1710000000,\n  "data": [{"b64_json": "{base64_image}"}]\n}'
  ),
  'image-async-generation': (url) => buildJSONExample(
    url,
    '{"model":"gpt-image-1","prompt":"A watercolor mountain landscape","n":1,"size":"1024x1024"}',
    '{\n  "id": "imgtask_...",\n  "task_id": "imgtask_...",\n  "object": "image.generation.task",\n  "status": "processing",\n  "created_at": 1710000000,\n  "expires_at": 1710086400,\n  "poll_url": "/v1/images/tasks/imgtask_..."\n}'
  ),
  'image-async-edit': (url) => buildJSONExample(
    url,
    '{"model":"gpt-image-1","prompt":"Add warm sunlight","image":{"url":"https://example.com/input.png"}}',
    '{\n  "id": "imgtask_...",\n  "task_id": "imgtask_...",\n  "object": "image.generation.task",\n  "status": "processing",\n  "poll_url": "/v1/images/tasks/imgtask_..."\n}'
  ),
  'image-async-poll': (url) => ({
    request: `curl -X GET '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]'`,
    response: '{\n  "id": "imgtask_...",\n  "task_id": "imgtask_...",\n  "object": "image.generation.task",\n  "status": "completed",\n  "http_status": 200,\n  "result": {"created": 1710000000, "data": [{"b64_json": "{base64_image}"}]},\n  "created_at": 1710000000,\n  "completed_at": 1710000030,\n  "expires_at": 1710086400\n}'
  }),
  'batch-image': (url) => ({
    request: `curl -X GET '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]'`,
    response: '{\n  "object": "list",\n  "data": [{\n    "id": "gemini-2.5-flash-image",\n    "object": "image.batch.model",\n    "provider": "gemini_api"\n  }]\n}'
  }),
  'gemini-native-generate-content': (url) => buildJSONExample(
    url,
    '{"contents":[{"parts":[{"text":"Create a watercolor mountain landscape"}]}],"generationConfig":{"imageConfig":{"aspectRatio":"1:1"}}}',
    '{\n  "candidates": [{\n    "content": {\n      "role": "model",\n      "parts": [{"inlineData": {"mimeType": "image/png", "data": "{base64_image}"}}]\n    },\n    "finishReason": "STOP"\n  }]\n}'
  ),
  'gemini-native-stream-generate-content': (url) => ({
    request: `curl -N -X POST '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]' \\\n  -H 'Content-Type: application/json' \\\n  -d '{"contents":[{"parts":[{"text":"Create a watercolor mountain landscape"}]}],"generationConfig":{"imageConfig":{"aspectRatio":"1:1"}}}'`,
    response: 'data: {"candidates":[{"content":{"role":"model","parts":[{"inlineData":{"mimeType":"image/png","data":"{base64_image_chunk}"}}]},"finishReason":"STOP"}]}\n\ndata: [DONE]',
    responseNote: 'The response is Server-Sent Events (SSE); read each data event until [DONE].'
  }),
  'video-generation': (url) => buildJSONExample(
    url,
    '{"model":"grok-imagine-video-1.5","prompt":"A cinematic sunrise over the mountains","duration":8,"aspect_ratio":"16:9","resolution":"720p"}',
    '{\n  "request_id": "d97415a1-5796-b7ec-379f-4e6819e08fdf"\n}'
  ),
  'video-status': (url) => ({
    request: `curl -X GET '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]'`,
    response: '{\n  "status": "done",\n  "model": "grok-imagine-video-1.5",\n  "video": {\n    "url": "https://vidgen.x.ai/.../video.mp4",\n    "duration": 8,\n    "respect_moderation": true\n  }\n}',
    responseNote: 'Poll the request ID every few seconds. Status values are pending, done, expired, or failed. When done, use the content endpoint to download the binary video.'
  }),
  'video-content': (url) => ({
    request: `curl --fail --location '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]' \\\n  --output video.mp4`,
    response: 'HTTP/1.1 200 OK\nContent-Type: video/mp4\n\n<binary video bytes>',
    responseNote: 'This endpoint returns the video binary, not a JSON envelope. Save the response body as an .mp4 file.'
  }),
  'video-edit': (url) => buildJSONExample(
    url,
    '{"model":"grok-imagine-video","prompt":"Add a slow camera pan","video":{"url":"https://example.com/input.mp4"}}',
    '{\n  "request_id": "d97415a1-5796-b7ec-379f-4e6819e08fdf"\n}'
  ),
  'video-extension': (url) => buildJSONExample(
    url,
    '{"model":"grok-imagine-video","prompt":"Continue the camera pan","duration":5,"video":{"url":"https://example.com/input.mp4"}}',
    '{\n  "request_id": "d97415a1-5796-b7ec-379f-4e6819e08fdf"\n}'
  ),
  'seedance-create': (url) => buildJSONExample(
    url,
    '{"model":"doubao-seedance-1-0-pro","content":[{"type":"text","text":"A cinematic sunrise over the mountains"}]}',
    '{\n  "id": "task_...",\n  "status": "queued"\n}'
  ),
  'seedance-status': (url) => ({
    request: `curl -X GET '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]'`,
    response: '{\n  "id": "task_...",\n  "status": "succeeded",\n  "model": "doubao-seedance-1-0-pro",\n  "content": [{"type": "video_url", "video_url": {"url": "https://example.com/video.mp4"}}]\n}'
  }),
  'seedance-delete': (url) => ({
    request: `curl -X DELETE '${url}' \\\n  -H 'Authorization: Bearer [REDACTED]'`,
    response: 'HTTP/1.1 204 No Content\n\n<empty response body>',
    responseNote: 'The upstream task is deleted; there is no JSON response body.'
  })
}

export function buildCapabilityAPIExample(id: string, url: string): CapabilityAPIExample | null {
  return exampleBuilders[id]?.(url) ?? null
}
