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
})
