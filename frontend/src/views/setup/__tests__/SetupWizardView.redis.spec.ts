import { flushPromises, mount } from '@vue/test-utils'
import { describe, expect, it, vi } from 'vitest'
import SetupWizardView from '../SetupWizardView.vue'

const { testRedis } = vi.hoisted(() => ({ testRedis: vi.fn().mockResolvedValue(undefined) }))
vi.mock('@/api/setup', () => ({
  testDatabase: vi.fn().mockResolvedValue(undefined), testRedis, install: vi.fn()
}))
vi.mock('@/api/client', () => ({ buildGatewayUrl: (path: string) => path }))
vi.mock('vue-i18n', () => ({ useI18n: () => ({ t: (key: string) => key }) }))

describe('Redis UDS setup', () => {
  it('normalizes and disables port/TLS for both Unix URLs, then restores TCP inputs', async () => {
    const wrapper = mount(SetupWizardView, { global: { stubs: { Icon: true, Select: true } } })
    const clickText = async (text: string) => {
      const button = wrapper.findAll('button').find(b => b.text().includes(text))
      expect(button).toBeDefined()
      await button!.trigger('click')
      await flushPromises()
    }
    await clickText('setup.status.testConnection')
    await clickText('common.next')
    const host = wrapper.get('input[placeholder="localhost"]')
    const port = wrapper.get('input[placeholder="6379"]')
    const tls = wrapper.get('button[role="switch"]')
    await tls.trigger('click')
    expect(tls.attributes('aria-checked')).toBe('true')
    for (const address of ['unix:/tmp/redis.sock', 'unix:///tmp/redis.sock']) {
      await host.setValue(address)
      expect((port.element as HTMLInputElement).value).toBe('0')
      expect((port.element as HTMLInputElement).disabled).toBe(true)
      expect((tls.element as HTMLButtonElement).disabled).toBe(true)
      expect(tls.attributes('aria-checked')).toBe('false')
      await wrapper.get('button.btn-secondary.w-full').trigger('click')
      await flushPromises()
      expect(testRedis).toHaveBeenLastCalledWith(expect.objectContaining({host:address,port:0,enable_tls:false}))
    }
    await host.setValue('127.0.0.1')
    expect((port.element as HTMLInputElement).value).toBe('6379')
    expect((port.element as HTMLInputElement).disabled).toBe(false)
    expect((tls.element as HTMLButtonElement).disabled).toBe(false)
    expect(tls.attributes('aria-checked')).toBe('false')
    await port.setValue(6380)
    await host.setValue('localhost')
    expect((port.element as HTMLInputElement).value).toBe('6380')
    wrapper.unmount()
  })
})
