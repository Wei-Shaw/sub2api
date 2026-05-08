import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

describe('Vite dev proxy', () => {
  it('does not proxy the image generation page route in development', () => {
    const configSource = readFileSync(resolve(process.cwd(), 'vite.config.ts'), 'utf8')

    expect(configSource).not.toContain("'/images'")
    expect(configSource).toContain("'/api'")
    expect(configSource).toContain('target: backendUrl')
    expect(configSource).toContain('proxyTimeout: 0')
    expect(configSource).toContain('timeout: 0')
  })
})
