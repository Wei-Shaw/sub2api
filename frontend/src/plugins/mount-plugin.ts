/**
 * Plugin Shadow DOM 挂载工具.
 *
 * 设计目标:
 *   - 给每个 plugin route 创建一个独立的 Shadow Root, plugin 自己的 Vue app
 *     mount 进去, DOM/CSS 与 host 物理隔离
 *   - plugin 渲染抛错 / 注入全局样式都不会污染 host (浏览器 Shadow DOM 边界保证)
 *   - 仍然通过 importmap 共享 vue / vue-router / vue-i18n / pinia / axios /
 *     @sub2api/plugin-sdk 这 6 个 singleton, plugin 借 sdk.runtime.createApp
 *     获得已 use(pinia)/use(i18n) 的 Vue app
 *
 * 调用流程 (PluginView 内):
 *   const handle = await mountPluginAssets({ container, assets, ctx, ... })
 *   // 路由切换时:
 *   handle.unmount()
 *
 * CSS 注入策略 (Shadow Root 内, 不污染 host head):
 *   1. plugin-sdk.css — SDK 共享组件 + Tailwind base/utilities 给 SDK 用
 *   2. plugin entry.css — plugin 自己的 SFC scoped style
 *   两者都通过 <link> 加到 shadow root, 浏览器自然按 ShadowRoot 处理 CSS scope.
 *
 * CSS 自定义属性 (--color-primary 等) 默认从 :root 通过 inheritance 穿透到
 * shadow host element, 然后 inherits across shadow boundary 进入 shadow tree.
 * 不需要显式注入 — host 的主题 token 在 plugin 内自动可用.
 */

import type { RouteLocationNormalizedLoaded } from 'vue-router'
import type {
  PluginInstance,
  PluginRuntimeAssets,
  PluginRuntimeContext,
} from '@sub2api/plugin-sdk'

import { TAILWIND_SHADOW_PREFLIGHT } from './tailwind-shadow-preflight'

/** SDK 共享样式表 URL — 后端 ServePluginSharedAsset 暴露. */
const SDK_STYLESHEET_URL = '/api/v1/plugin-assets/__shared__/plugin-sdk.css'

export interface MountPluginAssetsOptions {
  /** Shadow Root 的宿主 element. 必须已挂在 host DOM 树中. */
  container: HTMLElement
  /** loadPluginEntry 已加载并 install 完成的 assets, 必须含 mount(). */
  assets: PluginRuntimeAssets
  /** 当前路由的浅快照, 透传给 plugin.mount 的 ctx. */
  route: RouteLocationNormalizedLoaded
  /** 路由 meta.componentPath, plugin 据此决定渲染哪个 view. */
  componentPath: string
  /** plugin 名, 用于 logging + DOM 标识. */
  pluginName: string
  /** plugin 自己的 entry.css URL (manifest.entry_css_url). 没有时跳过. */
  entryCssUrl?: string
}

export interface MountedPluginHandle {
  /** 卸载 plugin: 调 instance.unmount + 清掉 Shadow Root 内容. */
  unmount(): void
}

