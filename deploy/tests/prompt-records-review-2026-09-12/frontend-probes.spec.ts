// Characterization probes for the audit; a passing test confirms the defect.
import { beforeEach, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PromptRecordsView from '../PromptRecordsView.vue'

const mocks = vi.hoisted(() => ({
  list: vi.fn(), get: vi.fn(), error: vi.fn(), copy: vi.fn(),
}))
vi.mock('../api', () => ({
  listPromptRecords: mocks.list, getPromptRecord: mocks.get,
  deletePromptRecord: vi.fn(), batchDeletePromptRecords: vi.fn(),
  updatePromptRecordingConfig: vi.fn(),
  getPromptRecordingConfig: vi.fn().mockResolvedValue({ enabled: true }),
}))
vi.mock('@/api/admin/usage', () => ({ adminUsageAPI: { searchUsers: vi.fn() }, default: { searchUsers: vi.fn() } }))
vi.mock('@/stores/app', () => ({ useAppStore: () => ({ showError: mocks.error, showSuccess: vi.fn() }) }))
vi.mock('@/composables/useClipboard', () => ({ useClipboard: () => ({ copyToClipboard: mocks.copy }) }))
vi.mock('vue-i18n', async () => ({
  ...await vi.importActual<typeof import('vue-i18n')>('vue-i18n'),
  useI18n: () => ({ t: (key: string) => key }),
}))

const row = { id: 7, request_id: 'synthetic', username: 'synthetic', user_email: '', user_id: 1, api_key_id: 1, message_count: 1, prompt_length: 10, created_at: '2026-09-12T12:00:00Z' }
const page = { items: [row], total: 100, page: 1, page_size: 20 }
const stubs = {
  AppLayout: { template: '<div><slot /></div>' },
  Pagination: { emits: ['update:page'], template: '<button data-test="audit-next" @click="$emit(\'update:page\', 2)">next</button>' },
}

beforeEach(() => {
  vi.clearAllMocks()
  mocks.list.mockResolvedValue(page)
})

it('observes large integer rounding in the detail display while copying the original', async () => {
  const body = '{"seed":9007199254740993,"input":"synthetic"}'
  mocks.get.mockResolvedValue({ ...row, request_body: body })
  const wrapper = mount(PromptRecordsView, { attachTo: document.body, global: { stubs } })
  await flushPromises()
  await wrapper.get('[data-test="prompt-record-detail-7"]').trigger('click')
  await flushPromises()
  expect(document.body.textContent).toContain('9007199254740992')
  expect(document.body.textContent).not.toContain('9007199254740993')
  document.body.querySelector<HTMLButtonElement>('[data-test="prompt-record-copy-request-body"]')!.click()
  await flushPromises()
  expect(mocks.copy).toHaveBeenCalledWith(body, 'admin.promptRecords.requestBodyCopied')
  wrapper.unmount()
})

it('observes an obsolete request failure overriding a successful newer page', async () => {
  const wrapper = mount(PromptRecordsView, { global: { stubs } })
  await flushPromises()
  let rejectOld!: (error: Error) => void
  mocks.list.mockImplementationOnce(() => new Promise((_, reject) => { rejectOld = reject }))
  await wrapper.get('form').trigger('submit')
  await wrapper.get('[data-test="audit-next"]').trigger('click')
  await flushPromises()
  expect(mocks.error).not.toHaveBeenCalled()
  expect(mocks.list.mock.calls[1]).toHaveLength(1)
  rejectOld(new Error('synthetic obsolete request failure'))
  await flushPromises()
  expect(mocks.error).toHaveBeenCalledWith('admin.promptRecords.loadFailed')
  expect(wrapper.find('[role="alert"]').exists()).toBe(true)
  wrapper.unmount()
})
