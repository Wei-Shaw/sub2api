/**
 * Video Playground Parameter Schema
 *
 * 为演练台的动态参数表单提供 3 个变体（text-to-video / image-to-video /
 * reference-to-video）的通用字段定义。字段来源：fal 官方 API 文档中
 * seedance 系列共享的 schema（prompt / image_url / reference_image_urls /
 * duration / resolution / aspect_ratio / seed）。
 *
 * 该 schema 只做最常用字段兜底展示；用户如需高级字段（camera_control 等），
 * 可切换到"原始 JSON 模式"直接编辑请求体。
 */

export type VideoVariant = 'text-to-video' | 'image-to-video' | 'reference-to-video'

export interface VideoParamField {
  key: string
  type: 'string' | 'text' | 'number' | 'select' | 'url' | 'url-list'
  label: string
  required?: boolean
  placeholder?: string
  options?: Array<{ value: string | number; label: string }>
  defaultValue?: string | number
  helper?: string
}

// 通用字段：所有 seedance 视频端点都支持
const commonFields: VideoParamField[] = [
  {
    key: 'prompt',
    type: 'text',
    label: 'Prompt',
    required: true,
    placeholder: 'Describe the video you want to generate...',
  },
  {
    key: 'resolution',
    type: 'select',
    label: 'Resolution',
    options: [
      { value: '480p', label: '480p' },
      { value: '720p', label: '720p' },
      { value: '1080p', label: '1080p' },
    ],
    defaultValue: '720p',
  },
  {
    key: 'duration',
    type: 'select',
    label: 'Duration (seconds)',
    options: [
      { value: '3', label: '3s' },
      { value: '5', label: '5s' },
      { value: '10', label: '10s' },
    ],
    defaultValue: '5',
  },
  {
    key: 'aspect_ratio',
    type: 'select',
    label: 'Aspect Ratio',
    options: [
      { value: '16:9', label: '16:9 (landscape)' },
      { value: '9:16', label: '9:16 (portrait)' },
      { value: '1:1', label: '1:1 (square)' },
    ],
    defaultValue: '16:9',
  },
  {
    key: 'seed',
    type: 'number',
    label: 'Seed (optional)',
    placeholder: 'Random if empty',
    helper: 'Deterministic seed for reproducibility',
  },
]

// 变体特定字段
const variantFields: Record<VideoVariant, VideoParamField[]> = {
  'text-to-video': [],
  'image-to-video': [
    {
      key: 'image_url',
      type: 'url',
      label: 'Image URL',
      required: true,
      placeholder: 'https://... (jpg/png publicly accessible)',
      helper: 'The image to animate. Must be a publicly accessible URL.',
    },
  ],
  'reference-to-video': [
    {
      key: 'reference_image_urls',
      type: 'url-list',
      label: 'Reference Image URLs',
      required: true,
      placeholder: 'One URL per line',
      helper: 'Reference images (subject/style anchors). One URL per line.',
    },
  ],
}

/**
 * 根据变体返回该变体应展示的字段列表。变体字段插在 prompt 之后。
 */
export function fieldsForVariant(variant: VideoVariant): VideoParamField[] {
  const promptField = commonFields[0]
  const restCommon = commonFields.slice(1)
  const specific = variantFields[variant] ?? []
  return [promptField, ...specific, ...restCommon]
}

/**
 * 变体推断：从 fal slug 尾部提取。
 * 例：fal-ai/bytedance/seedance-2.5/text-to-video → text-to-video
 */
export function variantFromSlug(slug: string): VideoVariant {
  if (slug.endsWith('/image-to-video')) return 'image-to-video'
  if (slug.endsWith('/reference-to-video')) return 'reference-to-video'
  return 'text-to-video'
}

/**
 * 表单值 → fal 原生请求体。会剔除空值、把 duration/seed 转成 number，
 * 把 reference_image_urls 从多行字符串转成 string[]。
 */
export function buildRequestBody(
  variant: VideoVariant,
  form: Record<string, string>
): Record<string, unknown> {
  const body: Record<string, unknown> = {}
  const fields = fieldsForVariant(variant)
  for (const f of fields) {
    const raw = (form[f.key] ?? '').trim()
    if (!raw) continue
    if (f.type === 'number') {
      const n = Number(raw)
      if (Number.isFinite(n)) body[f.key] = n
    } else if (f.type === 'url-list') {
      const urls = raw
        .split(/\r?\n/)
        .map((s) => s.trim())
        .filter(Boolean)
      if (urls.length) body[f.key] = urls
    } else {
      body[f.key] = raw
    }
  }
  return body
}
