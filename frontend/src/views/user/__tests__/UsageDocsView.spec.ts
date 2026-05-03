import { afterEach, describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import UsageDocsView from '../UsageDocsView.vue'

vi.mock('@/i18n', () => ({
  i18n: {
    global: {
      t: (key: string) => key,
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
    t: (key: string) =>
      ({
        'usageDocs.title': '使用文档',
        'usageDocs.textToImageUnsupported': '文生图暂不支持',
      })[key] || key,
  }),
}))

describe('UsageDocsView', () => {
  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('contains the required static documentation sections', () => {
    const wrapper = mount(UsageDocsView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })

    const text = wrapper.text()
    expect(text).toContain('快速开始')
    expect(text).toContain('充值与套餐')
    expect(text).toContain('API Key 管理')
    expect(text).toContain('渠道与模型')
    expect(text).toContain('通用配置步骤')
    expect(text).toContain('CLI 配置')
    expect(text).toContain('Curl 调用示例')
    expect(text).toContain('第三方工具配置')
    expect(text).toContain('常见问题')
    expect(text).toContain('Claude Code')
    expect(text).toContain('Codex')
    expect(text).toContain('Gemini')
    expect(text).toContain('Curl')
    expect(text).toContain('OpenCode')
    expect(text).toContain('Kilocode')
    expect(text).toContain('Zed')
    expect(text).toContain('Hermes Agent')
    expect(text).toContain('WSL')
    expect(text).toContain('文生图暂不支持')
  })

  it('uses a polished documentation layout with hero, sticky toc, panels, and tool grid', () => {
    const wrapper = mount(UsageDocsView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })

    expect(wrapper.find('.docs-hero').exists()).toBe(true)
    expect(wrapper.find('.docs-toc-card').exists()).toBe(true)
    expect(wrapper.find('.docs-config-grid').exists()).toBe(true)
    expect(wrapper.findAll('.docs-panel').length).toBeGreaterThanOrEqual(4)
    expect(wrapper.find('.docs-tool-grid').exists()).toBe(true)
  })

  it('copies code blocks with clipboard API when available', async () => {
    const writeText = vi.fn().mockResolvedValue(undefined)
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText },
    })
    const wrapper = mount(UsageDocsView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })

    await wrapper.get('[data-testid="copy-code-button"]').trigger('click')

    expect(writeText).toHaveBeenCalledWith(expect.stringContaining('/v1'))
  })

  it('falls back to a hidden textarea when clipboard API rejects', async () => {
    Object.defineProperty(navigator, 'clipboard', {
      configurable: true,
      value: { writeText: vi.fn().mockRejectedValue(new Error('denied')) },
    })
    Object.defineProperty(document, 'execCommand', {
      configurable: true,
      value: vi.fn().mockReturnValue(true),
    })
    const execCommand = vi.spyOn(document, 'execCommand').mockReturnValue(true)
    const wrapper = mount(UsageDocsView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })

    await wrapper.get('[data-testid="copy-code-button"]').trigger('click')

    expect(execCommand).toHaveBeenCalledWith('copy')
  })

  it('labels each copy button with the code block title', () => {
    const wrapper = mount(UsageDocsView, {
      global: { stubs: { AppLayout: { template: '<div><slot /></div>' } } },
    })

    const labels = wrapper.findAll('[data-testid="copy-code-button"]').map((button) => button.attributes('aria-label'))

    expect(labels).toContain('复制 Base URL')
    expect(labels).toContain('复制 API Key')
    expect(labels).toContain('复制 Model')
  })
})
