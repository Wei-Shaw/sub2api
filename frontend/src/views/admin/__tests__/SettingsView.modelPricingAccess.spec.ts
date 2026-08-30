import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../SettingsView.vue')
const source = readFileSync(viewPath, 'utf8')

describe('SettingsView model pricing access', () => {
  it('binds and saves the real model_plaza_require_auth value', () => {
    expect(source).toContain('v-model="form.model_plaza_require_auth"')
    expect(source).toContain('data-testid="model-plaza-require-auth-toggle"')
    expect(source).toContain('model_plaza_require_auth: form.model_plaza_require_auth')
  })
})
