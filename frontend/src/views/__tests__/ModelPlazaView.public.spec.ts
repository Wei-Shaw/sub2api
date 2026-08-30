import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../ModelPlazaView.vue')
const source = readFileSync(viewPath, 'utf8')

describe('ModelPlazaView public shell', () => {
  it('uses AppLayout for signed-in users and a public header for visitors', () => {
    expect(source).toContain('<AppLayout v-if="isAuthenticated">')
    expect(source).toContain('<div v-else class="min-h-screen')
    expect(source).toContain('<LocaleSwitcher />')
    expect(source).toContain('to="/login"')
    expect(source.match(/<ModelPlazaContent/g)).toHaveLength(2)
  })
})
