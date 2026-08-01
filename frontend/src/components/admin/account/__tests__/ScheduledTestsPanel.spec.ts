import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ScheduledTestsPanel from '../ScheduledTestsPanel.vue'

const api = vi.hoisted(() => ({
  listByAccount: vi.fn().mockResolvedValue([]),
  create: vi.fn().mockResolvedValue({}),
  update: vi.fn(),
  delete: vi.fn(),
  listResults: vi.fn()
}))

vi.mock('@/api/admin', () => ({ adminAPI: { scheduledTests: api } }))
vi.mock('@/stores/app', () => ({
  useAppStore: () => ({ showSuccess: vi.fn(), showError: vi.fn() })
}))
vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: { minutes?: number }) => params?.minutes ? `${key}:${params.minutes}` : key })
  }
})

const stubs = {
  BaseDialog: { props: ['show'], template: '<div v-if="show"><slot /></div>' },
  ConfirmDialog: true,
  HelpTooltip: { template: '<span><slot name="trigger" /><slot /></span>' },
  Select: {
    props: ['modelValue', 'options', 'placeholder', 'searchable'],
    emits: ['update:modelValue'],
    template: '<select :value="modelValue" @change="$emit(\'update:modelValue\', $event.target.value)"><option value=""></option><option value="gpt-5.6-luna">luna</option></select>'
  },
  ModelMultiSelect: {
    props: ['modelValue', 'options', 'placeholder'],
    emits: ['update:modelValue'],
    template: '<div data-testid="model-multi-select"><span>{{ modelValue.join(",") }}</span><button data-testid="select-sol-only" @click="$emit(\'update:modelValue\', [\'gpt-5.6-sol\'])">sol only</button></div>'
  },
  Input: {
    props: ['modelValue', 'type', 'min', 'max', 'placeholder', 'hint'],
    emits: ['update:modelValue'],
    template: '<input :type="type || \'text\'" :value="modelValue" @input="$emit(\'update:modelValue\', $event.target.value)" />'
  },
  Toggle: {
    props: ['modelValue'],
    emits: ['update:modelValue'],
    template: '<input type="checkbox" :checked="modelValue" @change="$emit(\'update:modelValue\', $event.target.checked)" />'
  },
  Icon: true
}

function mountPanel() {
  return mount(ScheduledTestsPanel, {
    props: {
      show: true,
      accountId: 42,
      modelOptions: [
        { value: 'gpt-5.6-luna', label: 'luna' },
        { value: 'gpt-5.6-sol', label: 'sol' }
      ]
    },
    global: { stubs }
  })
}

async function openForm(wrapper: ReturnType<typeof mountPanel>) {
  const addButton = wrapper.findAll('button').find(button => button.text().includes('admin.scheduledTests.addPlan'))
  await addButton!.trigger('click')
}

async function openScheduledFormAndSelectModel(wrapper: ReturnType<typeof mountPanel>) {
  await openForm(wrapper)
  const scheduledButton = wrapper.findAll('button').find(button => button.text() === 'admin.scheduledTests.scheduledMode')
  await scheduledButton!.trigger('click')
  await wrapper.find('select').setValue('gpt-5.6-luna')
}

async function save(wrapper: ReturnType<typeof mountPanel>) {
  const saveButton = wrapper.findAll('button').find(button => button.text() === 'common.save')
  await saveButton!.trigger('click')
  await flushPromises()
}

beforeEach(() => {
  api.create.mockClear()
  api.update.mockClear()
  api.listByAccount.mockClear()
  api.listByAccount.mockResolvedValue([])
  api.listResults.mockReset()
})

const runningResult = (startedAt = new Date(Date.now() - 1000).toISOString()) => ({
  id: 1,
  plan_id: 9,
  model_id: 'gpt-5.6-sol',
  status: 'running',
  response_text: '',
  error_message: '',
  latency_ms: 0,
  started_at: startedAt,
  finished_at: startedAt,
  created_at: startedAt
})

async function openResults(results: object[], refreshResults?: object[]) {
  const plan = {
    id: 9,
    account_id: 42,
    model_id: 'gpt-5.6-sol',
    model_ids: [],
    cron_expression: '*/30 * * * *',
    trigger_mode: 'error_recovery' as const,
    retry_interval_minutes: 5,
    retry_cron_expression: null,
    enabled: true,
    max_results: 100,
    auto_recover: true,
    last_run_at: null,
    next_run_at: null,
    created_at: '2026-08-01T00:00:00Z',
    updated_at: '2026-08-01T00:00:00Z'
  }
  api.listByAccount.mockResolvedValue([plan])
  api.listResults.mockResolvedValueOnce(results)
  if (refreshResults) api.listResults.mockResolvedValueOnce(refreshResults)
  const wrapper = mountPanel()
  await wrapper.setProps({ show: false })
  await wrapper.setProps({ show: true })
  await flushPromises()
  await wrapper.find('[class*="cursor-pointer"][class*="justify-between"]').trigger('click')
  await flushPromises()
  return wrapper
}

