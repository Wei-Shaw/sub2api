import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelCatalogView from '../ModelCatalogView.vue'

const {
  createModelCatalogIdempotencyKey,
  getModelCatalog,
  showError,
  showSuccess,
  syncModelCatalog,
  bulkUpdateModelCatalogState
} = vi.hoisted(() => ({
  createModelCatalogIdempotencyKey: vi.fn((operation: string) => `model-catalog-${operation}-test-key`),
  getModelCatalog: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  syncModelCatalog: vi.fn(),
  bulkUpdateModelCatalogState: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    modelCatalog: {
      createModelCatalogIdempotencyKey,
      getModelCatalog,
      syncModelCatalog,
      bulkUpdateModelCatalogState,
      updateModelCatalogState: vi.fn(),
      updateModelCatalogPricing: vi.fn()
    }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError,
    showSuccess
  })
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string) => key
    })
  }
})

function mountView() {
  return mount(ModelCatalogView, {
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        Icon: true,
        ToggleSwitch: true
      }
    }
  })
}

describe('admin ModelCatalogView authorization flow', () => {
  beforeEach(() => {
    createModelCatalogIdempotencyKey.mockClear()
    getModelCatalog.mockReset()
    showError.mockReset()
    showSuccess.mockReset()
    syncModelCatalog.mockReset()
    bulkUpdateModelCatalogState.mockReset()
    getModelCatalog.mockResolvedValue({
      initialized: false,
      models: [],
      legacy_model_count: 215
    })
  })

  it('syncs directly with the authenticated admin session without a TOTP dialog', async () => {
    syncModelCatalog.mockResolvedValue({
      initialized: true,
      models: [],
      legacy_model_count: 215
    })

    const wrapper = mountView()
    await flushPromises()
    const syncButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.modelCatalog.sync')
    )
    expect(syncButton).toBeDefined()

    await syncButton!.trigger('click')
    await flushPromises()

    expect(createModelCatalogIdempotencyKey).toHaveBeenCalledWith('sync')
    expect(syncModelCatalog).toHaveBeenCalledWith('model-catalog-sync-test-key')
    expect(showSuccess).toHaveBeenCalledWith('admin.modelCatalog.syncSuccess')
    expect(showError).not.toHaveBeenCalled()
    expect(wrapper.findComponent({ name: 'TotpStepUpDialog' }).exists()).toBe(false)
  })

  it('disables selected models through one bulk mutation with per-model versions', async () => {
    const models = [1, 2].map((id) => ({
      id,
      canonical_key: `model-${id}`,
      operator_state: 'enabled',
      operator_reason: '',
      operator_version: id,
      source_state: 'present',
      provider: 'openai',
      platform: 'openai',
      mode: 'chat',
      pricing_schema_version: 1,
      pricing: null,
      pricing_valid: true,
      pricing_source: 'test',
      source_hash: `hash-${id}`
    }))
    const snapshot = { initialized: true, epoch: 9, revision_id: 901, revision: 9, models, legacy_model_count: 2 }
    getModelCatalog.mockResolvedValue(snapshot)
    bulkUpdateModelCatalogState.mockResolvedValue({
      snapshot: { ...snapshot, models: models.map((model) => ({ ...model, operator_state: 'disabled' })) },
      runtime_reloaded: true
    })

    const wrapper = mountView()
    await flushPromises()
    expect(wrapper.findAll('input[type="checkbox"]')).toHaveLength(5)
    await wrapper.find('[data-test="select-model-1"]').setValue(true)
    await wrapper.find('[data-test="select-model-2"]').setValue(true)

    const bulkButton = wrapper.find('[data-test="bulk-disable-models"]')
    expect(bulkButton.exists()).toBe(true)
    await bulkButton.trigger('click')
    const reason = wrapper.find('textarea')
    await reason.setValue('provider maintenance')
    await wrapper.find('[data-test="confirm-model-catalog-state"]').trigger('click')
    await flushPromises()

    expect(bulkUpdateModelCatalogState).toHaveBeenCalledWith({
      expected_epoch: 9,
      expected_revision_id: 901,
      state: 'disabled',
      models: [
        { model_id: 1, expected_version: 1 },
        { model_id: 2, expected_version: 2 }
      ],
      reason: 'provider maintenance'
    }, 'model-catalog-bulk-state-test-key')
    expect(showSuccess).toHaveBeenCalledWith('admin.modelCatalog.bulkStateSaved')
  })
})
