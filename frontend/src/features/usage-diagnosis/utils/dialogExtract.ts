import type { DialogTurn, MediaItem } from '../types'
import { unwrapJson } from './jsonFormat'

let mediaSeq = 0
function nextId(prefix: string) {
  mediaSeq += 1
  return `${prefix}-${mediaSeq}`
}

function isDataUrl(s: string) {
  return /^data:/i.test(s)
}

function isImageMime(mime?: string) {
  return !!mime && mime.toLowerCase().startsWith('image/')
}

function pushUnique(list: MediaItem[], item: MediaItem) {
  const key = item.dataUrl || item.url || item.id
  if (list.some((x) => (x.dataUrl || x.url || x.id) === key)) return
  list.push(item)
}

function extractFromPart(part: any, images: MediaItem[], files: MediaItem[]) {
  if (!part || typeof part !== 'object') return
  const type = String(part.type || part.kind || '').toLowerCase()
  const mime = part.mime_type || part.media_type || part.mime || undefined

  // images
  if (['image_url', 'input_image', 'output_image', 'image_file', 'image', 'image_generation', 'image_generation_call'].includes(type) || isImageMime(mime)) {
    let url: string | undefined
    let dataUrl: string | undefined
    const iu = part.image_url
    if (typeof iu === 'string') url = iu
    else if (iu && typeof iu === 'object' && typeof iu.url === 'string') url = iu.url
    if (typeof part.url === 'string') url = part.url
    if (typeof part.b64_json === 'string') dataUrl = `data:${mime || 'image/png'};base64,${part.b64_json}`
    if (typeof part.source === 'string' && isDataUrl(part.source)) dataUrl = part.source
    if (url && isDataUrl(url)) {
      dataUrl = url
      url = undefined
    }
    pushUnique(images, {
      id: nextId('img'),
      name: part.filename || part.name || 'image',
      mime: mime || (dataUrl?.match(/^data:([^;]+)/)?.[1]),
      url,
      dataUrl,
      kind: 'image'
    })
    return
  }

  // files
  if (['file', 'input_file', 'document'].includes(type) || part.file) {
    const f = part.file && typeof part.file === 'object' ? part.file : part
    const name = f.filename || f.name || f.file_id || f.id || 'file'
    let dataUrl: string | undefined
    let url: string | undefined
    if (typeof f.file_data === 'string') {
      dataUrl = isDataUrl(f.file_data) ? f.file_data : `data:${f.mime_type || 'application/octet-stream'};base64,${f.file_data}`
    }
    if (typeof f.url === 'string') url = f.url
    if (f.source?.data) {
      dataUrl = `data:${f.source.media_type || f.mime_type || 'application/octet-stream'};base64,${f.source.data}`
    }
    const kind = isImageMime(f.mime_type || f.media_type) ? 'image' : 'file'
    const item: MediaItem = {
      id: nextId(kind === 'image' ? 'img' : 'file'),
      name: String(name),
      mime: f.mime_type || f.media_type || (f.file_id ? `id ${f.file_id}` : 'file'),
      url,
      dataUrl,
      kind
    }
    pushUnique(kind === 'image' ? images : files, item)
  }
}

function textFromContent(content: unknown): string {
  if (typeof content === 'string') return content
  if (Array.isArray(content)) {
    return content
      .map((p) => {
        if (typeof p === 'string') return p
        if (p && typeof p === 'object') {
          if (typeof (p as any).text === 'string') return (p as any).text
          if (typeof (p as any).content === 'string') return (p as any).content
        }
        return ''
      })
      .filter(Boolean)
      .join('\n')
  }
  if (content && typeof content === 'object' && typeof (content as any).text === 'string') {
    return (content as any).text
  }
  return ''
}

function collectMediaFromContent(content: unknown, images: MediaItem[], files: MediaItem[]) {
  if (!Array.isArray(content)) return
  for (const part of content) extractFromPart(part, images, files)
}

function roleLabel(role: string): DialogTurn['role'] {
  const r = role.toLowerCase()
  if (r === 'system' || r === 'developer') return 'system'
  if (r === 'user') return 'user'
  if (r === 'assistant') return 'assistant'
  if (r === 'thinking' || r === 'reason') return 'thinking'
  return role
}