describe('ScheduledTestsPanel error recovery form', () => {
  it('defaults to a five-minute error recovery plan', async () => {
    const wrapper = mountPanel()
    await openForm(wrapper)
    expect(wrapper.get('[data-testid="model-multi-select"]').text()).toContain('gpt-5.6-luna,gpt-5.6-sol')
    const autoRecover = wrapper.findAll('label').find(label => label.text().includes('admin.scheduledTests.autoRecover'))
    expect((autoRecover!.find('input').element as HTMLInputElement).checked).toBe(true)
    await save(wrapper)

    expect(api.create).toHaveBeenCalledWith({
      account_id: 42,
      model_id: 'gpt-5.6-luna',
      model_ids: [],
      enabled: true,
      max_results: 100,
      trigger_mode: 'error_recovery',
      retry_interval_minutes: 5,
      retry_cron_expression: null,
      auto_recover: true
    })
  })

  it('uses advanced cron instead of the minute interval', async () => {
    const wrapper = mountPanel()
    await openForm(wrapper)
    const advancedLabel = wrapper.findAll('label').find(label => label.text().includes('admin.scheduledTests.advancedCron'))
    await advancedLabel!.find('input').setValue(true)
    const cronInput = wrapper.findAll('input').find(input => input.element.type === 'text')
    await cronInput!.setValue('*/7 * * * *')
    await save(wrapper)

    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({
      trigger_mode: 'error_recovery',
      retry_interval_minutes: null,
      retry_cron_expression: '*/7 * * * *'
    }))
  })

  it('preserves an explicitly disabled auto-recovery setting on create', async () => {
    const wrapper = mountPanel()
    await openForm(wrapper)
    const autoRecover = wrapper.findAll('label').find(label => label.text().includes('admin.scheduledTests.autoRecover'))
    await autoRecover!.find('input').setValue(false)
    await save(wrapper)

    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({
      trigger_mode: 'error_recovery',
      auto_recover: false
    }))
  })

  it('preserves the existing scheduled cron payload', async () => {
    const wrapper = mountPanel()
    await openScheduledFormAndSelectModel(wrapper)
    await save(wrapper)

    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({
      trigger_mode: 'scheduled',
      cron_expression: '*/30 * * * *',
      retry_interval_minutes: null,
      retry_cron_expression: null
    }))
  })

  it('sends only the selected failed-model subset', async () => {
    const wrapper = mountPanel()
    await openForm(wrapper)
    await wrapper.get('[data-testid="select-sol-only"]').trigger('click')
    await save(wrapper)

    expect(api.create).toHaveBeenCalledWith(expect.objectContaining({
      model_id: 'gpt-5.6-sol',
      model_ids: ['gpt-5.6-sol']
    }))
  })

  it('explains that error recovery probes the models that actually failed', async () => {
    const wrapper = mountPanel()
    await openForm(wrapper)
    expect(wrapper.text()).toContain('admin.scheduledTests.errorRecoveryTooltipFailedModels')
    expect(wrapper.get('button[aria-label="admin.scheduledTests.errorRecoveryHelpAriaLabel"]').exists()).toBe(true)
  })

  it('loads and edits a legacy scheduled plan without recovery fields', async () => {
    const legacyPlan = {
      id: 7,
      account_id: 42,
      model_id: 'gpt-5.6-luna',
      cron_expression: '*/17 * * * *',
      enabled: true,
      max_results: 100,
      auto_recover: false,
      last_run_at: null,
      next_run_at: null,
      created_at: '2026-08-01T00:00:00Z',
      updated_at: '2026-08-01T00:00:00Z'
    }
    api.listByAccount.mockResolvedValue([legacyPlan])
    api.update.mockResolvedValueOnce({ ...legacyPlan, trigger_mode: 'scheduled' })
    const wrapper = mountPanel()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.attributes('title') === 'admin.scheduledTests.editPlan')
    await editButton!.trigger('click')
    expect(wrapper.find('input[value="*/17 * * * *"]').exists()).toBe(true)
    await save(wrapper)

    expect(api.update).toHaveBeenCalledWith(7, expect.objectContaining({
      model_id: 'gpt-5.6-luna',
      trigger_mode: 'scheduled',
      cron_expression: '*/17 * * * *',
      retry_interval_minutes: null,
      retry_cron_expression: null
    }))
  })

  it('preserves an explicitly disabled auto-recovery setting on edit', async () => {
    const plan = {
      id: 8,
      account_id: 42,
      model_id: 'gpt-5.6-luna',
      model_ids: [],
      cron_expression: '*/30 * * * *',
      trigger_mode: 'error_recovery',
      retry_interval_minutes: 5,
      retry_cron_expression: null,
      enabled: true,
      max_results: 100,
      auto_recover: true,
      last_run_at: null,
      next_run_at: null,
      created_at: '2026-08-01T00:00:00Z',
      updated_at: '2026-08-01T00:00:00Z'
    }
    api.listByAccount.mockResolvedValue([plan])
    api.update.mockResolvedValueOnce(plan)
    const wrapper = mountPanel()
    await wrapper.setProps({ show: false })
    await wrapper.setProps({ show: true })
    await flushPromises()

    const editButton = wrapper.findAll('button').find(button => button.attributes('title') === 'admin.scheduledTests.editPlan')
    await editButton!.trigger('click')
    const autoRecover = wrapper.findAll('label').find(label => label.text().includes('admin.scheduledTests.autoRecover'))
    await autoRecover!.find('input').setValue(false)
    await save(wrapper)

    expect(api.update).toHaveBeenCalledWith(8, expect.objectContaining({
      trigger_mode: 'error_recovery',
      auto_recover: false
    }))
  })
})

