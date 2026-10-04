import { sanitizeSvg } from './sanitize'

/** Extract a complete SVG from model output and render it in an isolated image. */
export function createSvgPreviewUrl(output: string): string | null {
  // Models sometimes wrap SVG in Markdown; parse the extracted document as XML.
  const svg = output.match(/<svg(?:\s|>)[\s\S]*<\/svg\s*>/i)?.[0]
  if (!svg) return null

  const parser = new DOMParser()
  const document = parser.parseFromString(svg, 'image/svg+xml')
  const root = document.documentElement
  if (document.querySelector('parsererror') || root.localName !== 'svg') return null
  if (root.namespaceURI && root.namespaceURI !== 'http://www.w3.org/2000/svg') return null

  if (!root.namespaceURI) root.setAttribute('xmlns', 'http://www.w3.org/2000/svg')
  const sanitized = sanitizeSvg(new XMLSerializer().serializeToString(root))
  const sanitizedDocument = parser.parseFromString(sanitized, 'image/svg+xml')
  if (sanitizedDocument.querySelector('parsererror') || sanitizedDocument.documentElement.localName !== 'svg') {
    return null
  }

  return `data:image/svg+xml;charset=utf-8,${encodeURIComponent(sanitized)}`
}
