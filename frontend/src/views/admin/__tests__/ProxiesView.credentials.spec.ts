import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, shallowMount } from '@vue/test-utils'
import ProxiesView from '../ProxiesView.vue'

const { list, update, getAllWithCount } = vi.hoisted(() => ({ list: vi.fn(), update: vi.fn(), getAllWithCount: vi.fn() }))
vi.mock('@/api/admin', () => ({ adminAPI: { proxies: { list, update, getAllWithCount } } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: vi.fn(), showSuccess: vi.fn() }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))
const mountView = () => shallowMount(ProxiesView, {
  global: { stubs: {
    AppLayout: { template: '<div><slot /></div>' },
    TablePageLayout: { template: '<div><slot name="table" /></div>' },
    DataTable: { props: ['data'], template: '<div v-for="row in data" :key="row.id"><slot name="cell-actions" :row="row" /></div>' },
    BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
  } },
})
let wrapper: ReturnType<typeof mountView>
beforeEach(() => {
  vi.clearAllMocks()
  list.mockResolvedValue({ items: [{ id: 9, name: 'proxy', protocol: 'http', host: 'proxy.example', port: 8080, username: 'old-user', status: 'active' }], total: 1, pages: 1 })
  getAllWithCount.mockResolvedValue([])
  update.mockResolvedValue({})
})
afterEach(() => wrapper?.unmount())
async function edit() {
  wrapper = mountView(); await flushPromises()
  await wrapper.findAll('button').find(button => button.text() === 'common.edit')!.trigger('click')
}
async function submit() {
  await wrapper.get('#edit-proxy-form').trigger('submit'); await flushPromises()
  expect(update).toHaveBeenCalledTimes(1)
  return update.mock.calls[0][1]
}

describe('proxy credential updates', () => {
  it('sends an explicit empty username when cleared', async () => {
    await edit()
    const username = wrapper.findAll<HTMLInputElement>('#edit-proxy-form input').find(input => input.element.value === 'old-user')!
    await username.setValue('')
    const payload = await submit()
    expect(payload.username).toBe('')
    expect(payload).not.toHaveProperty('password')
  })

  it('sends an explicit empty password when the user clears the field', async () => {
    await edit()
    await wrapper.get('#edit-proxy-form input[type="password"]').setValue('')
    expect((await submit()).password).toBe('')
  })

  it('omits an untouched password when saving other settings', async () => {
    await edit()
    const payload = await submit()
    expect(payload.username).toBe('old-user')
    expect(payload).not.toHaveProperty('password')
  })

  it('keeps trimming a replacement password', async () => {
    await edit()
    await wrapper.get('#edit-proxy-form input[type="password"]').setValue(' new-password ')
    expect((await submit()).password).toBe('new-password')
  })
})
