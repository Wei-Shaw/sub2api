import createDOMPurify from 'dompurify'
import { marked } from 'marked'

marked.setOptions({
  breaks: true,
  gfm: true,
})

const PUBLIC_CONTENT_ALLOWED_TAGS = [
  'p', 'br', 'strong', 'b', 'em', 'i', 'u', 's',
  'span', 'a', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
  'ul', 'ol', 'li', 'blockquote', 'pre', 'code', 'hr', 'img', 'div',
  'table', 'thead', 'tbody', 'tr', 'th', 'td',
] as const

const PUBLIC_CONTENT_ALLOWED_ATTR = ['href', 'target', 'rel', 'src', 'alt', 'title', 'style', 'class'] as const
const PUBLIC_CONTENT_SAFE_TEXT_ALIGN = new Set(['left', 'center', 'right', 'justify'])
const PUBLIC_CONTENT_SAFE_COLOR_NAMES = new Set([
  'black', 'white', 'red', 'blue', 'green', 'yellow', 'orange', 'purple',
  'gray', 'grey', 'teal', 'pink', 'brown',
])

const rgbColorPattern = /^rgba?\(\s*(\d{1,3}\s*,\s*){2}\d{1,3}(\s*,\s*(0|0?\.\d+|1(\.0+)?)\s*)?\)$/
const hslColorPattern = /^hsla?\(\s*\d{1,3}(\.\d+)?\s*,\s*\d{1,3}%\s*,\s*\d{1,3}%(\s*,\s*(0|0?\.\d+|1(\.0+)?)\s*)?\)$/
const languageClassPattern = /^language-[a-z0-9_-]+$/i

type DOMPurifyLike = {
  sanitize: (dirty: string, config?: Record<string, unknown>) => string
}

let browserPurifier: DOMPurifyLike | null = null

function getBrowserPurifier(): DOMPurifyLike {
  if (browserPurifier) {
    return browserPurifier
  }
  if (typeof window === 'undefined') {
    throw new Error('sanitizePublicHTML requires a browser-like window')
  }
  browserPurifier = createDOMPurify(window)
  return browserPurifier
}

function sanitizeStyle(style: string): string {
  const allowed: string[] = []

  for (const part of style.split(';')) {
    const trimmed = part.trim()
    if (!trimmed) {
      continue
    }
    const [rawKey, rawValue] = trimmed.split(':', 2)
    if (!rawKey || !rawValue) {
      continue
    }
    const key = rawKey.trim().toLowerCase()
    const value = rawValue.trim()
    switch (key) {
      case 'color':
      case 'background-color':
        if (isSafeCSSColor(value)) {
          allowed.push(`${key}: ${value}`)
        }
        break
      case 'text-align':
        if (PUBLIC_CONTENT_SAFE_TEXT_ALIGN.has(value.toLowerCase())) {
          allowed.push(`${key}: ${value.toLowerCase()}`)
        }
        break
      default:
        break
    }
  }

  return allowed.join('; ')
}

function isSafeCSSColor(value: string): boolean {
  const normalized = value.trim().toLowerCase()
  if (!normalized) {
    return false
  }
  if (normalized.startsWith('#')) {
    const hex = normalized.slice(1)
    return (hex.length === 3 || hex.length === 6) && /^[0-9a-f]+$/i.test(hex)
  }
  return rgbColorPattern.test(normalized)
    || hslColorPattern.test(normalized)
    || PUBLIC_CONTENT_SAFE_COLOR_NAMES.has(normalized)
}

function isAllowedHref(raw: string): boolean {
  const trimmed = raw.trim()
  if (!trimmed) {
    return false
  }
  if (trimmed.startsWith('#') || (trimmed.startsWith('/') && !trimmed.startsWith('//'))) {
    return true
  }
  try {
    const parsed = new URL(trimmed)
    return ['http:', 'https:', 'mailto:', 'tel:'].includes(parsed.protocol.toLowerCase())
  } catch {
    return false
  }
}

