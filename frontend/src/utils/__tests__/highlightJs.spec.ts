import { describe, expect, it } from 'vitest'
import { highlightJavaScript } from '../highlightJs'

describe('highlightJavaScript', () => {
  it('highlights keywords and strings', () => {
    const html = highlightJavaScript('function foo() { return "bar"; }')
    expect(html).toContain('tok-keyword')
    expect(html).toContain('function')
    expect(html).toContain('tok-string')
    expect(html).toContain('&quot;bar&quot;')
    expect(html).toContain('tok-function')
  })

  it('escapes HTML in source', () => {
    const html = highlightJavaScript('const x = "<script>"')
    expect(html).not.toContain('<script>')
    expect(html).toContain('&lt;script&gt;')
  })

  it('highlights comments', () => {
    const html = highlightJavaScript('// hello\nconst a = 1')
    expect(html).toContain('tok-comment')
    expect(html).toContain('tok-number')
  })
})
