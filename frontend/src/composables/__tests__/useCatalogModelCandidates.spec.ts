import { beforeEach, describe, expect, it, vi } from 'vitest'

const { getModelsListCandidates } = vi.hoisted(() => ({
  getModelsListCandidates: vi.fn()
}))

vi.mock('@/api/admin/groups', () => ({
  getModelsListCandidates
}))

import {
  fetchCatalogModelCandidates,
  filterCatalogModelSelection,
  mergeModelCandidateOptions
} from '@/composables/useCatalogModelCandidates'

describe('useCatalogModelCandidates', () => {
  beforeEach(() => {
    getModelsListCandidates.mockReset()
  })

  it('loads the shared group candidates for normalized account platforms', async () => {
    getModelsListCandidates
      .mockResolvedValueOnce(['claude-opus-5', 'claude-sonnet-5'])
      .mockResolvedValueOnce(['gpt-5.6', 'claude-opus-5'])

    const models = await fetchCatalogModelCandidates(['claude', 'openai', 'claude'])

    expect(getModelsListCandidates).toHaveBeenNthCalledWith(1, 0, 'anthropic')
    expect(getModelsListCandidates).toHaveBeenNthCalledWith(2, 0, 'openai')
    expect(models).toEqual(['claude-opus-5', 'claude-sonnet-5', 'gpt-5.6'])
  })

  it('uses catalog candidates without preserving selected custom models', () => {
    const options = mergeModelCandidateOptions(['claude-opus-5'])

    expect(options).toEqual([
      { value: 'claude-opus-5', label: 'claude-opus-5' }
    ])
  })

  it('removes legacy and custom selections that are absent from the catalog', () => {
    const selection = filterCatalogModelSelection(
      ['customer-custom-model', 'claude-opus-5', 'legacy-default-model'],
      ['claude-opus-5', 'claude-sonnet-5']
    )

    expect(selection).toEqual(['claude-opus-5'])
  })

  it('returns no options while the catalog is unavailable', () => {
    const options = mergeModelCandidateOptions(null)

    expect(options).toEqual([])
  })
})
