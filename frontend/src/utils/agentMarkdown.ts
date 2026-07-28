import DOMPurify from 'dompurify'
import { marked } from 'marked'

const allowedTags = [
  'a', 'blockquote', 'br', 'code', 'del', 'em', 'h1', 'h2', 'h3', 'h4', 'h5', 'h6',
  'hr', 'li', 'ol', 'p', 'pre', 'strong', 'table', 'tbody', 'td', 'th', 'thead', 'tr', 'ul'
]

const cache = new Map<string, string>()
const maxCacheEntries = 120

export function renderAgentMarkdown(content: string): string {
  const cached = cache.get(content)
  if (cached !== undefined) return cached

  const parsed = marked.parse(content, {
    async: false,
    breaks: true,
    gfm: true
  }) as string
  const sanitized = DOMPurify.sanitize(parsed, {
    ALLOWED_TAGS: allowedTags,
    ALLOWED_ATTR: ['href', 'title'],
    ALLOW_DATA_ATTR: false
  })

  cache.set(content, sanitized)
  if (cache.size > maxCacheEntries) {
    const oldest = cache.keys().next().value
    if (oldest !== undefined) cache.delete(oldest)
  }
  return sanitized
}
