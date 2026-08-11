/**
 * useModelIntroDocExtract：把"上游模型文档页的纯文本"交给大模型，解析成
 * 「模型介绍」编辑表单可直接回填的 JSON 草稿。
 *
 * 背景：管理员配置一个视频模型的介绍要手填非常多内容 —— 标题、中英文简介、
 * 输入参数 schema（含每个字段的类型/默认值/枚举/中英说明）、输出参数 schema、
 * 主结果字段…而这些信息在上游平台的模型文档页上基本都有。
 *
 * 流程（三步，前两步分别由后端 / 本模块负责）：
 *   1) 后端 POST /admin/model-intro-doc-fetch  —— 抓页面 + 抽正文纯文本
 *      （浏览器直连第三方站点会被 CORS 拦掉，且后端能统一做 SSRF/体积/超时限制）
 *   2) 本模块 extractModelIntroFromDoc()      —— 用管理员已选好的 API Key + chat
 *      模型，走同源 `/v1/chat/completions` 把纯文本解析成表单 JSON
 *   3) 视图层把 JSON 回填到编辑区，管理员再改再保存
 *
 * 复用关系：apiKey / model 直接复用编辑弹窗底部「翻译工具」里已选好的那一组
 * （见 useDescriptionTranslation.ts），不额外增加一套凭据选择 UI。
 */

/** 生成结果里"输入参数"的存储 shape 与 paramSchemaRow.ts 完全一致，这里不再重复建模。 */
export interface ModelIntroDraft {
  title?: string
  description?: string
  description_en?: string
  default_params?: Record<string, unknown>
  output_fields?: unknown[]
  result_field?: string
  result_type?: 'video' | 'image'
}

export interface ExtractModelIntroOptions {
  /** 文档页正文纯文本（由后端 fetchDoc 返回）。 */
  docText: string
  /** 文档页 <title>，作为额外线索传给模型（可选）。 */
  docTitle?: string
  /** 文档页 URL，作为额外线索（可选）。 */
  docUrl?: string
  /** 当前正在编辑的 model_key，帮助模型判断"该抽哪个模型的参数"（可选）。 */
  modelKey?: string
  /** 管理员补充的自然语言指示，例如"只要文生视频的参数"（可选）。 */
  hint?: string
  /** Bearer 凭据（网关自身的 API Key）。 */
  apiKey: string
  /** 用于解析的 chat 模型名。 */
  model: string
  /** 允许调用方取消请求。 */
  signal?: AbortSignal
}

export interface ExtractModelIntroResult {
  /** 解析出的草稿对象（已做基本形状归一）。 */
  draft: ModelIntroDraft
  /** 美化后的 JSON 文本，直接塞进导入 textarea 供管理员审阅。 */
  json: string
}

/**
 * SYSTEM_PROMPT：把"目标 JSON 形状"讲清楚是这个功能成败的关键。
 *
 * 这里刻意把存储 shape 一字不差地写出来（含 extra['x-order'] 这类扩展字段），
 * 因为视图层的 applyImport 就是按这份 shape 反解的；模型只要照抄结构，
 * 回填就不会丢字段。同时反复强调"只输出 JSON"，避免解释性文字污染结果。
 */
const SYSTEM_PROMPT = `You extract structured configuration from AI model documentation pages.

The user gives you the plain text of a model documentation page. You must output ONE JSON object that configures this model inside an admin console.

Output JSON shape (all keys optional, but include everything you can infer):
{
  "title": "short display title of the model",
  "description": "简体中文简介，2-4 句，说明这个模型能做什么、适合什么场景",
  "description_en": "English intro, 2-4 sentences, same meaning as description",
  "default_params": {
    "<param_name>": {
      "value": <default value: string | number | boolean>,
      "required": true,
      "description": "中文字段说明",
      "description_en": "English field description",
      "enum": true,
      "options": ["a", "b"],
      "widget": "textarea",
      "rows": 4,
      "maxItems": 4,
      "extra": { "x-order": 0, "advanced": true }
    }
  },
  "output_fields": [
    { "key": "video.url", "type": "string", "description": "中文说明", "required": true }
  ],
  "result_field": "video.url",
  "result_type": "video"
}

Rules for "default_params" (the request/input parameters of the model):
- One entry per documented input parameter, keyed by its exact API parameter name (snake_case as in the docs).
- "value" is the documented default. Use "" for required free-text params like prompt. Types must match the doc: number for numeric params, boolean for flags, string otherwise.
- Nested types are supported: use { "properties": { "<child>": <schema> }, ... } for objects and { "items": <schema>, ... } for arrays instead of "value".
- For an array of image URLs (image_urls, reference_images, frames, ...), use: { "items": { "value": "", "widget": "image" }, "widget": "imageUrls", "maxItems": N, "description": ..., "extra": { "x-order": N } }. "widget": "imageUrls" renders one gallery-style multi-image input; set "maxItems" to the documented maximum number of images (omit it when the docs state no limit). "maxItems" may also be used on any other array type to cap the element count.
- "required": true only for parameters the docs mark as required. Omit otherwise.
- "enum": true plus "options": [...] only when the docs list a fixed set of allowed values (e.g. aspect ratio, resolution, duration).
- "widget": "textarea" (with "rows": 3..6) for long text params such as prompt / negative_prompt. "widget": "image" for params that take an image URL (image_url, first_frame_image, reference images). Omit "widget" for everything else.
- "extra": { "x-order": N } must be present on every entry; N starts at 0 and increases by 10 following the documented order. Add "advanced": true inside "extra" for rarely used / expert parameters (seed, guidance scale, safety toggles, ...) so the playground hides them behind an "advanced" section.
- "description" MUST be Simplified Chinese; "description_en" MUST be English. Write both for every parameter. Include the valid range / unit when the docs state one.

Rules for "output_fields" (the response payload of the model):
- "type" is one of string | number | boolean | object | array.
- "key" is the path in the response payload, supporting property chains, indexes and wildcards: "video.url", "images[0].url", "images[*]", "seed".
- Use "properties" (for object) / "items" (for array) to describe nested structure, same shape as default_params.
- "description" MUST be Simplified Chinese.

Rules for "result_field" / "result_type":
- "result_field" must be one of the paths declared in output_fields, pointing at the main generated media URL (e.g. "video.url").
- "result_type" is "video" for video models, "image" for image models.

Hard requirements:
- Output ONLY the JSON object. No markdown fences, no comments, no explanation before or after.
- Never invent parameters that are not in the page. If the page documents several model variants, use the one matching the model key given by the user, otherwise the primary one.
- If the page contains no usable parameter information, return {} .`

