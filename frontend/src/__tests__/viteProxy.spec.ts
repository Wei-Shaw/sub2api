import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

describe('Vite dev proxy', () => {
  it('proxies image gateway endpoints to the backend in development', () => {
    const configSource = readFileSync(resolve(process.cwd(), 'vite.config.ts'), 'utf8')

    expect(configSource).toContain("'/images'")
    expect(configSource).toContain('target: backendUrl')
    expect(configSource).toContain('proxyTimeout: 0')
    expect(configSource).toContain('timeout: 0')
  })
})
