import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import MetaDeviceLogin from '../MetaDeviceLogin.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => (params ? key + JSON.stringify(params) : key) })
  }
})

const startDeviceFlow = vi.fn()
const pollDeviceFlow = vi.fn()

vi.mock('@/api/admin', () => ({
  adminAPI: {
    meta: {
      startDeviceFlow: (...args: unknown[]) => startDeviceFlow(...args),
      pollDeviceFlow: (...args: unknown[]) => pollDeviceFlow(...args)
    }
  }
}))

const started = {
  device_code_id: 'dev-1',
  user_code: 'ABCD-EFGH',
  verification_uri: 'https://auth.meta.com/device',
  verification_uri_complete: 'https://auth.meta.com/device?user_code=ABCD-EFGH',
  expires_in: 900,
  interval: 5
}

async function settle() {
  for (let i = 0; i < 6; i += 1) {
    await Promise.resolve()
  }
}

function mountLogin(props: Record<string, unknown> = {}, onCreated = vi.fn()) {
  const wrapper = mount(MetaDeviceLogin, {
    props: {
      accountOptions: {
        name: 'Muse prod',
        concurrency: 3,
        priority: 1,
        group_ids: [7],
        rate_multiplier: 1.5,
        credentials: { model_mapping: { fast: 'muse-spark-1.1' } }
      },
      pollIntervalSeconds: 5,
      ...props
    },
    attrs: { onCreated },
    global: { stubs: { Icon: true } }
  })
  return { wrapper, onCreated }
}

describe('MetaDeviceLogin', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    startDeviceFlow.mockReset()
    pollDeviceFlow.mockReset()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('starts the device flow and renders the user code with the verification link', async () => {
    startDeviceFlow.mockResolvedValue(started)
    const { wrapper } = mountLogin()

    await wrapper.get('button').trigger('click')
    await settle()

    expect(startDeviceFlow).toHaveBeenCalledTimes(1)
    expect(wrapper.text()).toContain('ABCD-EFGH')
    const link = wrapper.get('a')
    expect(link.attributes('href')).toBe('https://auth.meta.com/device?user_code=ABCD-EFGH')
    expect(link.attributes('target')).toBe('_blank')
    wrapper.unmount()
  })

  it('creates the account once Meta reports the authorization', async () => {
    startDeviceFlow.mockResolvedValue(started)
    pollDeviceFlow.mockResolvedValue({ state: 'authorized', interval: 5, account: { id: 12, platform: 'meta' } })
    const { wrapper, onCreated } = mountLogin()

    await wrapper.get('button').trigger('click')
    await settle()
    await vi.advanceTimersByTimeAsync(5000)
    await settle()

    expect(pollDeviceFlow).toHaveBeenCalledWith(expect.objectContaining({
      device_code_id: 'dev-1',
      name: 'Muse prod',
      concurrency: 3,
      priority: 1,
      group_ids: [7],
      rate_multiplier: 1.5,
      credentials: { model_mapping: { fast: 'muse-spark-1.1' } }
    }))
    expect(onCreated).toHaveBeenCalledWith({ id: 12, platform: 'meta' })
    wrapper.unmount()
  })

  it('keeps polling while authorization is pending', async () => {
    startDeviceFlow.mockResolvedValue(started)
    pollDeviceFlow
      .mockResolvedValueOnce({ state: 'pending', interval: 5 })
      .mockResolvedValueOnce({ state: 'authorized', interval: 5, account: { id: 13 } })
    const { wrapper, onCreated } = mountLogin()

    await wrapper.get('button').trigger('click')
    await settle()
    await vi.advanceTimersByTimeAsync(5000)
    await settle()
    expect(onCreated).not.toHaveBeenCalled()

    await vi.advanceTimersByTimeAsync(5000)
    await settle()
    expect(pollDeviceFlow).toHaveBeenCalledTimes(2)
    expect(onCreated).toHaveBeenCalledWith({ id: 13 })
    wrapper.unmount()
  })

  it('surfaces a denied authorization and stops polling', async () => {
    startDeviceFlow.mockResolvedValue(started)
    pollDeviceFlow.mockResolvedValue({ state: 'denied', interval: 5, message: 'denied by user' })
    const { wrapper } = mountLogin()

    await wrapper.get('button').trigger('click')
    await settle()
    await vi.advanceTimersByTimeAsync(5000)
    await settle()

    expect(wrapper.text()).toContain('denied by user')
    await vi.advanceTimersByTimeAsync(15000)
    await settle()
    expect(pollDeviceFlow).toHaveBeenCalledTimes(1)
    wrapper.unmount()
  })

  it('stops polling after unmount', async () => {
    startDeviceFlow.mockResolvedValue(started)
    pollDeviceFlow.mockResolvedValue({ state: 'pending', interval: 5 })
    const { wrapper } = mountLogin()

    await wrapper.get('button').trigger('click')
    await settle()
    wrapper.unmount()
    await vi.advanceTimersByTimeAsync(20000)
    expect(pollDeviceFlow).not.toHaveBeenCalled()
  })
})
