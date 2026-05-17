import { flushPromises, mount } from '@vue/test-utils'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import ChannelStatusView from '../ChannelStatusView.vue'

const listChannelMonitors = vi.hoisted(() => vi.fn())
const fetchChannelMonitorDetail = vi.hoisted(() => vi.fn())
const showError = vi.hoisted(() => vi.fn())
const fetchPublicSettings = vi.hoisted(() => vi.fn())
const appState = vi.hoisted(() => ({
  cachedPublicSettings: { channel_monitor_enabled: true } as null | Record<string, unknown>,
  publicSettingsLoaded: true,
  showError,
  fetchPublicSettings,
}))

vi.mock('@/api/channelMonitor', () => ({
  list: listChannelMonitors,
  status: fetchChannelMonitorDetail,
}))

vi.mock('@/components/layout/AppLayout.vue', () => ({
  default: { template: '<main><slot /></main>' },
}))

vi.mock('@/components/common/AutoRefreshButton.vue', () => ({
  default: { template: '<button />' },
}))

vi.mock('@/components/icons/Icon.vue', () => ({
  default: { template: '<span />' },
}))

vi.mock('@/stores/app', () => ({
  useAppStore: () => appState,
}))

vi.mock('@/stores', () => ({
  useAppStore: () => ({
    sidebarCollapsed: false,
    contactInfo: '',
    docUrl: '',
    cachedPublicSettings: appState.cachedPublicSettings,
    toggleMobileSidebar: vi.fn(),
  }),
  useAuthStore: () => ({
    isAuthenticated: false,
    user: null,
    isSimpleMode: false,
  }),
  useAdminSettingsStore: () => ({ customMenuItems: [] }),
  useOnboardingStore: () => ({ setReplayCallback: vi.fn(), replay: vi.fn() }),
}))

vi.mock('@/composables/useOnboardingTour', () => ({
  useOnboardingTour: () => ({ replayTour: vi.fn() }),
}))

vi.mock('@/utils/apiError', () => ({
  extractApiErrorMessage: (_err: unknown, fallback: string) => fallback,
}))

vi.mock('@/i18n', () => ({
  getLocale: () => 'zh',
  i18n: {
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  },
}))

vi.mock('vue-i18n', () => ({
  createI18n: () => ({
    global: {
      locale: { value: 'zh' },
      t: (key: string) => key,
      setLocaleMessage: vi.fn(),
    },
  }),
  useI18n: () => ({
    t: (key: string, params?: Record<string, unknown>) => {
      const dict: Record<string, string> = {
        'nav.channelStatus': '服务状态',
        'channelStatus.title': '服务状态',
        'channelStatus.description': '查看渠道可用性、延迟和近期状态',
        'channelStatus.disabled.title': '服务状态暂未公开',
        'channelStatus.disabled.description': '管理员关闭了渠道状态监控展示。',
        'channelStatus.empty.title': '暂无可显示的渠道',
        'channelStatus.empty.description': '管理员尚未配置可监控的渠道。',
        'channelStatus.summary.allNormal': '全部正常',
        'channelStatus.summary.partialUnavailable': '部分不可用',
        'channelStatus.summary.channelDown': '渠道宕机',
        'channelStatus.summary.normalCount': '正常 {n}',
        'channelStatus.summary.abnormalCount': '异常 {n}',
        'channelStatus.overall.operational': '渠道运行正常',
        'channelStatus.overall.degraded': '渠道部分异常',
        'channelStatus.overall.unavailable': '渠道不可用',
        'channelStatus.modelsTitle': '模型状态',
        'channelStatus.modelCount': '{n} 个模型',
        'channelStatus.global.operational': '所有渠道运行正常',
        'channelStatus.global.degraded': '部分服务异常',
        'channelStatus.global.unavailable': '服务暂不可用',
        'channelStatus.quickNav': '快速导航',
        'channelStatus.availability.7d': '7天',
        'channelStatus.availability.15d': '15天',
        'channelStatus.availability.30d': '30天',
        'channelStatus.latestLatency': '最新延迟',
        'channelStatus.columns.availability7d': '7 天可用率',
        'channelStatus.avgLatency7d': '7天平均',
        'monitorCommon.history60pts': '近 {n} 次记录',
        'monitorCommon.nextUpdateIn': '{n}s 后刷新',
        'monitorCommon.past': 'PAST',
        'monitorCommon.now': 'NOW',
        'monitorCommon.status.operational': '正常',
        'monitorCommon.status.failed': '失败',
        'monitorCommon.status.error': '错误',
        'monitorCommon.status.unknown': '-',
        'monitorCommon.providers.openai': 'OpenAI',
        'monitorCommon.latencyEmpty': '-',
        'monitorCommon.relativeSecondsAgo': '{n} 秒前',
        'monitorCommon.relativeMinutesAgo': '{n} 分钟前',
        'monitorCommon.relativeHoursAgo': '{n} 小时前',
        'monitorCommon.relativeDaysAgo': '{n} 天前',
        'common.refresh': '刷新',
      }
      let value = dict[key] || key
      Object.entries(params ?? {}).forEach(([k, v]) => {
        value = value.replace(`{${k}}`, String(v))
      })
      return value
    },
  }),
}))