/**
 * extractModelIntroFromDoc：调用 chat 模型把文档文本解析为表单草稿。
 *
 * 错误处理：HTTP 非 2xx / 空回复 / JSON 无法解析都抛 Error，message 里带上
 * 上游返回的片段，方便管理员判断是 key 无权、模型不支持还是页面内容不合适。
 */
export async function extractModelIntroFromDoc(
  opts: ExtractModelIntroOptions
): Promise<ExtractModelIntroResult> {
  const docText = (opts.docText ?? '').trim()
  if (!docText) throw new Error('document text is empty')
  if (!opts.apiKey.trim()) throw new Error('api key is empty')
  if (!opts.model.trim()) throw new Error('model is empty')

  const contextLines: string[] = []
  if (opts.modelKey?.trim()) contextLines.push(`Model key being configured: ${opts.modelKey.trim()}`)
  if (opts.docUrl?.trim()) contextLines.push(`Documentation URL: ${opts.docUrl.trim()}`)
  if (opts.docTitle?.trim()) contextLines.push(`Page title: ${opts.docTitle.trim()}`)
  if (opts.hint?.trim()) contextLines.push(`Extra instructions from the operator: ${opts.hint.trim()}`)

  const userPrompt =
    `${contextLines.join('\n')}\n\n` +
    `Documentation page text:\n"""\n${docText}\n"""\n\n` +
    `Return the JSON object now.`

  const resp = await fetch('/v1/chat/completions', {
    method: 'POST',
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${opts.apiKey.trim()}`,
    },
    body: JSON.stringify({
      model: opts.model.trim(),
      messages: [
        { role: 'system', content: SYSTEM_PROMPT },
        { role: 'user', content: userPrompt },
      ],
      stream: false,
      // 结构化抽取要稳定复现，温度压到接近 0。
      temperature: 0.1,
    }),
    signal: opts.signal,
  })

  if (!resp.ok) {
    const body = await resp.text().catch(() => '')
    throw new Error(`HTTP ${resp.status}${body ? ': ' + body.slice(0, 300) : ''}`)
  }

  const data = (await resp.json().catch(() => null)) as
    | { choices?: Array<{ message?: { content?: string } }> }
    | null
  const content = data?.choices?.[0]?.message?.content
  if (typeof content !== 'string' || !content.trim()) {
    throw new Error('empty response from model')
  }

  const draft = normalizeDraft(parseJsonLoose(content))
  return { draft, json: JSON.stringify(draft, null, 2) }
}

/**
 * parseJsonLoose：从模型回复里取出 JSON 对象。
 * 依次尝试：直接 parse → 剥掉 ```json 围栏 → 截取第一个 `{` 到最后一个 `}`。
 * 三种都失败才抛错（错误里带上回复片段，便于定位模型是不是在讲废话）。
 */
function parseJsonLoose(raw: string): Record<string, unknown> {
  const attempts: string[] = []
  const text = raw.trim()
  attempts.push(text)

  const fenced = text.replace(/^```[a-zA-Z]*\s*/, '').replace(/```\s*$/, '').trim()
  if (fenced !== text) attempts.push(fenced)

  const start = fenced.indexOf('{')
  const end = fenced.lastIndexOf('}')
  if (start >= 0 && end > start) attempts.push(fenced.slice(start, end + 1))

  for (const candidate of attempts) {
    try {
      const parsed = JSON.parse(candidate)
      if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
        return parsed as Record<string, unknown>
      }
    } catch {
      // 继续尝试下一种。
    }
  }
  throw new Error(`model did not return JSON: ${text.slice(0, 200)}`)
}

/**
 * normalizeDraft：只保留我们认识的键，并把类型不对的值丢掉。
 * 这样即便模型多输出了字段，也不会污染回填逻辑；缺失的键留给管理员自己填。
 */
function normalizeDraft(obj: Record<string, unknown>): ModelIntroDraft {
  const out: ModelIntroDraft = {}
  if (typeof obj.title === 'string') out.title = obj.title.trim()
  if (typeof obj.description === 'string') out.description = obj.description.trim()
  if (typeof obj.description_en === 'string') out.description_en = obj.description_en.trim()
  if (obj.default_params && typeof obj.default_params === 'object' && !Array.isArray(obj.default_params)) {
    out.default_params = obj.default_params as Record<string, unknown>
  }
  if (Array.isArray(obj.output_fields)) out.output_fields = obj.output_fields
  if (typeof obj.result_field === 'string') out.result_field = obj.result_field.trim()
  const rt = typeof obj.result_type === 'string' ? obj.result_type.trim().toLowerCase() : ''
  if (rt === 'image' || rt === 'video') out.result_type = rt
  return out
}