function isAllowedImageSrc(raw: string): boolean {
  const trimmed = raw.trim()
  if (!trimmed) {
    return false
  }
  if (trimmed.startsWith('/') && !trimmed.startsWith('//')) {
    return true
  }
  try {
    const parsed = new URL(trimmed)
    return ['http:', 'https:'].includes(parsed.protocol.toLowerCase())
  } catch {
    return false
  }
}

function unwrapElement(element: Element): void {
  const parent = element.parentNode
  if (!parent) {
    element.remove()
    return
  }
  while (element.firstChild) {
    parent.insertBefore(element.firstChild, element)
  }
  parent.removeChild(element)
}

function keepOnlyAllowedAttrs(element: HTMLElement, allowed: string[]): void {
  for (const { name } of Array.from(element.attributes)) {
    if (!allowed.includes(name.toLowerCase())) {
      element.removeAttribute(name)
    }
  }
}

function postProcessSanitizedHTML(root: HTMLElement): void {
  const elements = Array.from(root.querySelectorAll<HTMLElement>('*'))
  for (const element of elements) {
    const tag = element.tagName.toLowerCase()
    switch (tag) {
      case 'a': {
        const href = element.getAttribute('href')?.trim() ?? ''
        if (!isAllowedHref(href)) {
          unwrapElement(element)
          break
        }
        const target = element.getAttribute('target')?.trim().toLowerCase()
        if (target !== '_blank' && target !== '_self') {
          element.removeAttribute('target')
        } else {
          element.setAttribute('target', target)
        }
        element.setAttribute('href', href)
        element.setAttribute('rel', 'noopener noreferrer nofollow')
        keepOnlyAllowedAttrs(element, ['href', 'target', 'rel'])
        break
      }
      case 'img': {
        const src = element.getAttribute('src')?.trim() ?? ''
        if (!isAllowedImageSrc(src)) {
          element.remove()
          break
        }
        element.setAttribute('src', src)
        keepOnlyAllowedAttrs(element, ['src', 'alt', 'title'])
        break
      }
      case 'code': {
        const className = element.getAttribute('class')?.trim() ?? ''
        if (!languageClassPattern.test(className)) {
          element.removeAttribute('class')
        } else {
          element.setAttribute('class', className)
        }
        keepOnlyAllowedAttrs(element, ['class'])
        break
      }
      case 'span':
      case 'p':
      case 'div':
      case 'blockquote':
      case 'h1':
      case 'h2':
      case 'h3':
      case 'h4':
      case 'h5':
      case 'h6': {
        const style = element.getAttribute('style')?.trim() ?? ''
        const sanitizedStyle = sanitizeStyle(style)
        if (sanitizedStyle) {
          element.setAttribute('style', sanitizedStyle)
        } else {
          element.removeAttribute('style')
        }
        keepOnlyAllowedAttrs(element, sanitizedStyle ? ['style'] : [])
        break
      }
      default:
        keepOnlyAllowedAttrs(element, [])
        break
    }
  }
}

export function sanitizePublicHTMLWithPurifier(raw: string, purifier: DOMPurifyLike, document: Document): string {
  const sanitized = purifier.sanitize(raw, {
    ALLOWED_TAGS: [...PUBLIC_CONTENT_ALLOWED_TAGS],
    ALLOWED_ATTR: [...PUBLIC_CONTENT_ALLOWED_ATTR],
    ALLOW_DATA_ATTR: false,
    FORBID_TAGS: ['script', 'style', 'iframe', 'object', 'embed', 'svg', 'math', 'form', 'input', 'button', 'video', 'audio', 'source', 'details'],
    FORBID_ATTR: ['srcset'],
  })

  const root = document.createElement('div')
  root.innerHTML = sanitized
  postProcessSanitizedHTML(root)
  return root.innerHTML
}

export function sanitizePublicHTML(raw: string): string {
  return sanitizePublicHTMLWithPurifier(raw, getBrowserPurifier(), document)
}

export function renderPublicMarkdownWithPurifier(markdown: string, purifier: DOMPurifyLike, document: Document): string {
  const html = marked.parse(markdown) as string
  return sanitizePublicHTMLWithPurifier(html, purifier, document)
}

export function renderPublicMarkdown(markdown: string): string {
  return renderPublicMarkdownWithPurifier(markdown, getBrowserPurifier(), document)
}
