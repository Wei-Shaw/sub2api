import { readFileSync } from 'node:fs'
import { dirname, resolve } from 'node:path'
import { fileURLToPath } from 'node:url'
import { describe, expect, it } from 'vitest'

const viewSource = readFileSync(
  resolve(dirname(fileURLToPath(import.meta.url)), '../VideoPlaygroundView.vue'),
  'utf8',
)

describe('VideoPlaygroundView output payload fallback', () => {

  it('does not expose task cancellation from the playground', () => {
    expect(viewSource).not.toContain('playground.cancel')
    expect(viewSource).not.toContain('onCancel')
    expect(viewSource).not.toContain('videoModels.playground.btnCancel')
    expect(viewSource).not.toContain('videoModels.playground.confirmClose')
  })

  it('keeps the pricing table entry visible after the model loads', () => {
    expect(viewSource).toContain('v-if="model"\n            type="button"')
    expect(viewSource).toContain("t('videoModels.noPricing')")
    expect(viewSource).not.toContain('v-if="model && model.pricing && model.pricing.length > 0"\n            type="button"')
  })

  it('shows output field examples until a real result payload is available', () => {
    expect(viewSource).toContain(
      'JSON.stringify(buildOutputExamplePayload(outputFields.value), null, 2)',
    )
    expect(viewSource).toContain("t('videoModels.playground.outputValueFromExample')")
    expect(viewSource).toContain("t('videoModels.playground.outputValueFromPayload')")
    expect(viewSource).not.toContain(
      'v-if="playground.resultPayload.value"\n                class="max-h-96',
    )
    expect(viewSource).not.toContain('videoModels.playground.previewFromDefault')
    expect(viewSource).toContain(`v-if="resultType !== 'video'"`)
    expect(viewSource).not.toContain(':alt="primaryPreview.source')
  })

  it('saves only completed payload videos into the current user material library', () => {
    expect(viewSource).toContain("primaryPreview.source === 'payload' && playground.phase.value === 'completed'")
    expect(viewSource).toContain('userMaterialsAPI.importFromUrl(normalized)')
    expect(viewSource).toContain("t('videoModels.playground.saveToMaterials')")
  })

  it('shows left-aligned download and material actions for completed videos', () => {
    expect(viewSource).toContain('class="flex flex-wrap justify-start gap-2"')
    expect(viewSource.indexOf(':href="primaryPreview.url"')).toBeLessThan(
      viewSource.indexOf('@click="saveVideoToMaterials(primaryPreview.url)"'),
    )
    expect(viewSource).toContain(':download="videoDownloadFileName(primaryPreview.url)"')
    expect(viewSource).not.toContain('fetch(normalized)')
    expect(viewSource).toContain('<Icon name="download" size="xs" />')
  })
})
