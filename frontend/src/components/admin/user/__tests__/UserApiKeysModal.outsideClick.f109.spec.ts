import { afterEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import { nextTick } from 'vue'
import type { AdminUser } from '@/types'
import UserApiKeysModal from '../UserApiKeysModal.vue'

const f109GetKeys = vi.hoisted(() => vi.fn())
vi.mock('@/api/admin', () => ({ adminAPI: {
  users: { getUserApiKeys: f109GetKeys },
  groups: { getAll: vi.fn().mockResolvedValue([{ id: 3, name: 'Pro', platform: 'openai' }]) }
} }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('@/utils/format', () => ({ formatDateTime: (value: string) => value }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

let f109Wrapper: ReturnType<typeof mount> | null = null

afterEach(() => {
  f109Wrapper?.unmount()
  f109Wrapper = null
  document.body.innerHTML = ''
  document.body.classList.remove('modal-open')
})

describe('F1-09: UserApiKeysModal group selector inside BaseDialog', () => {
  it('closes the group selector on a click elsewhere in the dialog', async () => {
    f109GetKeys.mockResolvedValue({ items: [
      { id: 11, name: 'f109-key', key: 'sk-example-key-value-for-tests', status: 'active', created_at: '2026-09-20', group_id: null }
    ] })
    f109Wrapper = mount(UserApiKeysModal, {
      attachTo: document.body,
      props: { show: false, user: { id: 1, email: 'user1@example.com', username: 'user1' } as AdminUser },
      global: { stubs: { Icon: true, GroupBadge: true, GroupOptionItem: true } }
    })
    await f109Wrapper.setProps({ show: true })
    await flushPromises()

    const groupButton = document.querySelector<HTMLButtonElement>('.modal-body button')!
    groupButton.click()
    await nextTick()
    const selector = () => document.querySelector('.fixed.w-64')
    expect(selector()).not.toBeNull()

    // 点击下拉内部不应关闭
    ;(selector() as HTMLElement).click()
    await nextTick()
    expect(selector()).not.toBeNull()

    Array.from(document.querySelectorAll<HTMLElement>('.modal-body span')).find(el => el.textContent === 'f109-key')!.click()
    await nextTick()
    expect(selector()).toBeNull()
  })
})
