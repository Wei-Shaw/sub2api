import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../SettingsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('SettingsView feature management source contract', () => {
  it('renders and submits exactly one image generation feature switch', () => {
    expect(viewSource.match(/admin\.settings\.features\.imageGeneration\.title/g)).toHaveLength(1)
    expect(viewSource.match(/v-model="form\.image_generation_enabled"/g)).toHaveLength(1)
    expect(viewSource.match(/image_generation_enabled: form\.image_generation_enabled/g)).toHaveLength(1)
  })
})
