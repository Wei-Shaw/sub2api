import DOMPurify from 'dompurify'

// Render generated SVGs in image isolation, never as executable page markup.
export function qualitySvgDataUrl(text: string): string {
  const match = text.match(/<svg\b[\s\S]*?<\/svg\s*>/i)
  if (!match) return ''
  const safe = DOMPurify.sanitize(match[0], {
    USE_PROFILES: { svg: true, svgFilters: true },
    FORBID_TAGS: ['script', 'foreignObject', 'style', 'image', 'use', 'a', 'animate', 'set', 'animateTransform', 'animateMotion'],
    FORBID_ATTR: ['href', 'xlink:href', 'style'],
  })
  const doc = new DOMParser().parseFromString(safe, 'image/svg+xml')
  if (doc.querySelector('parsererror') || doc.documentElement.localName !== 'svg') return ''
  doc.documentElement.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  const xml = new XMLSerializer().serializeToString(doc.documentElement)
  return 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(xml)
}
