/**
 * 外部插件加载器守护测试：
 *   - 脚本注入：仅对带 assets 的启用插件注入 <script>，src 按 API base 解析
 *   - 注册链路：注入前登记期望 ID → 脚本调用 window.__SUB2API_PLUGIN_REGISTER__
 *     → 路由/导航/文案贡献生效（宿主绑定真实 router/i18n 的等价接口）
 *   - 隔离：单个脚本 onerror/超时仅 warn 跳过，不影响其他插件
 *   - 幂等与清理：脚本按 ID 只注入一次；停用移除路由与导航；
 *     同会话重新启用复活缓存描述符而不重复注入
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import type { RouteRecordRaw } from 'vue-router'

import { loadExternalPlugins, _resetLoaderForTest } from '../loader'
import { pluginNav, _resetPluginsForTest, _resetRuntimeForTest, _setPluginsForTest } from '../registry'
import type { PluginDescriptor } from '../types'

const { addRoute, hasRoute, getRoutes, mergeLocaleMessage, removers } = vi.hoisted(() => {
  const removerList: Array<ReturnType<typeof vi.fn>> = []
  return {
    addRoute: vi.fn(() => {
      const remover = vi.fn()
      removerList.push(remover)
      return remover
    }),
    hasRoute: vi.fn((_name: unknown) => false),
    getRoutes: vi.fn((): Array<{ path: string }> => []),
    mergeLocaleMessage: vi.fn(),
    removers: removerList
  }
})

// loader 经宿主接口使用 router/i18n：mock 为轻量假件，避免拖入整棵应用模块图
vi.mock('@/router', () => ({
  default: { addRoute, hasRoute, getRoutes }
}))

vi.mock('@/i18n', () => ({
  i18n: { global: { mergeLocaleMessage } },
  default: { global: { mergeLocaleMessage } }
}))

const EXT_ASSETS = '/api/v1/plugins/ext-hello/assets/plugin.js'

function extDescriptor(id = 'ext-hello'): PluginDescriptor {
  return {
    id,
    routes: [
      {
        path: `/admin/${id}`,
        name: `Ext-${id}`,
        component: () => Promise.resolve({ default: { template: '<div />' } })
      } as RouteRecordRaw
    ],
    adminNav: [{ path: `/admin/${id}`, labelKey: `plugins.${id}.navLabel` }],
    i18n: { zh: { navLabel: '外部演示' }, en: { navLabel: 'External Demo' } }
  }
}

function scriptOf(id: string): HTMLScriptElement | null {
  return document.head.querySelector<HTMLScriptElement>(`script[data-plugin-id="${id}"]`)
}

// registerFromScript 模拟"loader 注入的脚本在同步求值期间调用全局注册回调"：
// 把 document.currentScript 指向 loader 实际创建（并绑定）的那个元素。
function registerFromScript(scriptOwnerId: string, descriptor: PluginDescriptor): void {
  const scriptEl = scriptOf(scriptOwnerId)
  expect(scriptEl).not.toBeNull()
  const currentScriptSpy = vi.spyOn(document, 'currentScript', 'get').mockReturnValue(scriptEl)
  try {
    window.__SUB2API_PLUGIN_REGISTER__!(descriptor)
  } finally {
    currentScriptSpy.mockRestore()
  }
}

let warnSpy: ReturnType<typeof vi.spyOn>

beforeEach(() => {
  addRoute.mockClear()
  hasRoute.mockClear()
  hasRoute.mockImplementation(() => false)
  getRoutes.mockClear()
  getRoutes.mockImplementation(() => [])
  mergeLocaleMessage.mockClear()
  removers.length = 0
  _setPluginsForTest([])
  warnSpy = vi.spyOn(console, 'warn').mockImplementation(() => {})
})

afterEach(() => {
  _resetLoaderForTest()
  _resetRuntimeForTest()
  _resetPluginsForTest()
  document.head.querySelectorAll('script[data-plugin-id]').forEach((el) => el.remove())
  warnSpy.mockRestore()
  vi.useRealTimers()
})

describe('loadExternalPlugins 脚本注入', () => {
  it('对带 assets 的插件注入 script，src 为按 API base 解析后的资产路径', async () => {
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    const script = scriptOf('ext-hello')
    expect(script).not.toBeNull()
    expect(script!.getAttribute('src')).toBe(EXT_ASSETS)
    expect(script!.async).toBe(true)

    script!.dispatchEvent(new Event('load'))
    await pending
  })

  it('不带 assets / 空 id 的清单项不注入脚本', async () => {
    await loadExternalPlugins([{ id: 'demo' }, { id: '', assets: '/x.js' }, { id: 'blank', assets: '' }])

    expect(document.head.querySelectorAll('script[data-plugin-id]')).toHaveLength(0)
  })

  it('绝对 URL 的 assets 原样使用', async () => {
    const pending = loadExternalPlugins([
      { id: 'ext-hello', assets: 'https://cdn.example.com/plugin.js' }
    ])

    expect(scriptOf('ext-hello')!.getAttribute('src')).toBe('https://cdn.example.com/plugin.js')
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending
  })

  it('同一插件脚本只注入一次（重复调用 / 加载失败后均不重试，刷新自然重置）', async () => {
    const first = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await first

    await loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])
    expect(document.head.querySelectorAll('script[data-plugin-id="ext-hello"]')).toHaveLength(1)
  })
})

describe('loadExternalPlugins 注册链路', () => {
  it('脚本注册后：路由经 router.addRoute 注入（meta.pluginId 补写）、导航与文案生效', async () => {
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    // 模拟插件 IIFE 脚本体：同步求值期间调用全局注册回调
    registerFromScript('ext-hello', extDescriptor())
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    expect(addRoute).toHaveBeenCalledTimes(1)
    const injected = addRoute.mock.calls[0][0] as RouteRecordRaw
    expect(injected.meta?.pluginId).toBe('ext-hello')
    expect(pluginNav('admin', new Set(['ext-hello']))).toHaveLength(1)
    expect(mergeLocaleMessage).toHaveBeenCalledWith('zh', {
      plugins: { 'ext-hello': { navLabel: '外部演示' } }
    })
    expect(mergeLocaleMessage).toHaveBeenCalledWith('en', {
      plugins: { 'ext-hello': { navLabel: 'External Demo' } }
    })
  })

  it('未在清单内的 ID 无法经全局回调注册（期望集合核对，fail-closed）', async () => {
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    registerFromScript('ext-hello', extDescriptor('ext-rogue'))
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    expect(addRoute).not.toHaveBeenCalled()
    expect(pluginNav('admin', new Set(['ext-rogue']))).toEqual([])
    expect(warnSpy).toHaveBeenCalled()
  })

  it('异步注册（脚本求值结束后）被拒：归属绑定只认同步求值期间的调用', async () => {
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    // 脚本已求值完毕（currentScript === null）后经全局回调注册。
    window.__SUB2API_PLUGIN_REGISTER__!(extDescriptor())

    expect(addRoute).not.toHaveBeenCalled()
    expect(pluginNav('admin', new Set(['ext-hello']))).toEqual([])
    expect(warnSpy).toHaveBeenCalled()
  })
})

describe('loadExternalPlugins 路由冲突防护', () => {
  it('路由 name 与既有路由冲突：该路由被拒（warn），导航/文案贡献照常', async () => {
    hasRoute.mockImplementation((name: unknown) => name === 'Ext-ext-hello')
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    registerFromScript('ext-hello', extDescriptor())
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    expect(addRoute).not.toHaveBeenCalled()
    expect(pluginNav('admin', new Set(['ext-hello']))).toHaveLength(1)
    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('route'),
      expect.any(Error)
    )
  })

  it('路由 path 与既有路由冲突：该路由被拒，核心路由不被顶替', async () => {
    getRoutes.mockImplementation(() => [{ path: '/admin/ext-hello' }])
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    registerFromScript('ext-hello', extDescriptor())
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    expect(addRoute).not.toHaveBeenCalled()
    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('route'),
      expect.any(Error)
    )
  })

  it('children 命名与路径参与冲突检查（相对路径按父路径展开）', async () => {
    getRoutes.mockImplementation(() => [{ path: '/admin/ext-hello/sub' }])
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    const descriptor = extDescriptor()
    descriptor.routes = [
      {
        path: '/admin/ext-hello',
        component: () => Promise.resolve({ default: { template: '<div />' } }),
        children: [
          {
            path: 'sub',
            component: () => Promise.resolve({ default: { template: '<div />' } })
          }
        ]
      } as RouteRecordRaw
    ]
    registerFromScript('ext-hello', descriptor)
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    expect(addRoute).not.toHaveBeenCalled()
    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('route'),
      expect.any(Error)
    )
  })
})

describe('loadExternalPlugins 失败隔离', () => {
  it('单个脚本 onerror：仅 warn 跳过，其他插件照常注册', async () => {
    const pending = loadExternalPlugins([
      { id: 'ext-bad', assets: '/api/v1/plugins/ext-bad/assets/plugin.js' },
      { id: 'ext-hello', assets: EXT_ASSETS }
    ])

    scriptOf('ext-bad')!.dispatchEvent(new Event('error'))
    registerFromScript('ext-hello', extDescriptor())
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('ext-bad'),
      expect.any(Error)
    )
    expect(pluginNav('admin', new Set(['ext-hello']))).toHaveLength(1)
  })

  it('脚本加载超时：warn 跳过且 loadExternalPlugins 正常返回', async () => {
    vi.useFakeTimers()
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    await vi.advanceTimersByTimeAsync(15_000)
    await pending

    expect(warnSpy).toHaveBeenCalledWith(
      expect.stringContaining('ext-hello'),
      expect.any(Error)
    )
  })
})

describe('loadExternalPlugins 停用与复活', () => {
  async function loadAndRegister(): Promise<void> {
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])
    registerFromScript('ext-hello', extDescriptor())
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending
  }

  it('清单收缩（停用）：removeRoute 清理、导航移除', async () => {
    await loadAndRegister()
    expect(pluginNav('admin', new Set(['ext-hello']))).toHaveLength(1)

    await loadExternalPlugins([])

    expect(removers).toHaveLength(1)
    expect(removers[0]).toHaveBeenCalledTimes(1)
    expect(pluginNav('admin', new Set(['ext-hello']))).toEqual([])
  })

  it('同会话重新启用：复活缓存描述符（路由重新注入），不重复注入脚本', async () => {
    await loadAndRegister()
    await loadExternalPlugins([])

    await loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    expect(document.head.querySelectorAll('script[data-plugin-id="ext-hello"]')).toHaveLength(1)
    expect(addRoute).toHaveBeenCalledTimes(2)
    expect(pluginNav('admin', new Set(['ext-hello']))).toHaveLength(1)
  })

  it('脚本注入途中被停用（注册被拒、无缓存）：re-enable 时重新注入脚本', async () => {
    // 脚本注入后仍在网络加载途中（未触发 load、未注册）
    const pending = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    // 加载途中管理员停用：注册期望被撤销
    await loadExternalPlugins([])

    // 迟到的注册被拒（fail-closed），无描述符缓存
    registerFromScript('ext-hello', extDescriptor())
    expect(addRoute).not.toHaveBeenCalled()
    scriptOf('ext-hello')!.dispatchEvent(new Event('load'))
    await pending

    // re-enable：无缓存可复活 → 必须重新注入脚本（而非整会话失活）
    const second = loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])
    const scripts = Array.from(
      document.head.querySelectorAll<HTMLScriptElement>('script[data-plugin-id="ext-hello"]')
    )
    expect(scripts).toHaveLength(2)

    // 新脚本同步注册成功，贡献生效
    const newScript = scripts[scripts.length - 1]
    const currentScriptSpy = vi.spyOn(document, 'currentScript', 'get').mockReturnValue(newScript)
    try {
      window.__SUB2API_PLUGIN_REGISTER__!(extDescriptor())
    } finally {
      currentScriptSpy.mockRestore()
    }
    newScript.dispatchEvent(new Event('load'))
    await second

    expect(addRoute).toHaveBeenCalledTimes(1)
    expect(pluginNav('admin', new Set(['ext-hello']))).toHaveLength(1)
  })

  it('注册成功后停用再启用：缓存仍在，不因竞态补偿误清注入标记', async () => {
    await loadAndRegister()
    await loadExternalPlugins([]) // 停用：有缓存描述符，注入标记必须保留

    await loadExternalPlugins([{ id: 'ext-hello', assets: EXT_ASSETS }])

    expect(document.head.querySelectorAll('script[data-plugin-id="ext-hello"]')).toHaveLength(1)
  })
})
