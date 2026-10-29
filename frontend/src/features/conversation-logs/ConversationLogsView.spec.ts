import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ConversationLogsView from './ConversationLogsView.vue'
import { conversationAPI } from './api'

vi.mock('./api', () => ({ conversationAPI: { config: vi.fn(), save: vi.fn(), list: vi.fn(), detail: vi.fn(), remove: vi.fn() } }))
vi.mock('vue-i18n', async (importOriginal) => ({ ...await importOriginal<typeof import('vue-i18n')>(), useI18n: () => ({ locale: { value: 'zh-CN' } }) }))
const record = { id: 1, request_id: 'test-request', user_id: 7, api_key_id: 9, user_label: 'test@example.test', key_name: 'test-key', created_at: '2026-10-24T13:00:00Z', endpoint: '/v1/chat/completions', model: 'mock-chat', stream: true, status_code: 200, outcome: 'completed', duration_ms: 100, prompt: '<script>alert(1)</script>', answer: 'model reply', request_truncated: false, response_truncated: false, request_body: '{"messages":[]}', response_body: 'data: [DONE]', tool_calls: [], request_bytes: 128, response_bytes: 64, redacted: true }
const runtime = { config: { enabled: false, retention_days: 7, max_body_bytes: 1048576, redact_sensitive: true }, dropped_records: 0, failed_writes: 0, saved_records: 0, queue_depth: 0, websocket_capture: false }

function render() {
  return mount(ConversationLogsView, { global: { stubs: { AppLayout: { template: '<div><slot /></div>' }, BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /><slot name="footer" /></div>' } } } })
}
describe('conversation log admin view', () => {
  beforeEach(() => {
    vi.resetAllMocks()
    vi.mocked(conversationAPI.config).mockResolvedValue(structuredClone(runtime))
    vi.mocked(conversationAPI.list).mockResolvedValue({ items: [record], total: 1, page: 1, page_size: 20 })
    vi.mocked(conversationAPI.detail).mockResolvedValue(record)
  })
  it('renders prompts as text and opens the detail view', async () => {
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain(record.prompt)
    expect(wrapper.find('script').exists()).toBe(false)
    const view = wrapper.findAll('button').find(b => b.text() === '查看')!
    await view.trigger('click'); await flushPromises()
    expect(conversationAPI.detail).toHaveBeenCalledWith(1)
    expect(wrapper.text()).toContain('完整请求上下文')
    expect(wrapper.text()).toContain('model reply')
  })
  it('requires an explicit save to enable capture', async () => {
    const wrapper = render(); await flushPromises()
    await wrapper.get('[data-test="capture-enabled"]').setValue(true)
    expect(conversationAPI.save).not.toHaveBeenCalled()
    await wrapper.findAll('form')[0].trigger('submit'); await flushPromises()
    expect(conversationAPI.save).toHaveBeenCalledWith({ enabled: true, retention_days: 7, max_body_bytes: 1048576, redact_sensitive: true })
  })
  it('saves the explicit redaction choice only after submitting', async () => {
    const wrapper = render(); await flushPromises()
    await wrapper.get('[data-test="capture-redaction"]').setValue(false)
    expect(conversationAPI.save).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('脱敏关闭')
    await wrapper.findAll('form')[0].trigger('submit'); await flushPromises()
    expect(conversationAPI.save).toHaveBeenCalledWith({ enabled: false, retention_days: 7, max_body_bytes: 1048576, redact_sensitive: false })
  })
  it('uses the record mode rather than the current setting in detail labels', async () => {
    vi.mocked(conversationAPI.detail).mockResolvedValue({ ...record, redacted: false })
    const wrapper = render(); await flushPromises()
    await wrapper.findAll('button').find(b => b.text() === '查看')!.trigger('click'); await flushPromises()
    expect(wrapper.text()).toContain('JSON，未脱敏')
    expect(wrapper.text()).not.toContain('JSON，已脱敏')
  })
  it('shows an empty-state instruction instead of fake history', async () => {
    vi.mocked(conversationAPI.list).mockResolvedValue({ items: [], total: 0, page: 1, page_size: 20 })
    const wrapper = render(); await flushPromises()
    expect(wrapper.text()).toContain('暂无记录')
  })
})
