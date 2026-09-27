import { beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import ImportDataModal from '../ImportDataModal.vue'

const mocks = vi.hoisted(() => ({
  importData: vi.fn(),
  getAll: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
  showWarning: vi.fn()
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    accounts: { importData: mocks.importData },
    groups: { getAll: mocks.getAll }
  }
}))
vi.mock('@/stores/app', () => ({ useAppStore: () => mocks }))
vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key })
}))

const GroupSelectorStub = {
  name: 'GroupSelector',
  props: ['modelValue', 'groups'],
  emits: ['update:modelValue'],
  template: '<div class="group-selector-stub">{{ groups.length }}</div>'
}

const okResult = { proxy_created: 0, proxy_reused: 0, proxy_failed: 0, account_created: 1, account_failed: 0 }
const fileContent = JSON.stringify({ type: 'sub2api-data', version: 1, exported_at: '2026-09-01T00:00:00Z', proxies: [], accounts: [] })

async function mountOpen() {
  const wrapper = mount(ImportDataModal, {
    props: { show: false },
    global: {
      stubs: {
        BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' },
        GroupSelector: GroupSelectorStub
      }
    }
  })
  await wrapper.setProps({ show: true })
  await flushPromises()
  return wrapper
}

async function pickFile(wrapper: Awaited<ReturnType<typeof mountOpen>>, count = 1) {
  const input = wrapper.get('input[type="file"]')
  Object.defineProperty(input.element, 'files', {
    configurable: true,
    value: Array.from({ length: count }, (_, i) => new File([fileContent], `backup-${i}.json`, { type: 'application/json' }))
  })
  await input.trigger('change')
}

async function submit(wrapper: Awaited<ReturnType<typeof mountOpen>>, expectedCalls: number) {
  await wrapper.get('form').trigger('submit')
  await vi.waitFor(() => expect(mocks.importData).toHaveBeenCalledTimes(expectedCalls))
  await flushPromises()
}

describe('account ImportDataModal', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mocks.getAll.mockResolvedValue([{ id: 5, name: 'openai-default', platform: 'openai' }])
    mocks.importData.mockResolvedValue(okResult)
  })

  it('loads groups and sends the selected group_ids with an idempotency key', async () => {
    const wrapper = await mountOpen()
    expect(mocks.getAll).toHaveBeenCalledTimes(1)
    expect(wrapper.get('.group-selector-stub').text()).toBe('1')

    await pickFile(wrapper)
    wrapper.findComponent(GroupSelectorStub).vm.$emit('update:modelValue', [5])
    await submit(wrapper, 1)

    const [payload, options] = mocks.importData.mock.calls[0]
    expect(payload.group_ids).toEqual([5])
    expect(payload.skip_default_group_bind).toBe(true)
    expect(options.idempotencyKey).toMatch(/^account-import-/)
  })

  it('reuses the key and an identical merged payload when retrying, and rotates the key when groups change', async () => {
    const wrapper = await mountOpen()
    await pickFile(wrapper, 2)

    mocks.importData.mockRejectedValueOnce({ message: 'timeout of 300000ms exceeded' })
    await submit(wrapper, 1)
    expect(mocks.showError).toHaveBeenCalledWith('timeout of 300000ms exceeded')

    await submit(wrapper, 2)
    const firstKey = mocks.importData.mock.calls[0][1].idempotencyKey
    expect(mocks.importData.mock.calls[1][1].idempotencyKey).toBe(firstKey)
    expect(mocks.importData.mock.calls[1][0]).toEqual(mocks.importData.mock.calls[0][0])

    wrapper.findComponent(GroupSelectorStub).vm.$emit('update:modelValue', [5])
    await flushPromises()
    await submit(wrapper, 3)
    expect(mocks.importData.mock.calls[2][1].idempotencyKey).not.toBe(firstKey)
    expect(mocks.importData.mock.calls[2][0].group_ids).toEqual([5])
  })

  it('reports a failed group load instead of silently showing no groups', async () => {
    mocks.getAll.mockRejectedValueOnce(new Error('boom'))
    await mountOpen()
    expect(mocks.showError).toHaveBeenCalledWith('admin.groups.failedToLoad')
  })
})
