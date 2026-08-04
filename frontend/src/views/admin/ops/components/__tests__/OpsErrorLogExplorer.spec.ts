import { beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent } from 'vue'
import { flushPromises, mount } from '@vue/test-utils'
import OpsErrorLogExplorer from '../OpsErrorLogExplorer.vue'
import enLocale from '@/i18n/locales/en'
import zhLocale from '@/i18n/locales/zh'

const mockListErrorLogs = vi.fn()
const mockShowError = vi.fn()

vi.mock('@/api/admin/ops', () => ({
  opsAPI: {
    listErrorLogs: (...args: any[]) => mockListErrorLogs(...args)
  }
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({ showError: mockShowError })
}))

vi.mock('vue-i18n', async (importOriginal) => {
  const actual = await importOriginal<typeof import('vue-i18n')>()
  return {
    ...actual,
    useI18n: () => ({ t: (key: string, params?: Record<string, unknown>) => params ? `${key}:${JSON.stringify(params)}` : key })
  }
})

const SelectStub = defineComponent({
  name: 'SelectControlStub',
  props: ['modelValue', 'options'],
  emits: ['update:modelValue'],
  template: '<div class="select-stub" />'
})

const ErrorTableStub = defineComponent({
  name: 'OpsErrorLogTable',
  props: ['rows', 'total', 'loading', 'page', 'pageSize'],
  emits: ['openErrorDetail', 'sort', 'update:page', 'update:pageSize'],
  template: '<button class="open-detail" @click="$emit(\'openErrorDetail\', rows[0]?.id)">open</button>'
})

const IconStub = defineComponent({
  name: 'Icon',
  template: '<span />'
})

function mountExplorer() {
  return mount(OpsErrorLogExplorer, {
    props: {
      platformFilter: 'openai',
      groupIdFilter: 7,
      refreshToken: 0
    },
    global: {
      stubs: {
        Select: SelectStub,
        OpsErrorLogTable: ErrorTableStub,
        Icon: IconStub
      }
    }
  })
}

describe('OpsErrorLogExplorer', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockListErrorLogs.mockResolvedValue({
      items: [{ id: 41, phase: 'upstream', status_code: 502, message: 'upstream failed' }],
      total: 1,
      page: 1,
      page_size: 20
    })
  })

  it('loads the same actionable error view used by the digest', async () => {
    mountExplorer()
    await flushPromises()

    expect(mockListErrorLogs).toHaveBeenCalledWith(expect.objectContaining({
      page: 1,
      page_size: 20,
      time_range: '24h',
      view: 'errors',
      platform: 'openai',
      group_id: 7,
      sort_by: 'created_at',
      sort_order: 'desc'
    }))
  })

  it('supports retained-history and resolution filters', async () => {
    const wrapper = mountExplorer()
    await flushPromises()

    const selects = wrapper.findAllComponents(SelectStub)
    selects[0].vm.$emit('update:modelValue', '30d')
    selects[5].vm.$emit('update:modelValue', 'true')
    await wrapper.find('input[type="search"]').setValue(' timeout ')
    await wrapper.findAll('button').find((button) => button.text().startsWith('common.search'))!.trigger('click')
    await flushPromises()

    expect(mockListErrorLogs).toHaveBeenLastCalledWith(expect.objectContaining({
      time_range: '30d',
      resolved: 'true',
      q: 'timeout'
    }))
  })

  it('opens upstream rows with the upstream detail endpoint', async () => {
    const wrapper = mountExplorer()
    await flushPromises()
    await wrapper.find('.open-detail').trigger('click')

    expect(wrapper.emitted('openErrorDetail')).toEqual([[41, 'upstream']])
  })

  it.each([
    ['zh', zhLocale],
    ['en', enLocale]
  ])('defines explorer translations for %s', (_name, locale) => {
    expect(locale.admin.ops.logViews.errors).toBeTruthy()
    expect(locale.admin.ops.logViews.system).toBeTruthy()
    expect(locale.admin.ops.errorExplorer.title).toBeTruthy()
    expect(locale.admin.ops.errorExplorer.loadFailed).toBeTruthy()
  })
})
