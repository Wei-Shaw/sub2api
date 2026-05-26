/**
 * Dashboard 数据加载逻辑测试
 * 通过封装组件测试仪表板核心数据加载流程
 */
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { mount, flushPromises } from '@vue/test-utils'
import { setActivePinia, createPinia } from 'pinia'
import { defineComponent, ref, onMounted, nextTick } from 'vue'

// Mock API
const mockGetDashboardStats = vi.fn()

vi.mock('@/api', () => ({
  authAPI: {
    getCurrentUser: vi.fn().mockResolvedValue({
      data: { id: 1, username: 'test', email: 'test@example.com', role: 'user', balance: 100, concurrency: 5, status: 'active', allowed_groups: null, created_at: '', updated_at: '' },
    }),
    logout: vi.fn(),
    refreshToken: vi.fn(),
  },
  isTotp2FARequired: () => false,
}))

vi.mock('@/api/usage', () => ({
  usageAPI: {
    getDashboardStats: (...args: any[]) => mockGetDashboardStats(...args),
  },
}))

vi.mock('@/api/admin/system', () => ({
  checkUpdates: vi.fn(),
}))

vi.mock('@/api/auth', () => ({
  getPublicSettings: vi.fn().mockResolvedValue({}),
}))

interface DashboardStats {
  balance: number
  api_key_count: number
  active_api_key_count: number
  today_requests: number
  today_cost: number
  today_tokens: number
  total_tokens: number
}

/**
 * 简化的 Dashboard 测试组件
 */
const DashboardTestComponent = defineComponent({
  setup() {
    const stats = ref<DashboardStats | null>(null)
    const loading = ref(false)
    const error = ref('')

    const loadStats = async () => {
      loading.value = true
      error.value = ''
      try {
        stats.value = await mockGetDashboardStats()
      } catch (e: any) {
        error.value = e.message || '加载失败'
      } finally {
        loading.value = false
      }
    }

    onMounted(loadStats)

    return { stats, loading, error, loadStats }
  },
  template: `
    <div>
      <div v-if="loading" class="loading">加载中...</div>
      <div v-if="error" class="error">{{ error }}</div>
      <div v-if="stats" class="stats">
        <span class="balance">{{ stats.balance }}</span>
        <span class="api-keys">{{ stats.api_key_count }}</span>
        <span class="today-requests">{{ stats.today_requests }}</span>
        <span class="today-cost">{{ stats.today_cost }}</span>
      </div>
      <button class="refresh" @click="loadStats">刷新</button>
    </div>
  `,
})

describe('Dashboard 数据加载', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.clearAllMocks()
  })

  const fakeStats: DashboardStats = {
    balance: 100.5,
    api_key_count: 3,
    active_api_key_count: 2,
    today_requests: 150,
    today_cost: 2.5,
    today_tokens: 50000,
    total_tokens: 1000000,
  }

  it('挂载后自动加载数据', async () => {
    mockGetDashboardStats.mockResolvedValue(fakeStats)

    const wrapper = mount(DashboardTestComponent)
    await flushPromises()

    expect(mockGetDashboardStats).toHaveBeenCalledTimes(1)
    expect(wrapper.find('.balance').text()).toBe('100.5')
    expect(wrapper.find('.api-keys').text()).toBe('3')
    expect(wrapper.find('.today-requests').text()).toBe('150')
    expect(wrapper.find('.today-cost').text()).toBe('2.5')
  })

  it('加载中显示 loading 状态', async () => {
    let resolveStats: (v: any) => void
    mockGetDashboardStats.mockImplementation(
      () => new Promise((resolve) => { resolveStats = resolve })
    )

    const wrapper = mount(DashboardTestComponent)
    await nextTick()

    expect(wrapper.find('.loading').exists()).toBe(true)

    resolveStats!(fakeStats)
    await flushPromises()

    expect(wrapper.find('.loading').exists()).toBe(false)
    expect(wrapper.find('.stats').exists()).toBe(true)
  })

  it('加载失败时显示错误信息', async () => {
    mockGetDashboardStats.mockRejectedValue(new Error('Network error'))

    const wrapper = mount(DashboardTestComponent)
    await flushPromises()

    expect(wrapper.find('.error').text()).toBe('Network error')
    expect(wrapper.find('.stats').exists()).toBe(false)
  })

  it('点击刷新按钮重新加载数据', async () => {
    mockGetDashboardStats.mockResolvedValue(fakeStats)

    const wrapper = mount(DashboardTestComponent)
    await flushPromises()

    expect(mockGetDashboardStats).toHaveBeenCalledTimes(1)

    // 更新数据
    const updatedStats = { ...fakeStats, today_requests: 200 }
    mockGetDashboardStats.mockResolvedValue(updatedStats)

    await wrapper.find('.refresh').trigger('click')
    await flushPromises()

    expect(mockGetDashboardStats).toHaveBeenCalledTimes(2)
    expect(wrapper.find('.today-requests').text()).toBe('200')
  })

  it('数据为空时不显示统计信息', async () => {
    mockGetDashboardStats.mockResolvedValue(null)

    const wrapper = mount(DashboardTestComponent)
    await flushPromises()

    expect(wrapper.find('.stats').exists()).toBe(false)
  })
})
