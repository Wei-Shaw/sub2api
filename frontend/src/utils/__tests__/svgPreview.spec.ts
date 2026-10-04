import { describe, expect, it } from 'vitest'
import { createSvgPreviewUrl } from '../svgPreview'

describe('createSvgPreviewUrl', () => {
  it('extracts fenced SVG and preserves drawing details with an SVG namespace', () => {
    const url = createSvgPreviewUrl('```svg\n<svg viewBox="0 0 640 480"><text x="20" y="30">鹈鹕 &amp; bicycle</text><circle cx="100" cy="100" r="40" fill="#ef4444"/></svg>\n```')
    expect(url).toMatch(/^data:image\/svg\+xml;charset=utf-8,/)
    const svg = decodeURIComponent(url!.split(',')[1])
    const document = new DOMParser().parseFromString(svg, 'image/svg+xml')
    expect(document.documentElement.namespaceURI).toBe('http://www.w3.org/2000/svg')
    expect(document.documentElement.getAttribute('viewBox')).toBe('0 0 640 480')
    expect(document.querySelector('text')?.textContent).toBe('鹈鹕 & bicycle')
    expect(document.querySelector('circle')?.getAttribute('fill')).toBe('#ef4444')
  })

  it('removes active content before constructing an image source', () => {
    const url = createSvgPreviewUrl('<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script><foreignObject><div>unsafe</div></foreignObject><rect width="20" height="20"/></svg>')
    expect(url).not.toBeNull()
    const svg = decodeURIComponent(url!.split(',')[1])
    expect(svg).not.toContain('onload')
    expect(svg).not.toContain('<script')
    expect(svg).not.toContain('foreignObject')
    expect(svg).toContain('<rect')
  })

  it('preserves valid nested SVG drawings', () => {
    const url = createSvgPreviewUrl('<svg viewBox="0 0 100 100"><svg x="20" y="20"><circle r="10"/></svg></svg>')
    expect(url).not.toBeNull()
    expect(decodeURIComponent(url!.split(',')[1])).toContain('<circle')
  })

  it.each([
    '',
    'No SVG in this response',
    '<svg><path d="M0 0"/>',
    '<svg><g></svg>',
    '<svg xmlns="https://example.com/not-svg"><rect/></svg>'
  ])('rejects incomplete or invalid SVG: %s', (output) => {
    expect(createSvgPreviewUrl(output)).toBeNull()
  })
})
