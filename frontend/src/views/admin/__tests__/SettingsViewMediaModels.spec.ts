import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'

import { describe, expect, it } from 'vitest'

const viewPath = resolve(dirname(fileURLToPath(import.meta.url)), '../SettingsView.vue')
const viewSource = readFileSync(viewPath, 'utf8')

describe('SettingsView custom media model rules', () => {
  it('renders and saves image and video model patterns', () => {
    for (const field of ['custom_image_model_patterns', 'custom_video_model_patterns']) {
      expect(viewSource).toContain(`v-model="form.${field}"`)
      expect(viewSource).toMatch(new RegExp(`${field}:\\s*form\\.${field}`))
    }

    expect(viewSource).toContain('customVideoModelPatternsHint')
  })
})
