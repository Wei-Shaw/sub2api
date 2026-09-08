import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'
import { describe, expect, it } from 'vitest'

const sourcePath = resolve(process.cwd(), 'src/views/user/CustomPageView.vue')

describe('CustomPageView iframe embedding', () => {
  it('allows embedded plugin pages to use clipboard, local file, and fullscreen capabilities', () => {
    const source = readFileSync(sourcePath, 'utf8')

    expect(source).toContain('allow="clipboard-read; clipboard-write; file-system; fullscreen"')
    expect(source).toContain('allowfullscreen')
  })
})
