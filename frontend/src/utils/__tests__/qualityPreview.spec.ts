import { describe, expect, it } from 'vitest'
import { qualitySvgDataUrl } from '../qualityPreview'

describe('generated SVG preview isolation', () => {
  it('keeps vector geometry while removing executable and remote content', () => {
    const data = qualitySvgDataUrl('<svg xmlns="http://www.w3.org/2000/svg" onload="alert(1)"><script>alert(1)</script><foreignObject><div>unsafe</div></foreignObject><image href="https://example.com/tracker"/><a href="javascript:alert(1)">link</a><circle r="20"/><path d="M0 0L10 10"/></svg>')
    expect(data.startsWith('data:image/svg+xml;')).toBe(true)
    const xml = decodeURIComponent(data.split(',')[1])
    expect(xml).toContain('<circle')
    expect(xml).toContain('<path')
    expect(xml).not.toMatch(/onload|script|foreignObject|https:|javascript:|<image/)
  })
  it('rejects text and incomplete SVG output', () => {
    expect(qualitySvgDataUrl('no drawing')).toBe('')
    expect(qualitySvgDataUrl('<svg><circle/>')).toBe('')
  })
})
