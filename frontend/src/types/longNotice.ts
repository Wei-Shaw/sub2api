export type LongNoticeTone = 'blue' | 'green' | 'amber' | 'red' | 'gray'
export type LongNoticeBlockType = 'status' | 'heading' | 'text' | 'metrics' | 'feature' | 'callout' | 'buttons'

export interface LongNoticeMetric {
  label: string
  value: string
}

export interface LongNoticeButton {
  label: string
  url: string
}

export interface LongNoticeBlock {
  id: string
  type: LongNoticeBlockType
  tone: LongNoticeTone
  title?: string
  content?: string
  number?: string
  tags?: string[]
  highlight?: string
  metrics?: LongNoticeMetric[]
  buttons?: LongNoticeButton[]
}

export interface LongNoticeDocument {
  version: 1
  blocks: LongNoticeBlock[]
}

export const emptyLongNoticeDocument = (): LongNoticeDocument => ({ version: 1, blocks: [] })

export function parseLongNoticeDocument(raw?: string): LongNoticeDocument {
  if (!raw) return emptyLongNoticeDocument()
  try {
    const parsed = JSON.parse(raw) as Partial<LongNoticeDocument>
    if (parsed.version !== 1 || !Array.isArray(parsed.blocks)) return emptyLongNoticeDocument()
    return { version: 1, blocks: parsed.blocks.slice(0, 30) as LongNoticeBlock[] }
  } catch {
    return emptyLongNoticeDocument()
  }
}
