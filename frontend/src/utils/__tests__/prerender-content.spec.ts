import { describe, expect, it } from 'vitest'
import { injectPrerenderContent } from '@/utils/prerender'

describe('prerender content injection', () => {
  const baseHTML = '<!doctype html><html><head><title>Test</title></head><body><div id="app"></div></body></html>'

  it('injects legal markdown body into prerendered html', () => {
    const html = injectPrerenderContent(baseHTML, {
      route: '/legal/terms',
      title: 'Terms of Service',
      source: 'legal',
      markdown: '# Terms\n\nThis is the legal body.',
    })

    expect(html).toContain('Terms of Service')
    expect(html).toContain('<h1>Terms</h1>')
    expect(html).toContain('<p>This is the legal body.</p>')
  })

  it('injects custom markdown body into prerendered html', () => {
    const html = injectPrerenderContent(baseHTML, {
      route: '/custom/guide',
      title: 'Guide',
      source: 'custom-markdown',
      markdownSlug: 'guide',
      markdown: '# Guide\n\nCustom page content.',
    })

    expect(html).toContain('Guide')
    expect(html).toContain('<h1>Guide</h1>')
    expect(html).toContain('<p>Custom page content.</p>')
  })

  it('rewrites relative markdown image paths for prerendered public pages', () => {
    const html = injectPrerenderContent(baseHTML, {
      route: '/custom/guide',
      title: 'Guide',
      source: 'custom-markdown',
      markdownSlug: 'guide',
      markdown: '![截图](images/教程截图 中文.png)',
    })

    expect(html).toContain('/api/v1/pages/guide/images/images/%E6%95%99%E7%A8%8B%E6%88%AA%E5%9B%BE%20%E4%B8%AD%E6%96%87.png')
  })

  it('injects tutorial html body into prerendered html', () => {
    const html = injectPrerenderContent(baseHTML, {
      route: '/docs/tutorial',
      title: 'Tutorial Document',
      source: 'tutorial',
      html: '<h2>Tutorial</h2><p>Use prerendered HTML content.</p>',
    })

    expect(html).toContain('Tutorial Document')
    expect(html).toContain('<h2>Tutorial</h2>')
    expect(html).toContain('<p>Use prerendered HTML content.</p>')
  })

  it('escapes title and sanitizes prerendered body html', () => {
    const html = injectPrerenderContent(baseHTML, {
      route: '/custom/unsafe',
      title: '<img src=x onerror=alert(1)>Unsafe',
      source: 'custom-markdown',
      html: '<p>Hello</p><script>alert(1)</script><img src="/ok.png" onerror="alert(1)">',
    })

    expect(html).toContain('&lt;img src=x onerror=alert(1)&gt;Unsafe')
    expect(html).not.toContain('<script>alert(1)</script>')
    expect(html).not.toContain('onerror="alert(1)"')
    expect(html).toContain('<img src="/ok.png">')
  })
})
