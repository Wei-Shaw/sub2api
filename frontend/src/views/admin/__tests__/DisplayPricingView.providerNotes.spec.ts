import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../DisplayPricingView.vue')
const source = readFileSync(viewPath, 'utf8')

describe('DisplayPricingView provider notes', () => {
  it('edits and submits independent token, per-request, and image notes', () => {
    expect(source).toContain('v-model="providerForm.provider_note"')
    expect(source).toContain('v-model="providerForm.per_request_note"')
    expect(source).toContain('v-model="providerForm.image_note"')
    expect(source).toContain('per_request_note: providerForm.per_request_note.trim()')
    expect(source).toContain('image_note: providerForm.image_note.trim()')
  })
})
