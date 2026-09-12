import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount } from '@vue/test-utils'
import PromptRecordsView from '../PromptRecordsView.vue'

const mocks = vi.hoisted(() => ({
  listPromptRecords: vi.fn(),
  getPromptRecord: vi.fn(),
  deletePromptRecord: vi.fn(),
	batchDeletePromptRecords: vi.fn(),
	getPromptRecordingConfig: vi.fn(),
	updatePromptRecordingConfig: vi.fn(),
  searchUsers: vi.fn(),
  showError: vi.fn(),
  showSuccess: vi.fn(),
}))

vi.mock('../api', () => ({
  listPromptRecords: mocks.listPromptRecords,
  getPromptRecord: mocks.getPromptRecord,
  deletePromptRecord: mocks.deletePromptRecord,
	batchDeletePromptRecords: mocks.batchDeletePromptRecords,
	getPromptRecordingConfig: mocks.getPromptRecordingConfig,
	updatePromptRecordingConfig: mocks.updatePromptRecordingConfig,
}))

vi.mock('@/api/admin/usage', () => ({
  adminUsageAPI: { searchUsers: mocks.searchUsers },
  default: { searchUsers: mocks.searchUsers },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showError: mocks.showError, showSuccess: mocks.showSuccess }),
}))

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) =>
        key.replace(/\{(\w+)\}/g, (_, token) => String(params?.[token] ?? `{${token}}`)),
    }),
  }
})

const summary = {
  id: 7,
  request_id: 'req-7',
  turn_no: 1,
  stage: 'http',
  user_id: 2,
  username: 'alice',
  user_email: 'alice@example.com',
  api_key_id: 3,
  api_key_name: 'primary',
  group_name: 'default',
  provider: 'openai',
  endpoint: '/v1/chat/completions',
  protocol: 'openai_chat',
  model: 'gpt-test',
  prompt_hash: 'a'.repeat(64),
  prompt_length: 28,
  message_count: 1,
  risk_status: 'pending',
  created_at: '2026-09-11T12:00:00Z',
}

