import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../VideoPlaygroundView.vue'),
  'utf8',
)

describe('VideoPlaygroundView output payload fallback', () => {
  it('shows output field examples until a real result payload is available', () => {
    expect(viewSource).toContain(
      'JSON.stringify(buildOutputExamplePayload(outputFields.value), null, 2)',
    )
    expect(viewSource).toContain("t('videoModels.playground.outputValueFromExample')")
    expect(viewSource).toContain("t('videoModels.playground.outputValueFromPayload')")
    expect(viewSource).not.toContain(
      'v-if="playground.resultPayload.value"\n                class="max-h-96',
    )
  })

  it('saves only completed payload videos into the current user material library', () => {
    expect(viewSource).toContain("primaryPreview.source === 'payload' && playground.phase.value === 'completed'")
    expect(viewSource).toContain('userMaterialsAPI.importFromUrl(normalized)')
    expect(viewSource).toContain("t('videoModels.playground.saveToMaterials')")
  })
})