/** Extract dialog turns from request/response bodies. */
export function extractDialogTurns(reqBody?: string, resBody?: string, dialogFallback?: unknown): DialogTurn[] {
  const turns: DialogTurn[] = []
  const req = safeObj(reqBody)
  if (req) {
    if (typeof req.instructions === 'string' && req.instructions.trim()) {
      turns.push({ role: 'system', text: req.instructions, images: [], files: [] })
    }
    if (Array.isArray(req.messages)) {
      for (const m of req.messages) {
        if (!m || typeof m !== 'object') continue
        const images: MediaItem[] = []
        const files: MediaItem[] = []
        collectMediaFromContent((m as any).content, images, files)
        turns.push({
          role: roleLabel(String((m as any).role || 'user')),
          text: textFromContent((m as any).content),
          images,
          files
        })
      }
    } else if (typeof req.prompt === 'string') {
      turns.push({ role: 'user', text: req.prompt, images: [], files: [] })
    } else if (typeof req.input === 'string') {
      turns.push({ role: 'user', text: req.input, images: [], files: [] })
    } else if (Array.isArray(req.input)) {
      for (const item of req.input) {
        if (!item || typeof item !== 'object') continue
        const t = String((item as any).type || '')
        if (t === 'function_call' || t === 'reasoning') continue
        const images: MediaItem[] = []
        const files: MediaItem[] = []
        if (Array.isArray((item as any).content)) collectMediaFromContent((item as any).content, images, files)
        turns.push({
          role: roleLabel(String((item as any).role || 'user')),
          text: textFromContent((item as any).content) || String((item as any).text || ''),
          images,
          files
        })
      }
    }
  }

  const res = safeObj(resBody)
  if (res) {
    const images: MediaItem[] = []
    const files: MediaItem[] = []
    // top-level images
    for (const key of ['images', 'data', 'output']) {
      const arr = (res as any)[key]
      if (Array.isArray(arr)) {
        for (const it of arr) {
          if (it && typeof it === 'object') extractFromPart(it, images, files)
        }
      }
    }
    if (Array.isArray((res as any).choices)) {
      for (const ch of (res as any).choices) {
        const msg = ch?.message || {}
        const thinking = msg.reasoning_content || msg.reasoning || ch?.reasoning_content
        if (typeof thinking === 'string' && thinking.trim()) {
          turns.push({ role: 'thinking', text: thinking, images: [], files: [] })
        }
        const text = textFromContent(msg.content) || (typeof ch?.text === 'string' ? ch.text : '')
        const imgs: MediaItem[] = [...images]
        const fils: MediaItem[] = [...files]
        collectMediaFromContent(msg.content, imgs, fils)
        if (text || imgs.length || fils.length) {
          turns.push({ role: 'assistant', text, images: imgs, files: fils })
        }
      }
    } else if (typeof (res as any).content === 'string') {
      turns.push({ role: 'assistant', text: (res as any).content, images, files })
    } else if (Array.isArray((res as any).output)) {
      for (const item of (res as any).output) {
        if (!item || typeof item !== 'object') continue
        const t = String((item as any).type || '')
        if (t.includes('reason')) {
          turns.push({
            role: 'thinking',
            text: textFromContent((item as any).content) || String((item as any).text || ''),
            images: [],
            files: []
          })
          continue
        }
        const imgs: MediaItem[] = []
        const fils: MediaItem[] = []
        collectMediaFromContent((item as any).content, imgs, fils)
        extractFromPart(item, imgs, fils)
        turns.push({
          role: 'assistant',
          text: textFromContent((item as any).content) || String((item as any).text || ''),
          images: imgs,
          files: fils
        })
      }
    }

    // markdown / img in assistant text already handled via cards above
  }

  if (!turns.length && dialogFallback) {
    const fb = unwrapJson(dialogFallback)
    if (Array.isArray(fb)) {
      for (const t of fb) {
        if (!t || typeof t !== 'object') continue
        turns.push({
          role: roleLabel(String((t as any).role || 'user')),
          text: String((t as any).text || (t as any).content || ''),
          images: [],
          files: []
        })
      }
    }
  }
  return turns
}

function safeObj(raw?: string) {
  if (!raw || !raw.trim()) return null
  try {
    const v = unwrapJson(JSON.parse(raw))
    return v && typeof v === 'object' ? (v as Record<string, any>) : null
  } catch {
    return null
  }
}

export function extractImagesFromMarkdown(text: string): MediaItem[] {
  const out: MediaItem[] = []
  const md = /!\[([^\]]*)\]\(([^)]+)\)/g
  let m: RegExpExecArray | null
  while ((m = md.exec(text))) {
    const url = m[2]
    out.push({
      id: nextId('img'),
      name: m[1] || 'image',
      url: isDataUrl(url) ? undefined : url,
      dataUrl: isDataUrl(url) ? url : undefined,
      kind: 'image'
    })
  }
  const imgTag = /<img[^>]+src=["']([^"']+)["']/gi
  while ((m = imgTag.exec(text))) {
    const url = m[1]
    out.push({
      id: nextId('img'),
      name: 'image',
      url: isDataUrl(url) ? undefined : url,
      dataUrl: isDataUrl(url) ? url : undefined,
      kind: 'image'
    })
  }
  return out
}
