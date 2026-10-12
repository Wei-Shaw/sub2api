import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { flushPromises, mount, type VueWrapper } from '@vue/test-utils'
import { createI18n } from 'vue-i18n'
import { baseCompile } from '@intlify/message-compiler'
import VideoTasksView from '../VideoTasksView.vue'
import DataTable from '@/components/common/DataTable.vue'
import Select from '@/components/common/Select.vue'
import Pagination from '@/components/common/Pagination.vue'
import { videoTasksAPI, videoTaskStatuses, type VideoTaskAdminItem } from '@/api/admin/videoTasks'
import { displayStatus, elapsedParts, formatCost, reasonKey } from '../videoTasksPresentation'
import enCommon from '@/i18n/locales/en/common'
import zhCommon from '@/i18n/locales/zh/common'
import enDashboard from '@/i18n/locales/en/dashboard'
import zhDashboard from '@/i18n/locales/zh/dashboard'
import enAdmin from '@/i18n/locales/en/admin/videoTasks'
import zhAdmin from '@/i18n/locales/zh/admin/videoTasks'

vi.mock('@/api/client', () => ({ apiClient: { get: vi.fn() } }))
vi.mock('@/components/layout/AppLayout.vue', () => ({ default: { template: '<main><slot /></main>' } }))

const item = (overrides: Partial<VideoTaskAdminItem> = {}): VideoTaskAdminItem => ({
  kind: 'task', task_id: 'video-1', user_id: 3, user_email: 'owner@example.test',
  api_key_id: 4, api_key_name: 'Named key', account_id: 5, account_name: 'Video account',
  group_id: 6, group_name: 'Video group', model: 'seedance', state: 'pending',
  display_status: 'in_progress', reason_code: '', created_at: '2026-09-15T01:00:00Z',
  updated_at: '2026-09-15T01:05:00Z', completed_at: null, next_attempt_at: '2026-09-15T01:06:00Z',
  attempts: 2, expected_cost: null, charged_cost: null, usage_log_id: null,
  billing_status: 'not_charged', video_duration_seconds: null, video_resolution: '', ...overrides
})
const response = (items: VideoTaskAdminItem[] = [], page = 1, total = items.length, pageSize = 20) => ({
  items, total, page, page_size: pageSize, pages: Math.ceil(total / pageSize)
})
let wrapper: VueWrapper
let list: ReturnType<typeof vi.spyOn>
// Vitest aliases vue-i18n to its runtime-only build. Precompile real translations for it.
function compileMessages(messages: Record<string, unknown>): Record<string, any> {
  return Object.fromEntries(Object.entries(messages).map(([key, value]) => [key,
    typeof value === 'string'
      ? new Function(`return ${baseCompile(value).code}`)()
      : compileMessages(value as Record<string, unknown>)
  ]))
}
const mountView = (locale = 'en') => {
  wrapper = mount(VideoTasksView, {
    global: {
      plugins: [createI18n({ legacy: false, locale, messages: {
        en: compileMessages({ ...enCommon, pagination: enDashboard.pagination, admin: enAdmin }),
        zh: compileMessages({ ...zhCommon, pagination: zhDashboard.pagination, admin: zhAdmin })
      } })],
      stubs: { teleport: true }
    }
  })
  return wrapper
}
const select = (id: string) => wrapper.findAllComponents(Select).find(component => component.props('id') === id)!
const refresh = () => wrapper.findAll('button').find(button => button.text() === 'Refresh')!.trigger('click')

beforeEach(() => {
  list = vi.spyOn(videoTasksAPI, 'list').mockResolvedValue(response())
})
afterEach(() => { wrapper?.unmount(); vi.restoreAllMocks() })