export async function mountPluginAssets(opts: MountPluginAssetsOptions): Promise<MountedPluginHandle> {
  const { container, assets, route, componentPath, pluginName, entryCssUrl } = opts

  if (!assets || typeof assets.mount !== 'function') {
    throw new Error(`plugin "${pluginName}" did not return a mount() function`)
  }

  // 设置 host element 自身的 CSS — 让 shadow host 占满父级.
  container.style.display = 'block'
  container.style.flex = '1'
  container.style.minHeight = '0'

  // 同一 host element 多次 mount (路由切换 / retry / hot-reload) 时, 浏览器禁止
  // 重复 attachShadow — 抛 "Shadow root cannot be created on a host which already
  // hosts a shadow tree". 必须复用已有 shadowRoot, 仅清空内容.
  let shadow = container.shadowRoot
  if (shadow) {
    while (shadow.firstChild) {
      shadow.removeChild(shadow.firstChild)
    }
  } else {
    shadow = container.attachShadow({ mode: 'open' })
  }

  // Shadow Root 内 :host 样式 + minimal reset.
  //
  // 为什么需要 reset:
  //   Shadow DOM 内的元素*不会*从 host 继承 author-CSS 类型的样式 (例如
  //   `*, *::before, *::after { box-sizing: border-box }`). Tailwind 的工具类
  //   假设 box-sizing: border-box, 没有这条 reset 时 padding/border 会让
  //   元素超出预期宽度. 我们手动注入 minimal reset.
  //
  //   完整 Tailwind preflight 在 plugin-sdk.css 编译产物内不存在 (SDK 没在
  //   build 时注入 @tailwind base — preset 只提供 token, 不开 preflight),
  //   所以这里我们提供必要的最小集.
  const hostStyle = document.createElement('style')
  hostStyle.textContent = `
    :host {
      display: block;
      width: 100%;
      height: 100%;
      min-height: 0;
      font-family: inherit;
      color: inherit;
    }
    *, *::before, *::after {
      box-sizing: border-box;
    }
    ${TAILWIND_SHADOW_PREFLIGHT}
    .plugin-shadow-root {
      width: 100%;
      height: 100%;
      min-height: 0;
      display: flex;
      flex-direction: column;
    }
  `
  shadow.appendChild(hostStyle)

  // 1) SDK 共享样式表. plugin 内使用 SDK 组件 (Icon/DataTable/...) 时需要这份样式.
  //    Tailwind base 也通过这里提供, plugin 自己 entry.css 可以只 @tailwind utilities.
  appendStylesheetLink(shadow, SDK_STYLESHEET_URL, `plugin-sdk-css-${pluginName}`)

  // 2) plugin 自己的 entry.css. 注入到 shadow root, 不污染 host head.
  if (entryCssUrl) {
    appendStylesheetLink(shadow, entryCssUrl, `plugin-entry-css-${pluginName}`)
  }

  // 3) plugin 渲染容器 — sdk.runtime.createApp 会 mount 到这个 div.
  const root = document.createElement('div')
  root.className = 'plugin-shadow-root'
  root.dataset.pluginName = pluginName
  shadow.appendChild(root)

  // 3.0) Teleport 落地容器 (BaseDialog / Select 等通过 Vue inject 拿到这个 div).
  //
  // 为什么必须是 .plugin-shadow-root 的*同级*兄弟节点 (而不是 root 的子节点):
  //   - 插件组件树内部任意祖先若有 overflow:hidden / clip / contain:layout,
  //     Teleport 到子节点的 overlay 会被裁切. portal 作为 root 的兄弟节点, 不在
  //     插件组件树的 ancestor chain 里, 等价于「teleport 到 shadow body」.
  //   - portal 仍在 ShadowRoot 内, 自动继承 shadow 内的 stylesheet (entry.css /
  //     plugin-sdk.css), Tailwind utilities 与 SDK 组件样式正常生效, 同时不污染
  //     host light DOM.
  //   - position:fixed 相对 viewport, 不受 shadow 边界影响, 全屏遮罩 / 下拉定位
  //     算法 (getBoundingClientRect) 全部按原逻辑工作.
  const portal = document.createElement('div')
  portal.className = 'plugin-shadow-portal'
  portal.dataset.pluginName = pluginName
  shadow.appendChild(portal)

  // 3.1) 同步 host 的 dark class 到 shadow root 内根容器.
  //
  // 为什么需要:
  //   host 用 `:is(.dark *)` 选择器实现 tailwind `dark:` variant, 但这个选择器
  //   不能跨 shadow 边界 — shadow tree 内的元素不会被 host 的 `.dark` ancestor
  //   匹配. 解决方案: 在 shadow tree 自己的根容器上镜像一份 `dark` class, 这样
  //   shadow 内 stylesheet 里的 `.dark ...` / `:is(.dark *)` 选择器就能命中.
  const applyDarkState = (): void => {
    const isDark = document.documentElement.classList.contains('dark')
    root.classList.toggle('dark', isDark)
    // portal 是 root 的*同级*节点, 不会被 root 上的 `.dark` ancestor 选择器命中,
    // 必须独立同步, 否则 Teleport 进 portal 的 overlay 在 dark mode 下样式残缺.
    portal.classList.toggle('dark', isDark)
  }
  applyDarkState()

  const darkObserver = new MutationObserver(() => {
    applyDarkState()
  })
  darkObserver.observe(document.documentElement, {
    attributes: true,
    attributeFilter: ['class'],
  })

  // 4) 调 plugin 自己的 mount.
  //    plugin 收到的是整个 ShadowRoot, 它可以选择 root div 或自己再 createElement.
  //    约定: plugin mount 内部应该把 view mount 到 .plugin-shadow-root (上面的 root).
  const ctx: PluginRuntimeContext = {
    componentPath,
    route,
    pluginName,
  }
  let instance: PluginInstance
  try {
    instance = await assets.mount(shadow, ctx)
  } catch (err) {
    // mount 失败时把 shadow 清空, 不留半成品.
    darkObserver.disconnect()
    while (shadow.firstChild) {
      shadow.removeChild(shadow.firstChild)
    }
    throw err
  }

  return {
    unmount(): void {
      darkObserver.disconnect()
      try {
        instance.unmount()
      } catch (err) {
        // eslint-disable-next-line no-console
        console.error('[mount-plugin] plugin unmount threw, ignoring:', err)
      }
      // shadow root 跟着 container DOM 自然 GC, 不需要手动 detach (DOM 不允许).
      // PluginView 会清掉 container 自身.
    },
  }
}

function appendStylesheetLink(shadow: ShadowRoot, href: string, marker: string): void {
  const link = document.createElement('link')
  link.rel = 'stylesheet'
  link.href = href
  link.dataset.marker = marker
  shadow.appendChild(link)
}
