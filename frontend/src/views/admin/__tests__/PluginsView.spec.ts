import { enableAutoUnmount, flushPromises, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import PluginsView from '../PluginsView.vue'
import BaseDialog from '@/components/common/BaseDialog.vue'
import type { PluginUISession } from '@/api/admin'

enableAutoUnmount(afterEach)

const {
  listPlugins,
  uploadPlugin,
  enablePlugin,
  savePluginConfig,
  getPluginCatalog,
  createUISession,
  stepUpRun,
} = vi.hoisted(() => ({
  listPlugins: vi.fn(),
  uploadPlugin: vi.fn(),
  enablePlugin: vi.fn(),
  savePluginConfig: vi.fn(),
  getPluginCatalog: vi.fn(),
  createUISession: vi.fn(),
  stepUpRun: vi.fn((action: () => Promise<unknown>) => action()),
}))

vi.mock('@/api/admin', () => ({
  adminAPI: {
    plugins: {
      list: listPlugins,
      upload: uploadPlugin,
      enable: enablePlugin,
      disable: vi.fn(),
      remove: vi.fn(),
      getConfig: vi.fn().mockResolvedValue({}),
      getConfigRevision: vi.fn().mockResolvedValue({ config: { models: [] }, revision: '"' + 'a'.repeat(64) + '"' }),
      getCatalog: getPluginCatalog,
      saveConfig: savePluginConfig,
      test: vi.fn().mockResolvedValue({ success: true, message: 'ok', latency_ms: 1 }),
      createUISession,
    },
  },
}))

vi.mock('@/stores', () => ({
  useAuthStore: () => ({ user: { id: 19 } }),
  useAppStore: () => ({
    showError: vi.fn(),
    showSuccess: vi.fn(),
    showInfo: vi.fn(),
  }),
}))

vi.mock('@/composables/useStepUp', () => ({
  useStepUp: () => ({ run: stepUpRun }),
  isStepUpBlocked: () => false,
  isStepUpCancelled: () => false,
  stepUpBlockReason: () => '',
}))

vi.mock('vue-i18n', async (importOriginal) => ({
  ...(await importOriginal<typeof import('vue-i18n')>()),
  useI18n: () => ({ t: (key: string) => key }),
}))

const plugin = {
  id: 7,
  plugin_key: 'local.test.transport',
  name: 'Test Transport',
  version: '1.0.0',
  description: '',
  author: 'test',
  manifest: {
    schema_version: 1,
    id: 'local.test.transport',
    name: 'Test Transport',
    version: '1.0.0',
    requires: {
      sub2api: '>=0.1.0',
      plugin_protocol: 1,
      transport_api: 1,
      ui_bridge: 1,
    },
    capabilities: [],
    ui: { entrypoint: 'ui/index.html' },
  },
  binary_sha256: 'a'.repeat(64),
  signature_status: 'trusted' as const,
  state: 'disabled' as const,
  last_error: '',
  installed_at: '2026-08-22T00:00:00Z',
  updated_at: '2026-08-22T00:00:00Z',
  bindings: [
    {
      id: 1,
      plugin_id: 7,
      capability: 'openai.oauth.outbound_transport.v1',
      platform: 'openai',
      account_type: 'oauth',
      enabled: false,
      rollout_percent: 100,
    },
  ],
  compatibility: {
    compatible: true,
    tested: true,
    status: 'compatible' as const,
    message: '',
    current_sub2api_version: '0.1.0',
    required_sub2api_version: '>=0.1.0',
    recommended_sub2api_version: '0.1.0',
    plugin_protocol: 1,
    transport_api: 1,
    ui_bridge: 1,
  },
  runtime_healthy: false,
  runtime_message: '',
}

function mountView() {
  return mount(PluginsView, {
    attachTo: document.body,
    global: {
      stubs: {
        AppLayout: { template: '<div><slot /></div>' },
        BaseDialog: { template: '<div><slot /></div>' },
        Icon: true,
        TotpStepUpDialog: true,
      },
    },
  })
}

describe('管理员插件页二次验证', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    stepUpRun.mockImplementation((action: () => Promise<unknown>) => action())
    listPlugins.mockResolvedValue([plugin])
    uploadPlugin.mockResolvedValue(plugin)
    enablePlugin.mockResolvedValue(plugin)
    savePluginConfig.mockResolvedValue({ enabled: true })
    getPluginCatalog.mockResolvedValue({ group_id: 2, groups: [{ id: 2, name: 'Group' }], models: [] })
    createUISession.mockResolvedValue({
      url: '/api/v1/plugin-ui/token/index.html#bridge_token=bridge',
      bridge_token: 'bridge',
      ui_bridge_version: 1,
      expires_at: '2026-08-22T01:00:00Z',
    })
  })

  it('启用插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()

    const button = wrapper.findAll('button').find((item) => item.text().includes('admin.plugins.enable'))
    expect(button).toBeDefined()
    await button!.trigger('click')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(enablePlugin).toHaveBeenCalledWith(7, 100, false)
  })

  it('模型目录插件不显示账号路由灰度控制', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, manifest: { ...plugin.manifest, capabilities: [{
      id: 'openai.codex.model_catalog.v1', platform: 'openai', account_type: 'catalog',
    }] } }])
    const wrapper = mountView()
    await flushPromises()

    expect(wrapper.text()).not.toContain('admin.plugins.rollout')
  })

  it.each([7, 8])('keeps the latest session when reopening configuration for plugin %s', async (id) => {
    let resolveOld!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveOld = resolve }))
    listPlugins.mockResolvedValue([plugin, { ...plugin, id: 8, name: 'Other Transport' }])
    const wrapper = mountView()
    await flushPromises()
    const buttons = wrapper.findAll('button').filter(item => item.text() === 'admin.plugins.configure')
    await buttons[0].trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await buttons[id === 7 ? 0 : 1].trigger('click')
    await flushPromises()
    const currentURL = wrapper.get('iframe').attributes('src')

    resolveOld({ url: '/obsolete-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.get('iframe').attributes('src')).toBe(currentURL)
  })

  it('ignores an old session error while the reopened configuration is loading', async () => {
    let rejectOld!: (error: Error) => void
    let resolveCurrent!: (session: PluginUISession) => void
    createUISession
      .mockReturnValueOnce(new Promise((_, reject) => { rejectOld = reject }))
      .mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveCurrent = resolve }))
    const wrapper = mountView()
    await flushPromises()
    const button = wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!
    await button.trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    await button.trigger('click')
    rejectOld(new Error('obsolete failure'))
    await flushPromises()

    expect(wrapper.text()).not.toContain('obsolete failure')
    expect(wrapper.text()).toContain('admin.plugins.loadingUI')
    resolveCurrent({ url: '/current-session', bridge_token: 'current', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    await wrapper.get('iframe').trigger('load')
    expect(wrapper.get('iframe').attributes('src')).toBe('/current-session')
    expect(wrapper.text()).not.toContain('admin.plugins.loadingUI')
  })

  it('does not restore a session after the configuration dialog closes', async () => {
    let resolveSession!: (session: PluginUISession) => void
    createUISession.mockReturnValueOnce(new Promise<PluginUISession>((resolve) => { resolveSession = resolve }))
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    wrapper.getComponent(BaseDialog).vm.$emit('close')
    resolveSession({ url: '/closed-session', bridge_token: 'old', ui_bridge_version: 1, expires_at: '' })
    await flushPromises()
    expect(wrapper.find('iframe').exists()).toBe(false)
  })

  it('上传插件通过 step-up 控制器执行', async () => {
    const wrapper = mountView()
    await flushPromises()
    const input = wrapper.get('input[type="file"]')
    Object.defineProperty(input.element, 'files', {
      configurable: true,
      value: [new File(['plugin'], 'transport.s2plugin', { type: 'application/zip' })],
    })

    await input.trigger('change')
    await flushPromises()

    expect(stepUpRun).toHaveBeenCalledTimes(1)
    expect(uploadPlugin).toHaveBeenCalledTimes(1)
  })

  it('catalog plugins read only the public model inventory through the bridge', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, manifest: { ...plugin.manifest, capabilities: [{ id: 'openai.codex.model_catalog.v1', platform: 'openai', account_type: 'catalog' }] } }])
    const wrapper = mountView(); await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises(); const frame = wrapper.get('iframe').element as HTMLIFrameElement
    await wrapper.get('iframe').trigger('load')
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    window.dispatchEvent(new MessageEvent('message', { origin: 'null', source: frame.contentWindow, data: { source: 'sub2api-plugin-ui', bridge_token: 'bridge', type: 'catalog.load', request_id: 'catalog-read', group_id: 2 } }))
    await flushPromises()
    expect(getPluginCatalog).toHaveBeenCalledWith(7, 2)
    expect(stepUpRun).not.toHaveBeenCalled()
    expect(post).toHaveBeenCalledWith(expect.objectContaining({ ok: true, catalog: expect.objectContaining({ group_id: 2 }) }), '*')
  })

  it('does not expose catalog inventory to unrelated plugins', async () => {
    const wrapper = mountView(); await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises(); const frame = wrapper.get('iframe').element as HTMLIFrameElement
    window.dispatchEvent(new MessageEvent('message', { origin: 'null', source: frame.contentWindow, data: { source: 'sub2api-plugin-ui', bridge_token: 'bridge', type: 'catalog.load', request_id: 'catalog-forbidden' } }))
    await flushPromises(); expect(getPluginCatalog).not.toHaveBeenCalled()
  })

  it('passes the exact revision read by the editor through step-up to the guarded save', async () => {
    const wrapper = mountView(); await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const frame = wrapper.get('iframe').element as HTMLIFrameElement
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    const send = (type: string, args: Record<string, unknown>) => window.dispatchEvent(new MessageEvent('message', {
      origin: 'null', source: frame.contentWindow, data: {
        source: 'sub2api-plugin-ui', bridge_token: 'bridge', type, request_id: type, ...args,
      },
    }))
    const revision = '"' + 'a'.repeat(64) + '"'
    send('config.load', { with_revision: true }); await flushPromises()
    expect(post).toHaveBeenLastCalledWith(expect.objectContaining({ ok: true, revision }), '*')
    send('config.save', { config: { models: [] }, revision }); await flushPromises()
    expect(savePluginConfig).toHaveBeenCalledWith(7, { models: [] }, revision)
    expect(stepUpRun).toHaveBeenCalledOnce()
  })

  it.each([true, false])('isolates catalog drafts without publishing (catalog capability: %s)', async allowed => {
    if (allowed) listPlugins.mockResolvedValue([{ ...plugin, manifest: { ...plugin.manifest, capabilities: [{ id: 'openai.codex.model_catalog.v1' }] } }])
    const wrapper = mountView()
    await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises()
    const frame = wrapper.get('iframe').element as HTMLIFrameElement
    await wrapper.get('iframe').trigger('load')
    const post = vi.spyOn(frame.contentWindow!, 'postMessage')
    const draft = { config: { models: [] } }
    for (const [i, op] of ['save', 'load', 'clear', 'load'].entries()) {
      window.dispatchEvent(new MessageEvent('message', { origin: 'null', source: frame.contentWindow, data: {
        source: 'sub2api-plugin-ui', bridge_token: 'bridge', type: 'catalog.draft.' + op, request_id: 'draft-' + i, draft,
      } }))
      await flushPromises()
      const expected = allowed && op === 'load' ? { draft: i === 1 ? draft : null } : {}
      expect(post).toHaveBeenLastCalledWith(expect.objectContaining({ ok: allowed, ...expected }), '*')
    }
    expect(stepUpRun).not.toHaveBeenCalled()
    expect(savePluginConfig).not.toHaveBeenCalled()
  })

  it('rejects invalid catalog group parameters before making an API request', async () => {
    listPlugins.mockResolvedValue([{ ...plugin, manifest: { ...plugin.manifest, capabilities: [{ id: 'openai.codex.model_catalog.v1', platform: 'openai', account_type: 'catalog' }] } }])
    const wrapper = mountView(); await flushPromises()
    await wrapper.findAll('button').find(item => item.text() === 'admin.plugins.configure')!.trigger('click')
    await flushPromises(); const frame = wrapper.get('iframe').element as HTMLIFrameElement
    window.dispatchEvent(new MessageEvent('message', { origin: 'null', source: frame.contentWindow, data: { source: 'sub2api-plugin-ui', bridge_token: 'bridge', type: 'catalog.load', request_id: 'catalog-invalid', group_id: '../accounts' } }))
    await flushPromises(); expect(getPluginCatalog).not.toHaveBeenCalled()
  })
})