describe('video task presentation safety', () => {
  it('keeps unknown amounts distinct from a known zero and preserves small costs', () => {
    expect(formatCost(null, 'en')).toBe('—')
    expect(formatCost(undefined, 'en')).toBe('—')
    expect(formatCost(0, 'en')).toBe('$0.00')
    expect(formatCost(0.000001, 'en')).toBe('$0.000001')
  })

  it('marks pending tasks delayed strictly after two hours and preserves retry/terminal precedence', () => {
    const now = Date.parse('2026-09-15T03:00:00Z')
    expect(displayStatus(item(), now)).toBe('in_progress')
    expect(displayStatus(item(), now + 1)).toBe('delayed')
    for (const status of ['query_retry', 'review_required', 'billing_pending', 'completed', 'failed']) {
      expect(displayStatus(item({ display_status: status }), now + 1)).toBe(status)
    }
    expect(displayStatus(item({ kind: 'submission', task_id: '' }), now + 1)).toBe('submission_unknown')
  })

  it('translates only allowed reason codes in both languages', () => {
    expect(reasonKey('')).toBeNull()
    expect(reasonKey('https://secret.example/token?key=secret')).toBe('admin.videoTasks.reasons.unknown')
    for (const code of ['poll_failed', 'poll_timeout', 'upstream_failed', 'upstream_expired', 'completion_save_failed', 'billing_failed', 'upstream_cancelled', 'poll_not_found', 'poll_exhausted', 'account_route_changed']) {
      expect(reasonKey(code)).toBe(`admin.videoTasks.reasons.${code}`)
      expect(enAdmin.videoTasks.reasons).toHaveProperty(code)
      expect(zhAdmin.videoTasks.reasons).toHaveProperty(code)
    }
  })

  it('derives elapsed from completion or last update and leaves invalid facts unknown', () => {
    expect(elapsedParts(item())).toEqual({ hours: 0, minutes: 5, seconds: 0 })
    expect(elapsedParts(item({ completed_at: '2026-09-15T03:01:02Z' }))).toEqual({ hours: 2, minutes: 1, seconds: 2 })
    expect(elapsedParts(item({ updated_at: '' }))).toBeNull()
    expect(elapsedParts(item({ updated_at: '2026-09-14T00:00:00Z' }))).toBeNull()
  })
})

