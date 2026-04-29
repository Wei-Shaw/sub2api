import { describe, it, expect, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import PathChevron from './PathChevron.vue'

vi.mock('vue-i18n', async () => {
  const actual = await vi.importActual<typeof import('vue-i18n')>('vue-i18n')
  return {
    ...actual,
    useI18n: () => ({
      t: (key: string, params?: Record<string, unknown>) => (params ? `${key}:${JSON.stringify(params)}` : key),
    }),
  }
})

// 全局 i18n mock：formatPlatformLabel 内部调用 i18n.global.t
vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string, fallback?: string) => fallback || key,
    },
  },
}))

// admin API mock：useEntityName 调用 channels/groups/accounts/users.getById；
// 测试不联网，统一返回 reject 让占位 ch#id / g#id / acc#id / #id 保留。
vi.mock('@/api/admin', () => ({
  default: {
    channels: { getById: () => Promise.reject(new Error('mocked')) },
    groups: { getById: () => Promise.reject(new Error('mocked')) },
    accounts: { getById: () => Promise.reject(new Error('mocked')) },
    users: { getById: () => Promise.reject(new Error('mocked')) },
  },
}))

const stubs = {
  PlatformIcon: { template: '<span data-test="platform-icon" />' },
  Icon: { template: '<span data-test="chevron" />' },
}

describe('PathChevron', () => {
  it('全部 nil → 渲染 allRequests 文案', () => {
    const wrapper = mount(PathChevron, {
      props: { summary: null, showInternal: true },
      global: { stubs },
    })
    expect(wrapper.text()).toContain('admin.serviceQuota.scopeDetails.allRequests')
  })

  it('admin 视角：渲染 platform chip + 4 个 chevron 分隔', () => {
    const wrapper = mount(PathChevron, {
      props: {
        summary: { platform: 'openai', channel_id: 1, group_id: 2, account_id: 3, model_pattern: 'gpt-*' },
        showInternal: true,
      },
      global: { stubs },
    })
    expect(wrapper.findAll('[data-test="platform-icon"]').length).toBe(1)
    // 4 个 chevron（5 段中 4 个分隔点）
    expect(wrapper.findAll('[data-test="chevron"]').length).toBe(4)
    expect(wrapper.text()).toContain('ch#1')
    expect(wrapper.text()).toContain('g#2')
    expect(wrapper.text()).toContain('acc#3')
    expect(wrapper.text()).toContain('gpt-*')
  })

  it('admin 视角：缺失维度用 * 占位', () => {
    const wrapper = mount(PathChevron, {
      props: {
        summary: { platform: 'openai', channel_id: null, group_id: null, account_id: null, model_pattern: null },
        showInternal: true,
      },
      global: { stubs },
    })
    expect(wrapper.findAll('[data-test="platform-icon"]').length).toBe(1)
    // 4 个 * 占位（channel/group/account/model_pattern 都缺）
    expect((wrapper.text().match(/\*/g) || []).length).toBeGreaterThanOrEqual(4)
  })

  it('用户视角 (showInternal=false)：渲染 platform + model 但隐藏 channel/group/account', () => {
    const wrapper = mount(PathChevron, {
      props: {
        summary: { platform: 'openai', channel_id: 99, group_id: 88, account_id: 77, model_pattern: 'gpt-*' },
        showInternal: false,
      },
      global: { stubs },
    })
    expect(wrapper.findAll('[data-test="platform-icon"]').length).toBe(1)
    expect(wrapper.text()).not.toContain('ch#99')
    expect(wrapper.text()).not.toContain('g#88')
    expect(wrapper.text()).not.toContain('acc#77')
    // model_pattern 仍然展示给用户
    expect(wrapper.text()).toContain('gpt-*')
    // 只剩 platform → model 一个 chevron 分隔
    expect(wrapper.findAll('[data-test="chevron"]').length).toBe(1)
  })

  it('用户视角无 model_pattern：渲染 platform > * （model 段为占位）', () => {
    const wrapper = mount(PathChevron, {
      props: {
        summary: { platform: 'openai', channel_id: 99, group_id: 88, account_id: 77 },
        showInternal: false,
      },
      global: { stubs },
    })
    expect(wrapper.findAll('[data-test="platform-icon"]').length).toBe(1)
    // 仍有一个 chevron（platform → model 占位 *）
    expect(wrapper.findAll('[data-test="chevron"]').length).toBe(1)
  })
})
