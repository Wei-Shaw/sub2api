import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'

import ModelCatalogView from '../ModelCatalogView.vue'

const {
  createModelCatalogIdempotencyKey,
  getModelCatalog,
  showError,
  showSuccess,
  syncModelCatalog
} = vi.hoisted(() => ({
  createModelCatalogIdempotencyKey: vi.fn(() => 'model-catalog-sync-test-key'),
  getModelCatalog: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  syncModelCatalog: vi.fn()
}))

vi.mock('@/api', () => ({
  adminAPI: {
    modelCatalog: {
      createModelCatalogIdempotencyKey,
      getModelCatalog,
      syncModelCatalog,
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
        BaseDialog: true,
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
})