describe('ScheduledTestsPanel running results', () => {
  it('refreshes expanded results while a test is running', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = await openResults([runningResult()], [{ ...runningResult(), status: 'success' }])

      expect(api.listResults).toHaveBeenCalledTimes(1)
      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()

      expect(api.listResults).toHaveBeenCalledTimes(2)
      expect(wrapper.text()).toContain('admin.scheduledTests.success')
    } finally {
      vi.useRealTimers()
    }
  })

  it('marks a stale running result as interrupted after the runner timeout', async () => {
    const wrapper = await openResults([runningResult(new Date(Date.now() - 6 * 60 * 1000).toISOString())])

    expect(wrapper.text()).toContain('admin.scheduledTests.interrupted')
  })

  it('changes a visible running result to interrupted when it crosses the timeout', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = await openResults([
        runningResult(new Date(Date.now() - 5 * 60 * 1000 + 1000).toISOString())
      ])

      expect(wrapper.text()).toContain('admin.scheduledTests.running')
      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()

      expect(wrapper.text()).toContain('admin.scheduledTests.interrupted')
    } finally {
      vi.useRealTimers()
    }
  })

  it('does not overlap result polls while the previous request is pending', async () => {
    vi.useFakeTimers()
    try {
      let resolvePoll!: (results: object[]) => void
      const pendingPoll = new Promise<object[]>((resolve) => { resolvePoll = resolve })
      const wrapper = await openResults([runningResult()])
      api.listResults.mockReturnValueOnce(pendingPoll)

      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()

      expect(api.listResults).toHaveBeenCalledTimes(2)
      resolvePoll([{ ...runningResult(), status: 'success' }])
      await flushPromises()
      expect(wrapper.text()).toContain('admin.scheduledTests.success')
    } finally {
      vi.useRealTimers()
    }
  })

  it('ignores an old poll response after the same plan is closed and reopened', async () => {
    vi.useFakeTimers()
    try {
      let resolveOldPoll!: (results: object[]) => void
      const oldPoll = new Promise<object[]>((resolve) => { resolveOldPoll = resolve })
      const wrapper = await openResults([runningResult()])
      api.listResults.mockReturnValueOnce(oldPoll)

      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()
      const planHeader = wrapper.find('[class*="cursor-pointer"][class*="justify-between"]')
      await planHeader.trigger('click')
      api.listResults.mockResolvedValueOnce([{ ...runningResult(), status: 'success' }])
      await planHeader.trigger('click')
      await flushPromises()
      expect(wrapper.text()).toContain('admin.scheduledTests.success')

      resolveOldPoll([runningResult()])
      await flushPromises()

      expect(wrapper.text()).toContain('admin.scheduledTests.success')
      expect(wrapper.text()).not.toContain('admin.scheduledTests.running')
    } finally {
      vi.useRealTimers()
    }
  })

  it('stops refreshing when a running-result poll fails', async () => {
    vi.useFakeTimers()
    try {
      const wrapper = await openResults([runningResult()])
      api.listResults.mockRejectedValueOnce(new Error('network unavailable'))

      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()
      await vi.advanceTimersByTimeAsync(5000)
      await flushPromises()

      expect(api.listResults).toHaveBeenCalledTimes(2)
      expect(wrapper.exists()).toBe(true)
    } finally {
      vi.useRealTimers()
    }
  })
})
