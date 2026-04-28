/**
 * 把 HostSdk 实例挂到 `window` 上，让动态加载的插件 entry.js 通过全局对象拿到。
 *
 * 选择全局对象注入而不是直接 import 的原因：
 *   - 插件 bundle 是远程 URL（不参与 host vite 编译），无法用 `import` 解析 host 内部模块。
 *   - 全局变量约定简单清晰，配合 SDK version 字段做兼容判断。
 *   - 插件可以通过 `window.__SUB2API_HOST_SDK__` 同步取到 SDK，无需异步等待。
 */
import type { Router } from 'vue-router'
import type { Pinia } from 'pinia'
import { HOST_SDK_GLOBAL_KEY, type HostSdk } from './host-sdk'
import { createHostSdk } from './host-sdk-impl'

let installedSdk: HostSdk | null = null

/**
 * 创建 host sdk 实例并挂到 window。重复调用是 idempotent 的：第二次会返回已挂载的实例。
 *
 * 主入口（main.ts）应在 router 安装之后调用一次，把 sdk 暴露给后续动态加载的插件。
 *
 * pinia 实例必须显式传入: sdk.runtime.createApp 内部会 app.use(pinia) 让 plugin
 * 子 app 与 host 共享 store registry. 不能在 createHostSdk 内 useStore — 那只读
 * 当前 active pinia, 我们要对象本身.
 */
export function attachHostSdkToWindow(router: Router, pinia: Pinia): HostSdk {
  if (installedSdk) {
    return installedSdk
  }
  const sdk = createHostSdk(router, pinia)
  installedSdk = sdk
  if (typeof window !== 'undefined') {
    window[HOST_SDK_GLOBAL_KEY] = sdk
  }
  return sdk
}

/**
 * 读取已挂载的 host sdk。仅当先调用过 `attachHostSdkToWindow` 后才有效。
 * 主要给 loader-runtime 在动态 import 失败时降级使用 / 单测使用。
 */
export function getHostSdk(): HostSdk | null {
  if (installedSdk) {
    return installedSdk
  }
  if (typeof window !== 'undefined') {
    return window[HOST_SDK_GLOBAL_KEY] ?? null
  }
  return null
}
