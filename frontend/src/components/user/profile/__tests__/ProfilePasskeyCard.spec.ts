import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ProfilePasskeyCard from '@/components/user/profile/ProfilePasskeyCard.vue'

const { listMock, showErrorMock, showSuccessMock } = vi.hoisted(() => ({
  listMock: vi.fn(),
  showErrorMock: vi.fn(),
  showSuccessMock: vi.fn()
}))

vi.mock('@/api', () => ({
  passkeyAPI: {
    isSupported: () => true,
    list: listMock,
    register: vi.fn(),
    rename: vi.fn(),
    remove: vi.fn()
  }
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({
    showError: showErrorMock,
    showSuccess: showSuccessMock
  })
}))

vi.mock('vue-i18n', () => ({
  useI18n: () => ({
    t: (key: string) => key
  })
}))

function credential(id: number, name: string) {
  return {
    id,
    name,
    created_at: '2026-07-29T00:00:00Z',
    backup: false
  }
}

describe('ProfilePasskeyCard', () => {
  beforeEach(() => {
    listMock.mockReset()
    showErrorMock.mockReset()
    showSuccessMock.mockReset()
    listMock.mockResolvedValue([])
  })

  it('keeps existing credentials manageable while passkey sign-in is disabled', async () => {
    listMock.mockResolvedValue([credential(1, 'existing credential')])
    const wrapper = mount(ProfilePasskeyCard, {
      props: { enabled: false },
      global: { stubs: { Icon: true } }
    })

    await flushPromises()

    expect(listMock).toHaveBeenCalledOnce()
    expect(showErrorMock).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('profile.passkey.featureDisabled')
    expect(wrapper.text()).toContain('existing credential')
    expect(wrapper.text()).toContain('common.edit')
    expect(wrapper.text()).toContain('common.delete')
    expect(wrapper.text()).not.toContain('profile.passkey.add')
  })

  it('silences PASSKEY_DISABLED returned during a settings race', async () => {
    listMock.mockRejectedValue({ code: 403, reason: 'PASSKEY_DISABLED' })

    mount(ProfilePasskeyCard, {
      props: { enabled: true },
      global: { stubs: { Icon: true } }
    })

    await flushPromises()

    expect(listMock).toHaveBeenCalledOnce()
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('reports other credential loading failures', async () => {
    listMock.mockRejectedValue({ code: 500, reason: 'INTERNAL_ERROR' })

    mount(ProfilePasskeyCard, {
      props: { enabled: true },
      global: { stubs: { Icon: true } }
    })

    await flushPromises()

    expect(showErrorMock).toHaveBeenCalledOnce()
    expect(showErrorMock).toHaveBeenCalledWith('profile.passkey.loadFailed')
  })

  it('discards an older credential response after the setting changes', async () => {
    let resolveList!: (value: ReturnType<typeof credential>[]) => void
    listMock
      .mockReturnValueOnce(new Promise((resolve) => {
        resolveList = resolve
      }))
      .mockResolvedValueOnce([credential(2, 'fresh credential')])
    const wrapper = mount(ProfilePasskeyCard, {
      props: { enabled: true },
      global: { stubs: { Icon: true } }
    })
    await flushPromises()

    await wrapper.setProps({ enabled: false })
    await flushPromises()
    resolveList([credential(1, 'stale credential')])
    await flushPromises()

    expect(wrapper.text()).toContain('profile.passkey.featureDisabled')
    expect(wrapper.text()).toContain('fresh credential')
    expect(wrapper.text()).not.toContain('stale credential')
    expect(showErrorMock).not.toHaveBeenCalled()
  })

  it('does not report a credential loading failure after unmount', async () => {
    let rejectList!: (reason: unknown) => void
    listMock.mockReturnValue(new Promise((_resolve, reject) => {
      rejectList = reject
    }))
    const wrapper = mount(ProfilePasskeyCard, {
      props: { enabled: true },
      global: { stubs: { Icon: true } }
    })
    await flushPromises()

    wrapper.unmount()
    rejectList({ code: 500, reason: 'INTERNAL_ERROR' })
    await flushPromises()

    expect(showErrorMock).not.toHaveBeenCalled()
  })
})