describe('VideoTasksView', () => {
  it.each(['en', 'zh'])('renders all statuses, coverage and missing costs in %s', async locale => {
    list.mockResolvedValue(response(videoTaskStatuses.map((status, index) => item({
      task_id: `video-${index}`, display_status: status, state: status,
      kind: status === 'submission_unknown' ? 'submission' : 'task'
    }))))
    mountView(locale)
    await flushPromises()
    const messages = locale === 'en' ? enAdmin.videoTasks : zhAdmin.videoTasks
    for (const status of videoTaskStatuses) expect(wrapper.text()).toContain(messages.statuses[status])
    expect(wrapper.text()).toContain(messages.coverage)
    expect(wrapper.text()).toContain(messages.billingNote)
    expect(wrapper.text()).not.toContain('$0.00')
    expect(wrapper.text()).not.toContain('admin.videoTasks.')
  })

  it('loads the seven-day window and shows the empty state', async () => {
    mountView()
    await flushPromises()
    expect(list).toHaveBeenCalledWith({ days: 7, page: 1, page_size: 20 }, expect.any(AbortSignal))
    expect(wrapper.text()).toContain(enAdmin.videoTasks.empty)
  })

  it('applies literal filters, resets pagination and refreshes the applied query', async () => {
    list.mockResolvedValue(response([item()], 1, 101))
    mountView()
    await flushPromises()
    wrapper.findComponent(Pagination).vm.$emit('update:page', 2)
    await flushPromises()
    expect(list.mock.lastCall?.[0].page).toBe(2)
    await wrapper.get('#video-task-search').setValue(' %_task ')
    select('video-task-status').vm.$emit('update:modelValue', 'query_retry')
    select('video-task-days').vm.$emit('update:modelValue', 30)
    await wrapper.get('form').trigger('submit')
    await flushPromises()
    expect(list.mock.lastCall?.[0]).toEqual({ page: 1, page_size: 20, search: ' %_task ', status: 'query_retry', days: 30 })
    await wrapper.get('#video-task-search').setValue('unapplied draft')
    await refresh()
    await flushPromises()
    expect(list.mock.lastCall?.[0].search).toBe(' %_task ')
    expect(select('video-task-page-size').props('options').map((option: { value: number }) => option.value)).toEqual([20, 50, 100])
    expect(wrapper.findComponent(Pagination).props('showPageSizeSelector')).toBe(false)
    select('video-task-page-size').vm.$emit('update:modelValue', 100)
    await flushPromises()
    expect(list.mock.lastCall?.[0]).toMatchObject({ page: 1, page_size: 100 })
  })

  it('renders unknown submissions distinctly, with review status and safe local details', async () => {
    const submission = item({ kind: 'submission', task_id: '', user_id: null, model: null, user_email: '',
      display_status: 'submission_unknown', billing_status: 'review', reason_code: 'secret raw error',
      account_id: null, account_name: '', group_id: null, group_name: '', next_attempt_at: null,
      ...({ prompt: 'PRIVATE PROMPT', url: 'https://private.invalid/video', api_key: 'PRIVATE TOKEN' } as object)
    })
    list.mockResolvedValue(response([submission, { ...submission }]))
    mountView()
    await flushPromises()
    const rows = wrapper.findComponent(DataTable).props('data')
    expect(rows[0]._rowKey).not.toBe(rows[1]._rowKey)
    expect(wrapper.text()).toContain(enAdmin.videoTasks.explanations.submission_unknown)
    expect(wrapper.text()).toContain('Needs review')
    await wrapper.findAll('button').find(button => button.text() === 'Task details')!.trigger('click')
    expect(list).toHaveBeenCalledTimes(1)
    expect(wrapper.get('[role="dialog"]').text()).toContain('Next check')
    expect(wrapper.get('[role="dialog"]').text()).toContain('Elapsed until completion / last update')
    for (const secret of ['secret raw error', 'PRIVATE PROMPT', 'private.invalid', 'PRIVATE TOKEN']) {
      expect(wrapper.html()).not.toContain(secret)
    }
    const buttons = wrapper.findAll('button').map(button => button.text()).join(' ')
    expect(buttons).not.toMatch(/refund|delete|generate|resubmit/i)
  })

  it('keeps a failed task with a recorded charge visible for review', async () => {
    list.mockResolvedValue(response([item({ display_status: 'failed', state: 'failed', billing_status: 'review', charged_cost: 1.25, usage_log_id: 25, reason_code: 'upstream_failed' })]))
    mountView()
    await flushPromises()
    expect(wrapper.text()).toContain('$1.25')
    expect(wrapper.text()).toContain('Needs review')
    expect(wrapper.text()).toContain(enAdmin.videoTasks.reasons.upstream_failed)
  })

  it('shows a safe load error, clears stale rows, and recovers through read-only refresh', async () => {
    list.mockResolvedValueOnce(response([item()])).mockRejectedValueOnce(new Error('PRIVATE SERVER ERROR')).mockResolvedValue(response())
    mountView()
    await flushPromises()
    await refresh()
    await flushPromises()
    expect(wrapper.get('[role="alert"]').text()).toBe(enAdmin.videoTasks.loadFailed)
    expect(wrapper.text()).not.toContain('PRIVATE SERVER ERROR')
    expect(wrapper.text()).not.toContain('video-1')
    await refresh()
    await flushPromises()
    expect(wrapper.find('[role="alert"]').exists()).toBe(false)
    expect(wrapper.text()).toContain(enAdmin.videoTasks.empty)
  })

  it('ignores late responses and cancels superseded or unmounted requests', async () => {
    let resolveOld!: (value: ReturnType<typeof response>) => void
    list.mockReturnValueOnce(new Promise(resolve => { resolveOld = resolve })).mockResolvedValue(response([item({ task_id: 'new-result' })]))
    mountView()
    const oldSignal = list.mock.calls[0][1] as AbortSignal
    select('video-task-status').vm.$emit('update:modelValue', 'failed')
    select('video-task-status').vm.$emit('change', 'failed')
    await flushPromises()
    expect(oldSignal.aborted).toBe(true)
    resolveOld(response([item({ task_id: 'old-result' })]))
    await flushPromises()
    expect(wrapper.text()).toContain('new-result')
    expect(wrapper.text()).not.toContain('old-result')
    const signal = list.mock.lastCall?.[1] as AbortSignal
    wrapper.unmount()
    expect(signal.aborted).toBe(true)
  })
})
