# Multimodal embeddings

Sub2API forwards structured multimodal embedding requests through the existing
OpenAI API-key account, authentication, model allowlist, scheduling, failover,
and usage-recording pipeline.

## Configure an upstream account

For Volcengine Ark, add an **OpenAI API Key** account with:

- Base URL: `https://ark.cn-beijing.volces.com/api/v3`
- API key: the upstream Ark API key
- Model mapping/allowlist: include the enabled upstream model, for example
  `doubao-embedding-vision-251215`, or map a public model alias to it

Assign the account to an OpenAI group and use a Sub2API key for that group.
Composite groups can use an explicit `embeddings` route targeting OpenAI.
Configure pricing for the mapped embedding model as for existing embeddings;
upstream image-token usage is preserved for image-input pricing.

## Call the gateway

The following POST routes are equivalent:

```text
/v1/embeddings/multimodal
/embeddings/multimodal
/api/v3/embeddings/multimodal
/v3/embeddings/multimodal
```

```bash
curl "$SUB2API_BASE_URL/v1/embeddings/multimodal" \
  -H "Authorization: Bearer $SUB2API_KEY" \
  -H 'Content-Type: application/json' \
  -d '{
    "model": "doubao-embedding-vision-251215",
    "input": [
      {"type": "text", "text": "A blue sky above the sea"},
      {
        "type": "image_url",
        "image_url": {
          "url": "https://ark-project.tos-cn-beijing.volces.com/images/view.jpeg"
        }
      }
    ],
    "encoding_format": "float",
    "dimensions": 1024
  }'
```

Ark SDK clients can set their Base URL to `$SUB2API_BASE_URL/api/v3` and call
the SDK's multimodal embeddings method with a Sub2API key.

The gateway preserves structured `input`, including image URLs, Base64 data
URLs, and video objects supported by the upstream. Parameters such as
`instructions`, `multi_embedding`, and `sparse_embedding` are forwarded as
provided. Only `model` is rewritten when a channel/account mapping applies.
Upstream input validation, limits, and response/error formats still apply.

The response stays in the upstream's native format. Ark may return a single
`data.embedding` object rather than OpenAI's `data[]` array. Token details such
as `usage.prompt_tokens_details.image_tokens` feed the existing usage parser.

Multimodal requests use `gateway.max_body_size` and the shared request-body
memory budget so inline images are not constrained by the smaller
`gateway.text_max_body_size`. Ordinary `/v1/embeddings` and `/embeddings` retain
their text-body limit and ordinary upstream endpoint; the gateway does not
infer a different endpoint from a model name or convert text inputs to image
objects.

See the [Ark multimodal embeddings API](https://ark.volcengine.com/region:cn-beijing/docs/multimodal-vectorization-api)
for model-specific inputs and options.
