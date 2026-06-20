import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const componentPath = resolve(dirname(fileURLToPath(import.meta.url)), '../AuthLayout.vue')
const source = readFileSync(componentPath, 'utf8')

describe('AuthLayout visual baseline', () => {
  it('does not use mesh backgrounds, decorative orbs, glow shadows, or glass cards', () => {
    expect(source).not.toContain('bg-gradient-to-br from-gray-50 via-primary-50/30')
    expect(source).not.toContain('blur-3xl')
    expect(source).not.toContain('shadow-primary')
    expect(source).not.toContain('card-glass')
    expect(source).not.toContain('text-gradient')
  })

  it('uses the display font for the brand title', () => {
    expect(source).toContain('font-display')
  })
})
