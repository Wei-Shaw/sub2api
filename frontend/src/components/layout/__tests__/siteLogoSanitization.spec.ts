import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const dir = dirname(fileURLToPath(import.meta.url))
const brandingSource = readFileSync(resolve(dir, '../../../utils/branding.ts'), 'utf8')
const plazaNavSource = readFileSync(resolve(dir, '../../modelPlaza/PlazaNavBar.vue'), 'utf8')

describe('site_logo sanitization', () => {
  it('favicon sanitizes siteLogo before applying it', () => {
    expect(brandingSource).toContain("import { sanitizeUrl } from '@/utils/url'")
    expect(brandingSource).toContain('sanitizeUrl(logoUrl')
  })

  it('model plaza sanitizes siteLogo before rendering it', () => {
    expect(plazaNavSource).toContain("import { sanitizeUrl } from '@/utils/url'")
    expect(plazaNavSource).toContain("sanitizeUrl(settings.value?.site_logo")
  })

  it('all renderers allow safe relative and image data URLs', () => {
    for (const src of [brandingSource, plazaNavSource]) {
      expect(src).toContain('allowRelative: true')
      expect(src).toContain('allowDataUrl: true')
    }
  })
})
