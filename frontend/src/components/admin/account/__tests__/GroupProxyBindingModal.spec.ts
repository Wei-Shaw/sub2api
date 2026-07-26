import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import GroupProxyBindingModal from '../GroupProxyBindingModal.vue'

const { bindProxyByGroup, showSuccess, showWarning, showError } = vi.hoisted(() => ({
  bindProxyByGroup: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn(),
  showError: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { bindProxyByGroup }
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess, showWarning, showError })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) =>
      key === 'admin.accounts.groupProxyBinding.accountCount'
        ? `${params?.count} accounts`
        : key
  })
}))

const groups = [
  { id: 12, name: 'Grok', platform: 'grok', account_count: 38, sort_order: 2 },
  { id: 3, name: 'Codex', platform: 'openai', account_count: 9, sort_order: 1 }
] as any

const proxies = [
  { id: 7, name: 'Residential US', protocol: 'socks5', host: 'proxy.example.com', port: 12323 }
] as any

const globalStubs = {
  BaseDialog: {
    props: ['show'],
    template: '<div v-if="show"><slot /><slot name="footer" /></div>'
  },
  Select: {
    props: ['modelValue', 'options'],
    emits: ['update:modelValue'],
    template: `
      <select
        data-testid="group-select"
        :value="modelValue ?? ''"
        @change="$emit('update:modelValue', Number($event.target.value))"
      >
        <option value=""></option>
        <option v-for="option in options" :key="option.value" :value="option.value">
          {{ option.label }}
        </option>
      </select>
    `
  },
  ProxySelector: {
    props: ['modelValue', 'proxies', 'allowNoProxy'],
    emits: ['update:modelValue'],
    template: `
      <select
        data-testid="proxy-select"
        :data-allow-no-proxy="String(allowNoProxy)"
        :value="modelValue ?? ''"
        @change="$emit('update:modelValue', Number($event.target.value))"
      >
        <option value=""></option>
        <option v-for="proxy in proxies" :key="proxy.id" :value="proxy.id">
          {{ proxy.name }}
        </option>
      </select>
    `
  },
  Icon: true
}

const mountModal = (groupItems = groups) => mount(GroupProxyBindingModal, {
  props: { show: true, groups: groupItems, proxies },
  global: { stubs: globalStubs }
})

describe('GroupProxyBindingModal', () => {
  beforeEach(() => {
    bindProxyByGroup.mockReset()
    showSuccess.mockReset()
    showWarning.mockReset()
    showError.mockReset()
    bindProxyByGroup.mockResolvedValue({ success: 38, failed: 0, results: [] })
  })

  it('binds the selected proxy to the complete selected group', async () => {
    const wrapper = mountModal()

    expect(wrapper.get('[data-testid="proxy-select"]').attributes('data-allow-no-proxy')).toBe('false')
    await wrapper.get('[data-testid="group-select"]').setValue('12')
    await wrapper.get('[data-testid="proxy-select"]').setValue('7')

    expect(wrapper.text()).toContain('38 accounts')
    await wrapper.get('[data-testid="bind-group-proxy-submit"]').trigger('click')
    await flushPromises()

    expect(bindProxyByGroup).toHaveBeenCalledWith(12, 7)
    expect(showSuccess).toHaveBeenCalled()
    expect(wrapper.emitted('updated')?.[0]?.[0]).toMatchObject({ success: 38, failed: 0 })
  })

  it('does not submit a group with no accounts', async () => {
    const wrapper = mountModal([{ ...groups[0], account_count: 0 }])

    await wrapper.get('[data-testid="group-select"]').setValue('12')
    await wrapper.get('[data-testid="proxy-select"]').setValue('7')

    expect(wrapper.get('[data-testid="bind-group-proxy-submit"]').attributes()).toHaveProperty('disabled')
    expect(bindProxyByGroup).not.toHaveBeenCalled()
  })
})
