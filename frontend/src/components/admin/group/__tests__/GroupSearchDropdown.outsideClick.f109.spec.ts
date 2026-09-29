import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { defineComponent, h, nextTick } from 'vue'
import type { AdminGroup } from '@/types'
import BaseDialog from '@/components/common/BaseDialog.vue'
import GroupRPMOverridesModal from '../GroupRPMOverridesModal.vue'
import GroupRateMultipliersModal from '../GroupRateMultipliersModal.vue'
import CodexManifestAccountsField from '../CodexManifestAccountsField.vue'

const f109Mocks = vi.hoisted(() => ({
  usersList: vi.fn(),
  accountsList: vi.fn(),
  getGroupRPMOverrides: vi.fn(),
  getGroupRateMultipliers: vi.fn()
}))
vi.mock('@/api/admin', () => ({
  adminAPI: {
    users: { list: f109Mocks.usersList },
    accounts: { list: f109Mocks.accountsList },
    groups: {
      getGroupRPMOverrides: f109Mocks.getGroupRPMOverrides,
      getGroupRateMultipliers: f109Mocks.getGroupRateMultipliers
    }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() }) }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

let f109Wrapper: ReturnType<typeof mount> | null = null

beforeEach(() => {
  vi.useFakeTimers()
  f109Mocks.usersList.mockResolvedValue({ items: [{ id: 7, email: 'user@example.com', status: 'active' }] })
  f109Mocks.accountsList.mockResolvedValue({ items: [{ id: 5, name: 'oauth-five' }] })
  f109Mocks.getGroupRPMOverrides.mockResolvedValue([])
  f109Mocks.getGroupRateMultipliers.mockResolvedValue([])
})

afterEach(() => {
  f109Wrapper?.unmount()
  f109Wrapper = null
  vi.useRealTimers()
  vi.clearAllMocks()
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
})

const f109Stubs = { Icon: true, PlatformIcon: true, Pagination: true }

async function f109Type(input: HTMLInputElement, value: string) {
  input.value = value
  input.dispatchEvent(new Event('input', { bubbles: true }))
  await vi.advanceTimersByTimeAsync(300)
  await flushPromises()
}

describe.each([
  ['GroupRPMOverridesModal', GroupRPMOverridesModal, '100'],
  ['GroupRateMultipliersModal', GroupRateMultipliersModal, '1.0']
] as const)('F1-09: %s user search list closes on a click elsewhere in the dialog', (_name, Modal, valuePlaceholder) => {
  it('keeps the list open for clicks in the search box and closes it for other fields', async () => {
    f109Wrapper = mount(Modal, {
      attachTo: document.body,
      props: { show: false, group: { id: 1, name: 'Group', platform: 'openai' } as AdminGroup },
      global: { stubs: f109Stubs }
    })
    await f109Wrapper.setProps({ show: true })
    await flushPromises()

    const search = document.querySelector<HTMLInputElement>('input[placeholder="admin.groups.searchUserPlaceholder"]')!
    await f109Type(search, 'user')
    const hasResult = () => document.body.textContent!.includes('user@example.com')
    expect(hasResult()).toBe(true)

    search.click()
    await nextTick()
    expect(hasResult()).toBe(true)

    document.querySelector<HTMLInputElement>(`input[placeholder="${valuePlaceholder}"]`)!.click()
    await nextTick()
    expect(hasResult()).toBe(false)
  })
})

describe('F1-09: CodexManifestAccountsField dropdown inside a BaseDialog', () => {
  it('closes the account search dropdown on a click elsewhere in the dialog', async () => {
    const Host = defineComponent({
      setup() {
        return () =>
          h(BaseDialog, { show: true, title: 'Edit group' }, {
            default: () => [
              h('input', { class: 'f109-other', type: 'text' }),
              h(CodexManifestAccountsField, {
                groupId: 7,
                modelValue: { enabled: true, account_ids: [], fallback_to_scheduler: false }
              })
            ]
          })
      }
    })
    f109Wrapper = mount(Host, { attachTo: document.body, global: { stubs: f109Stubs } })
    await nextTick()
    await nextTick()

    const search = document.querySelector<HTMLInputElement>('[data-testid="codex-manifest-search"]')!
    search.dispatchEvent(new Event('focus'))
    await f109Type(search, 'oauth')
    const dropdown = () => document.querySelector('[data-testid="codex-manifest-dropdown"]')
    expect(dropdown()).not.toBeNull()

    search.click()
    await nextTick()
    expect(dropdown()).not.toBeNull()

    document.querySelector<HTMLInputElement>('.f109-other')!.click()
    await nextTick()
    expect(dropdown()).toBeNull()
  })
})