describe('PromptRecordsView', () => {
  beforeEach(() => {
    Object.values(mocks).forEach((mock) => mock.mockReset())
    mocks.listPromptRecords.mockResolvedValue({
      items: [summary],
      page: 1,
      page_size: 20,
      total: 1,
      total_pages: 1,
      queue: {
        queue_length: 0,
        queue_capacity: 256,
        overflow_length: 0,
        overflow_capacity: 2048,
        worker_count: 4,
        dropped_total: 0,
        persist_failed_total: 0,
      },
    })
    mocks.getPromptRecord.mockResolvedValue({
      ...summary,
      prompt_text: 'PROMPT_DETAIL_ONLY_AFTER_CLICK',
      risk_result: '{}',
      response_text: 'RESPONSE_DETAIL_ONLY_AFTER_CLICK',
      response_length: 32,
      response_truncated: false,
      response_captured_at: '2026-09-11T12:00:02Z',
    })
    mocks.deletePromptRecord.mockResolvedValue(undefined)
		mocks.batchDeletePromptRecords.mockResolvedValue({ deleted: 1 })
		mocks.getPromptRecordingConfig.mockResolvedValue({ enabled: true })
		mocks.updatePromptRecordingConfig.mockResolvedValue({ enabled: false })
    mocks.searchUsers.mockResolvedValue([])
  })

  afterEach(() => {
    document.body.innerHTML = ''
  })

	it('loads full prompt text only after the row detail button is clicked', async () => {
    const wrapper = mount(PromptRecordsView, {
      attachTo: document.body,
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
        },
      },
	})

		await flushPromises()
    expect(mocks.listPromptRecords).toHaveBeenCalledOnce()
    expect(mocks.getPromptRecord).not.toHaveBeenCalled()
    expect(document.body.textContent).not.toContain('PROMPT_DETAIL_ONLY_AFTER_CLICK')
    expect(document.body.textContent).not.toContain('RESPONSE_DETAIL_ONLY_AFTER_CLICK')

    await wrapper.get('[data-test="prompt-record-detail-7"]').trigger('click')
    await flushPromises()

    expect(mocks.getPromptRecord).toHaveBeenCalledWith(7)
    expect(document.body.textContent).toContain('PROMPT_DETAIL_ONLY_AFTER_CLICK')
    expect(document.body.textContent).toContain('RESPONSE_DETAIL_ONLY_AFTER_CLICK')
		wrapper.unmount()
	})

	it('updates the request and response recording switch', async () => {
		const wrapper = mount(PromptRecordsView, {
			global: {
				stubs: { AppLayout: { template: '<div><slot /></div>' } },
			},
		})
		await flushPromises()

		const toggle = wrapper.get('[data-test="prompt-recording-toggle"]')
		expect(toggle.attributes('aria-checked')).toBe('true')
		await toggle.trigger('click')
		await flushPromises()

		expect(mocks.updatePromptRecordingConfig).toHaveBeenCalledWith(false)
		expect(toggle.attributes('aria-checked')).toBe('false')
		expect(mocks.showSuccess).toHaveBeenCalledWith('admin.promptRecords.recordingDisabledSuccess')
		wrapper.unmount()
	})

  it('deletes one row only after confirmation', async () => {
    const wrapper = mount(PromptRecordsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          ConfirmDialog: {
            props: ['show'],
            emits: ['confirm', 'cancel'],
            template: '<button v-if="show" data-test="confirm-delete" @click="$emit(\'confirm\')">confirm</button>',
          },
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-test="prompt-record-delete-7"]').trigger('click')
    expect(mocks.deletePromptRecord).not.toHaveBeenCalled()
    await wrapper.get('[data-test="confirm-delete"]').trigger('click')
    await flushPromises()

    expect(mocks.deletePromptRecord).toHaveBeenCalledWith(7)
    expect(mocks.listPromptRecords).toHaveBeenCalledTimes(2)
    wrapper.unmount()
  })

  it('saves content switches independently and preserves state on failure', async () => {
    mocks.getPromptRecordingConfig.mockResolvedValue({ enabled: true, headers_enabled: true, prompt_enabled: true })
    mocks.updatePromptRecordingConfig.mockResolvedValueOnce({ enabled: true, headers_enabled: false, prompt_enabled: true })
    const wrapper = mount(PromptRecordsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()
    const headers = wrapper.get('[data-test="prompt-recording-headers_enabled"]')
    const prompt = wrapper.get('[data-test="prompt-recording-prompt_enabled"]')
    expect(headers.attributes('aria-checked')).toBe('true')
    expect(prompt.attributes('aria-checked')).toBe('true')
    await headers.trigger('click')
    await flushPromises()
    expect(mocks.updatePromptRecordingConfig).toHaveBeenCalledWith({ headers_enabled: false })
    expect(headers.attributes('aria-checked')).toBe('false')
    expect(prompt.attributes('aria-checked')).toBe('true')
    mocks.updatePromptRecordingConfig.mockRejectedValueOnce(new Error('save failed'))
    await prompt.trigger('click')
    await flushPromises()
    expect(mocks.updatePromptRecordingConfig).toHaveBeenLastCalledWith({ prompt_enabled: false })
    expect(prompt.attributes('aria-checked')).toBe('true')
    expect(mocks.showError).toHaveBeenCalledWith('admin.promptRecords.recordingUpdateFailed')
    wrapper.unmount()
  })

  it('defaults preset filtering off and reflects successful toggles while preserving failures', async () => {
    const wrapper = mount(PromptRecordsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()
    const toggle = wrapper.get('[data-test="prompt-recording-filter_preset"]')
    expect(toggle.attributes('aria-checked')).toBe('false')
    mocks.updatePromptRecordingConfig.mockResolvedValueOnce({ enabled: true, headers_enabled: true, prompt_enabled: true, filter_preset: true })
    await toggle.trigger('click')
    await flushPromises()
    expect(mocks.updatePromptRecordingConfig).toHaveBeenLastCalledWith({ filter_preset: true })
    expect(toggle.attributes('aria-checked')).toBe('true')
    mocks.updatePromptRecordingConfig.mockRejectedValueOnce(new Error('save failed'))
    await toggle.trigger('click')
    await flushPromises()
    expect(mocks.updatePromptRecordingConfig).toHaveBeenLastCalledWith({ filter_preset: false })
    expect(toggle.attributes('aria-checked')).toBe('true')
    mocks.updatePromptRecordingConfig.mockResolvedValueOnce({ enabled: true, headers_enabled: true, prompt_enabled: true, filter_preset: false })
    await toggle.trigger('click')
    await flushPromises()
    expect(toggle.attributes('aria-checked')).toBe('false')
    wrapper.unmount()
  })

  it('shows the complete request body and headers only in details', async () => {
    mocks.getPromptRecord.mockResolvedValue({ ...summary, request_body: '{"tools":[{"name":"full-request-tool"}]}', request_headers: '{"X-Test":["saved-header"]}' })
    const wrapper = mount(PromptRecordsView, { attachTo: document.body, global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } } })
    await flushPromises()
    expect(document.body.textContent).not.toContain('saved-header')
    await wrapper.get('[data-test="prompt-record-detail-7"]').trigger('click')
    await flushPromises()
    expect(document.body.textContent).toContain('full-request-tool')
    expect(document.body.textContent).toContain('saved-header')
    wrapper.unmount()
  })

  it('batch deletes selected rows', async () => {
    const wrapper = mount(PromptRecordsView, {
      global: {
        stubs: {
          AppLayout: { template: '<div><slot /></div>' },
          ConfirmDialog: {
            props: ['show'],
            emits: ['confirm', 'cancel'],
            template: '<button v-if="show" data-test="confirm-delete" @click="$emit(\'confirm\')">confirm</button>',
          },
        },
      },
    })
    await flushPromises()

    await wrapper.get('[data-test="select-row"]').setValue(true)
    await wrapper.get('[data-test="prompt-record-batch-delete"]').trigger('click')
    await wrapper.get('[data-test="confirm-delete"]').trigger('click')
    await flushPromises()

    expect(mocks.batchDeletePromptRecords).toHaveBeenCalledWith([7])
    wrapper.unmount()
  })
})
