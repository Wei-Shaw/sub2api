import { readFileSync } from 'node:fs'
import { resolve } from 'node:path'

import { describe, expect, it } from 'vitest'

describe('admin settings openai global pool wiring', () => {
  it('exposes the new field in the admin settings API types', () => {
    const source = readFileSync(resolve(__dirname, '../../api/admin/settings.ts'), 'utf8')
    expect(source).toContain('openai_global_pool_for_ungrouped_keys')
  })

  it('renders and submits the new field in SettingsView', () => {
    const source = readFileSync(resolve(__dirname, './SettingsView.vue'), 'utf8')
    expect(source).toContain('<Toggle v-model="form.allow_ungrouped_key_scheduling" />')
    expect(source).toContain('<Toggle v-model="form.openai_global_pool_for_ungrouped_keys" />')
    expect(source).toContain('openai_global_pool_for_ungrouped_keys: false')
    expect(source).toContain('openai_global_pool_for_ungrouped_keys: form.openai_global_pool_for_ungrouped_keys')
  })
})