function mountView() {
  return mount(ChannelStatusView, {
    global: {
      stubs: {
        Icon: { template: '<span />' },
        LocaleSwitcher: { template: '<button />' },
        AnnouncementBell: true,
        SubscriptionProgressMini: true,
        RouterLink: { template: '<a><slot /></a>' },
        AutoRefreshButton: true,
      },
    },
  })
}

describe('ChannelStatusView', () => {
  beforeEach(() => {
    vi.useRealTimers()
    appState.cachedPublicSettings = { channel_monitor_enabled: true }
    appState.publicSettingsLoaded = true
    showError.mockReset()
    fetchPublicSettings.mockReset()
    listChannelMonitors.mockReset()
    fetchChannelMonitorDetail.mockReset()
    listChannelMonitors.mockResolvedValue({ items: [] })
    fetchChannelMonitorDetail.mockResolvedValue({ models: [] })
  })

  it('renders flat channel groups, sorts abnormal channels first, and expands model rows by default', async () => {
    listChannelMonitors.mockResolvedValue({
      items: [
        {
          id: 1,
          name: 'OpenAI Pro 号池',
          provider: 'openai',
          group_name: 'default',
          primary_model: 'gpt-5.5',
          primary_status: 'operational',
          primary_latency_ms: 284,
          primary_ping_latency_ms: 42,
          availability_7d: 99.5,
          extra_models: [{ model: 'gpt-5.4', status: 'failed', latency_ms: null }],
          timeline: [],
        },
        {
          id: 2,
          name: 'Azure 美东备用',
          provider: 'openai',
          group_name: 'backup',
          primary_model: 'gpt-5.5',
          primary_status: 'failed',
          primary_latency_ms: null,
          primary_ping_latency_ms: null,
          availability_7d: 45,
          extra_models: [{ model: 'gpt-5.4', status: 'operational', latency_ms: 1200 }],
          timeline: [],
        },
      ],
    })
    fetchChannelMonitorDetail.mockImplementation((id: number) => {
      const common = {
        provider: 'openai',
        group_name: id === 1 ? 'default' : 'backup',
      }
      if (id === 2) {
        return Promise.resolve({
          id,
          name: 'Azure 美东备用',
          ...common,
          models: [
            {
              model: 'gpt-5.5',
              latest_status: 'failed',
              latest_latency_ms: null,
              availability_7d: 45,
              availability_15d: 50,
              availability_30d: 60,
              avg_latency_7d_ms: null,
              timeline: [
                {
                  status: 'failed',
                  latency_ms: null,
                  ping_latency_ms: null,
                  checked_at: new Date().toISOString(),
                },
              ],
            },
          ],
        })
      }
      return Promise.resolve({
        id,
        name: 'OpenAI Pro 号池',
        ...common,
        models: [
          {
            model: 'gpt-5.5',
            latest_status: 'operational',
            latest_latency_ms: 284,
            availability_7d: 99.5,
            availability_15d: 98.9,
            availability_30d: 97.8,
            avg_latency_7d_ms: 301,
            timeline: [
              {
                status: 'operational',
                latency_ms: 284,
                ping_latency_ms: 42,
                checked_at: new Date().toISOString(),
              },
            ],
          },
        ],
      })
    })

    const wrapper = mountView()
    await flushPromises()

    await flushPromises()

    expect(wrapper.find('section').classes()).toEqual(expect.arrayContaining(['w-full', 'max-w-[1500px]']))
    expect(wrapper.find('[data-testid="channel-status-header-grid"]').classes()).not.toContain('xl:grid-cols-[minmax(0,1fr)_180px]')
    expect(wrapper.find('[data-testid="channel-status-content-grid"]').classes()).not.toContain('xl:grid-cols-[minmax(0,1fr)_180px]')
    expect(wrapper.find('h1').text()).toBe('服务状态')
    expect(wrapper.find('h1').element.parentElement?.textContent).toContain('部分服务异常')
    expect(wrapper.find('h1').element.parentElement?.textContent).not.toContain('Azure 美东备用')
    expect(wrapper.find('h1').element.parentElement?.textContent).not.toContain('gpt-5.5')
    expect(wrapper.find('.channel-monitor-flat-list').exists()).toBe(true)
    expect(wrapper.find('.channel-monitor-accordion').exists()).toBe(false)
    expect(wrapper.find('.channel-monitor-global-alert').exists()).toBe(false)
    const headers = wrapper.findAll('.channel-monitor-section-header')
    expect(headers[0].text()).toContain('Azure 美东备用')
    expect(headers[0].text()).toContain('部分不可用')
    expect(headers[1].text()).toContain('OpenAI Pro 号池')
    expect(fetchChannelMonitorDetail).toHaveBeenCalledWith(1)
    expect(fetchChannelMonitorDetail).toHaveBeenCalledWith(2)

    expect(wrapper.find('.channel-monitor-model-row').text()).toContain('gpt-5.5')
    expect(wrapper.find('.channel-monitor-model-row').text()).toContain('7 天可用率')
    expect(wrapper.find('.channel-monitor-model-row').text()).toContain('45.00%')
    expect(wrapper.find('.channel-monitor-model-row').text()).not.toContain('50.00%')
    expect(wrapper.find('.channel-monitor-model-row').text()).not.toContain('60.00%')
    expect(wrapper.findComponent({ name: 'MonitorTimeline' }).exists()).toBe(true)
  })

  it('shows a sticky quick navigation when there are many channels', async () => {
    listChannelMonitors.mockResolvedValue({
      items: Array.from({ length: 11 }, (_, idx) => ({
        id: idx + 1,
        name: `渠道 ${idx + 1}`,
        provider: 'openai',
        group_name: 'default',
        primary_model: 'gpt-5.5',
        primary_status: 'operational',
        primary_latency_ms: 200,
        primary_ping_latency_ms: 40,
        availability_7d: 99,
        extra_models: [],
        timeline: [],
      })),
    })
    fetchChannelMonitorDetail.mockResolvedValue({
      models: [
        {
          model: 'gpt-5.5',
          latest_status: 'operational',
          latest_latency_ms: 200,
          availability_7d: 99,
          availability_15d: 99,
          availability_30d: 99,
          avg_latency_7d_ms: 200,
          timeline: [],
        },
      ],
    })

    const wrapper = mountView()
    await flushPromises()
    await flushPromises()

    expect(wrapper.find('.channel-monitor-toc').exists()).toBe(true)
    expect(wrapper.findAll('.channel-monitor-toc a')).toHaveLength(11)
    expect(wrapper.find('h1').element.parentElement?.textContent).toContain('所有渠道运行正常')
  })

  it('uses public settings switch before loading monitor data', async () => {
    appState.cachedPublicSettings = null
    appState.publicSettingsLoaded = false
    fetchPublicSettings.mockResolvedValue({ channel_monitor_enabled: false })

    const wrapper = mountView()
    await flushPromises()

    expect(fetchPublicSettings).toHaveBeenCalled()
    expect(listChannelMonitors).not.toHaveBeenCalled()
    expect(wrapper.text()).toContain('服务状态暂未公开')
  })
})
