import { describe, expect, it } from 'vitest'
import { buildPrerenderManifest } from '@/utils/prerender'

describe('prerender manifest', () => {
  it('captures route metadata for generated entries', () => {
    const manifest = buildPrerenderManifest([
      {
        route: '/legal/terms',
        title: 'Terms of Service',
        source: 'legal',
        markdown: '# Terms\n\nContent',
      },
      {
        route: '/custom/guide',
        title: 'Guide',
        source: 'custom-markdown',
        markdownSlug: 'guide',
        markdown: '# Guide\n\nBody',
      },
    ])

    expect(manifest.total_routes).toBe(2)
    expect(manifest.routes[0]).toMatchObject({
      route: '/legal/terms',
      source: 'legal',
      has_markdown: true,
      output: 'legal/terms/index.html',
    })
    expect(manifest.routes[1]).toMatchObject({
      route: '/custom/guide',
      source: 'custom-markdown',
      has_markdown: true,
      markdown_slug: 'guide',
      output: 'custom/guide/index.html',
    })
  })
})
