import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'

const { previewFromCrs, syncFromCrs } = vi.hoisted(() => ({
  previewFromCrs: vi.fn(),
  syncFromCrs: vi.fn(),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { previewFromCrs, syncFromCrs },
  },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }),
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({ t: (key: string) => key }),
}))

import SyncFromCrsModal from '../SyncFromCrsModal.vue'

const BaseDialogStub = defineComponent({
  props: { show: Boolean },
  template: '<div v-if="show"><slot /><slot name="footer" /></div>',
})

describe('SyncFromCrsModal Codex identity policy', () => {
  beforeEach(() => {
    previewFromCrs.mockReset().mockResolvedValue({
      existing_accounts: [],
      new_accounts: [{
        crs_account_id: 'openai-1',
        kind: 'account',
        name: 'OpenAI OAuth',
        platform: 'openai',
        type: 'oauth',
      }],
    })
    syncFromCrs.mockReset().mockResolvedValue({
      created: 1,
      updated: 0,
      skipped: 0,
      failed: 0,
      items: [],
    })
  })

  it('includes the reviewed policy when selected CRS rows contain OpenAI OAuth accounts', async () => {
    const wrapper = mount(SyncFromCrsModal, {
      props: { show: true, proxies: [] },
      global: {
        stubs: {
          BaseDialog: BaseDialogStub,
          Icon: true,
          Select: true,
        },
      },
    })

    const inputs = wrapper.findAll('form input')
    await inputs[0]?.setValue('https://crs.example.com')
    await inputs[1]?.setValue('admin')
    await inputs[2]?.setValue('secret')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.find('[data-testid="codex-identity-policy-toggle"]').exists()).toBe(true)
    const syncButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.accounts.syncNow'))
    expect(syncButton).toBeDefined()
    await syncButton?.trigger('click')
    await flushPromises()

    expect(syncFromCrs).toHaveBeenCalledWith(expect.objectContaining({
      selected_account_ids: ['openai-1'],
      codex_identity_policy: {
        mode: 'off',
        binding_scope: 'api_key_os',
        session_policy: { mode: 'conversation_isolated' },
        affinity_ttl_seconds: 3600,
        unsupported_policy: 'reject',
        profiles: []
      }
    }))
  })

  it('does not apply a new-account policy to existing OpenAI OAuth accounts by default', async () => {
    previewFromCrs.mockResolvedValueOnce({
      existing_accounts: [{
        crs_account_id: 'existing-openai',
        kind: 'account',
        name: 'Existing OpenAI OAuth',
        platform: 'openai',
        type: 'oauth',
      }],
      new_accounts: [{
        crs_account_id: 'new-openai',
        kind: 'account',
        name: 'New OpenAI OAuth',
        platform: 'openai',
        type: 'oauth',
      }],
    })
    const wrapper = mount(SyncFromCrsModal, {
      props: { show: true, proxies: [] },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true, Select: true } },
    })

    const inputs = wrapper.findAll('form input')
    await inputs[0]?.setValue('https://crs.example.com')
    await inputs[1]?.setValue('admin')
    await inputs[2]?.setValue('secret')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect((wrapper.get('[data-testid="crs-override-existing-codex-identity"]').element as HTMLInputElement).checked).toBe(false)
    const syncButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.accounts.syncNow'))
    await syncButton?.trigger('click')
    await flushPromises()

    expect(syncFromCrs).toHaveBeenCalledWith(expect.objectContaining({
      selected_account_ids: ['new-openai'],
      codex_identity_policy: expect.objectContaining({ mode: 'off' }),
      override_existing_codex_identity_policies: undefined,
    }))
  })

  it('requires an explicit control before replacing existing OpenAI OAuth policies', async () => {
    previewFromCrs.mockResolvedValueOnce({
      existing_accounts: [{
        crs_account_id: 'existing-openai',
        kind: 'account',
        name: 'Existing OpenAI OAuth',
        platform: 'openai',
        type: 'oauth',
      }],
      new_accounts: [],
    })
    const wrapper = mount(SyncFromCrsModal, {
      props: { show: true, proxies: [] },
      global: { stubs: { BaseDialog: BaseDialogStub, Icon: true, Select: true } },
    })

    const inputs = wrapper.findAll('form input')
    await inputs[0]?.setValue('https://crs.example.com')
    await inputs[1]?.setValue('admin')
    await inputs[2]?.setValue('secret')
    await wrapper.get('form').trigger('submit.prevent')
    await flushPromises()

    expect(wrapper.find('[data-testid="codex-identity-policy-toggle"]').exists()).toBe(false)
    await wrapper.get('[data-testid="crs-override-existing-codex-identity"]').setValue(true)
    expect(wrapper.find('[data-testid="codex-identity-policy-toggle"]').exists()).toBe(true)
    const syncButton = wrapper.findAll('button').find((button) =>
      button.text().includes('admin.accounts.syncNow'))
    await syncButton?.trigger('click')
    await flushPromises()

    expect(syncFromCrs).toHaveBeenCalledWith(expect.objectContaining({
      selected_account_ids: [],
      codex_identity_policy: expect.objectContaining({ mode: 'off' }),
      override_existing_codex_identity_policies: true,
    }))
  })
})
