import { authenticatedMediaUrl } from '../api/client'
import { imageParameterLabel } from '../parameterLabels'
import type { ChatMessage, GenerateVariant, GeneratedImage, GeneratedImagePayload, GenerationSlot, HistoryRecord, ReferenceImageRequest } from '../types'

function references(record: HistoryRecord) {
  const raw = record.request.reference_images
  const items = Array.isArray(raw) ? raw as ReferenceImageRequest[] : []
  return items.flatMap((reference, index) => {
    const original = reference.storage_key
      ? `/plugins/image-generation/api/assets/${record.id}/reference/${index}`
      : reference.data_url || reference.remote_url || ''
    if (!original) return []
    return [{
      id: `${record.id}-reference-${index}`,
      dataUrl: authenticatedMediaUrl(reference.preview_url || original),
      originalDataUrl: authenticatedMediaUrl(original),
      fileName: reference.name || `reference-${index + 1}.png`,
      mimeType: reference.mime_type || 'image/png',
    }]
  })
}

function source(image: GeneratedImagePayload): string {
  return image.url ? authenticatedMediaUrl(image.url) : ''
}

function images(record: HistoryRecord): GeneratedImage[] {
  const resultPrompt = record.result?.revised_prompt || record.prompt
  const variants = Array.isArray(record.request.variants) ? record.request.variants as GenerateVariant[] : []
  return (record.result?.images ?? []).map((image, index) => ({
    id: `${record.id}-image-${index}`,
    historyId: record.id,
    src: authenticatedMediaUrl(image.preview_url || source(image)),
    originalSrc: source(image),
    revisedPrompt: image.revised_prompt || resultPrompt,
    variantLabel: image.variant_label || variants[index]?.label,
    createdAt: new Date(record.updated_at).toLocaleString(),
		lazy: true,
  })).filter(image => image.src)
}

function generationSlots(record: HistoryRecord): GenerationSlot[] {
  const generated = images(record)
  const variants = Array.isArray(record.request.variants) ? record.request.variants as GenerateVariant[] : []
  const outputCount = Number(record.request.output_count) || variants.length || Math.max(generated.length, 1)
  const failedVariants = new Set(record.result?.failed_variants ?? [])
  const unusedImages = [...generated]

  return Array.from({ length: outputCount }, (_, index) => {
    const label = variants[index]?.label
    const matchingIndex = label ? unusedImages.findIndex(image => image.variantLabel === label) : 0
    const image = matchingIndex >= 0 ? unusedImages.splice(matchingIndex, 1)[0] : undefined
    const base = { id: `${record.id}-slot-${index}`, label, progress: 100 }
    if (image) return { ...base, status: 'succeeded' as const, image }
    if (record.status === 'pending') return { ...base, status: 'pending' as const, progress: 1 }
    if (record.status === 'canceled') return { ...base, status: 'canceled' as const, error: record.error_message || '生成已取消' }
    return {
      ...base,
      status: 'failed' as const,
      error: record.error_message || (label && failedVariants.has(label) ? `${label}生成失败` : '图片生成未返回可显示的图片'),
    }
  })
}

export function projectConversationMessages(records: HistoryRecord[]): ChatMessage[] {
  const sorted = [...records].sort((left, right) => Date.parse(left.created_at) - Date.parse(right.created_at))
  const groups = new Map<string, HistoryRecord[]>()
  const entries: HistoryRecord[][] = []
  for (const record of sorted) {
    const groupID = typeof record.request.generation_group_id === 'string' ? record.request.generation_group_id : ''
    if (!groupID) {
      entries.push([record])
      continue
    }
    const existing = groups.get(groupID)
    if (existing) existing.push(record)
    else {
      const group = [record]
      groups.set(groupID, group)
      entries.push(group)
    }
  }
  return entries
    .flatMap((group) => {
      const record = group[0]
      const latest = group.reduce((current, item) => Date.parse(item.updated_at) > Date.parse(current.updated_at) ? item : current, record)
      const totalOutputCount = group.reduce((total, item) => total + (Number(item.request.output_count) || 1), 0)
      const historyTasks = group.map(item => ({ id: item.id, outputCount: Number(item.request.output_count) || 1 }))
      const user: ChatMessage = {
        id: `${record.id}-user`, role: 'user', content: record.prompt,
        createdAt: new Date(record.created_at).toLocaleString(), referenceImages: references(record),
        requestSettings: [{
          modelLabel: String(record.request.model ?? ''),
          sizeLabel: String(record.request.size ?? record.request.aspect_ratio ?? ''),
          countLabel: `数量: ${totalOutputCount}`,
          detailsLabel: historyParameterSummary(record.request),
        }],
        historyIds: group.map(item => item.id),
        historyTasks,
      }
      const generated = group.flatMap(images)
      const slots = group.flatMap(generationSlots)
      const hasPendingSlots = slots.some(slot => slot.status === 'pending')
      if (generated.length) {
        const hasIncompleteSlots = slots.some(slot => slot.status !== 'succeeded')
        return [user, {
          id: `${record.id}-assistant`, role: 'assistant', content: hasPendingSlots ? '正在生成图片，请稍候...' : '生成结果',
          createdAt: new Date(latest.updated_at).toLocaleString(), images: generated,
          generationSlots: hasIncompleteSlots ? slots : undefined,
          status: hasPendingSlots ? 'pending' : undefined,
          historyIds: group.map(item => item.id),
          historyTasks,
        } as ChatMessage]
      }
      if (hasPendingSlots) return [user, {
        id: `${record.id}-assistant`, role: 'assistant', content: '正在生成图片，请稍候...',
        createdAt: new Date(latest.updated_at).toLocaleString(), status: 'pending', generationSlots: slots,
        historyIds: group.map(item => item.id), historyTasks,
      } as ChatMessage]
      if (group.every(item => item.status === 'succeeded')) return [user]
      const status = group.every(item => item.status === 'canceled') ? 'canceled' : 'failed'
      const failed = group.find(item => item.status === 'failed')
      return [user, { id: `${record.id}-assistant`, role: 'assistant', content: failed?.error_message || (status === 'canceled' ? '生成已取消' : '图片生成失败'), createdAt: new Date(latest.updated_at).toLocaleString(), status, historyIds: group.map(item => item.id), historyTasks } as ChatMessage]
    })
}

function historyParameterSummary(request: Record<string, unknown>): string {
  const details = [
    request.aspect_ratio ? `比例: ${request.aspect_ratio}` : '',
    request.resolution ? `分辨率: ${request.resolution}` : '',
    request.quality ? `画质: ${imageParameterLabel('quality', String(request.quality))}` : '',
    request.output_format ? `格式: ${request.output_format}` : '',
    request.output_compression !== undefined ? `压缩: ${request.output_compression}%` : '',
    request.background ? `背景: ${imageParameterLabel('background', String(request.background))}` : '',
    request.input_fidelity ? `保真度: ${imageParameterLabel('input_fidelity', String(request.input_fidelity))}` : '',
  ]
  return details.filter(Boolean).join(' | ')
}
